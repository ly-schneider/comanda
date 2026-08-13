package staffperks

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/order"
	"backend/internal/generated/ent/orderline"
	"backend/internal/generated/ent/orderpayment"
	"backend/internal/generated/ent/volunteercampaign"
	"backend/internal/repository"
	coreservice "backend/internal/service"
	"backend/internal/utils"
)

const Key = "staff-perks"

const (
	sessionBytes    = 24
	accessCodeLen   = 4
	accessCodeAlph  = "ABCDEFGHIJKLMNOPQRSTUVWXYZ123456789"
	qrPayloadPrefix = "CAMP:"
)

type CreateCampaignInput struct {
	Name           string
	ValidFrom      *time.Time
	ValidUntil     *time.Time
	Products       []VolunteerCampaignProductInput
	MaxRedemptions int
}

type UpdateCampaignInput struct {
	Name           string
	ValidFrom      *time.Time
	ValidUntil     *time.Time
	Status         volunteercampaign.Status
	Products       []VolunteerCampaignProductInput
	MaxRedemptions *int
}

type RedemptionView struct {
	ID        string
	OrderID   string
	CreatedAt time.Time
}

type CampaignDetail struct {
	Campaign    *ent.VolunteerCampaign
	Products    []coreservice.VolunteerCampaignProductView
	Redemptions []RedemptionView
}

type Service interface {
	coreservice.CampaignClaimService

	CreateCampaign(ctx context.Context, input CreateCampaignInput) (*ent.VolunteerCampaign, error)
	ListCampaigns(ctx context.Context) ([]*ent.VolunteerCampaign, error)
	GetCampaign(ctx context.Context, id string) (*CampaignDetail, error)
	UpdateCampaign(ctx context.Context, id string, input UpdateCampaignInput) (*ent.VolunteerCampaign, error)
	EndCampaign(ctx context.Context, id string) error
	RotateClaimToken(ctx context.Context, id string) (string, error)
}

type service struct {
	campaigns   VolunteerCampaignRepository
	redemptions VolunteerRedemptionRepository
	orders      repository.OrderRepository
	lines       repository.OrderLineRepository
	payments    repository.OrderPaymentRepository
	stations    coreservice.StationService
}

func NewService(
	campaigns VolunteerCampaignRepository,
	redemptions VolunteerRedemptionRepository,
	orders repository.OrderRepository,
	lines repository.OrderLineRepository,
	payments repository.OrderPaymentRepository,
	stations coreservice.StationService,
) Service {
	return &service{
		campaigns:   campaigns,
		redemptions: redemptions,
		orders:      orders,
		lines:       lines,
		payments:    payments,
		stations:    stations,
	}
}

func (s *service) newSessionID() (string, error) {
	return utils.GenerateRandomURLSafe(sessionBytes)
}

func generateAccessCode() string {
	buf := make([]byte, accessCodeLen)
	raw := make([]byte, accessCodeLen)
	_, _ = rand.Read(raw)
	n := byte(len(accessCodeAlph))
	for i := 0; i < accessCodeLen; i++ {
		buf[i] = accessCodeAlph[raw[i]%n]
	}
	return string(buf)
}

// The station scanner keys off the CAMP: prefix to route to campaign redeem.
func buildQRPayload(claimToken string) string {
	return qrPayloadPrefix + claimToken
}

func (s *service) CreateCampaign(ctx context.Context, input CreateCampaignInput) (*ent.VolunteerCampaign, error) {
	if strings.TrimSpace(input.Name) == "" {
		return nil, errors.New("name_required")
	}
	if len(input.Products) == 0 {
		return nil, coreservice.ErrVolunteerCampaignHasNoProducts
	}
	if input.MaxRedemptions <= 0 {
		return nil, errors.New("max_redemptions_must_be_positive")
	}
	if err := validateProducts(input.Products); err != nil {
		return nil, err
	}

	accessCode := generateAccessCode()

	// Runs inside the per-request tenant transaction; the writes are atomic
	// through that outer tx. Opening a nested Ent tx here would run on the
	// static app pool with no app.tenant_id GUC and be blocked by RLS.
	campaign, err := s.campaigns.Create(ctx, input.Name, accessCode, input.ValidFrom, input.ValidUntil, volunteercampaign.StatusActive, input.MaxRedemptions)
	if err != nil {
		return nil, fmt.Errorf("create campaign: %w", err)
	}

	if err := s.campaigns.ReplaceProducts(ctx, campaign.ID, input.Products); err != nil {
		return nil, fmt.Errorf("attach products: %w", err)
	}

	return campaign, nil
}

type productSnapshot struct {
	ID       string
	Name     string
	Quantity int
}

func (s *service) createGratisOrder(ctx context.Context, products []productSnapshot) (string, error) {
	ord, err := s.orders.Create(ctx, 0, order.StatusPaid, order.OriginShop, nil, nil, nil, nil, nil)
	if err != nil {
		return "", fmt.Errorf("create order: %w", err)
	}

	for _, p := range products {
		if _, err := s.lines.Create(ctx, ord.ID, orderline.LineTypeSimple, p.ID, truncatedTitle(p.Name), p.Quantity, 0, nil, nil, nil); err != nil {
			return "", fmt.Errorf("create order line: %w", err)
		}
	}

	if _, err := s.payments.Create(ctx, ord.ID, orderpayment.MethodGRATIS_STAFF, 0, time.Now(), nil); err != nil {
		return "", fmt.Errorf("create payment: %w", err)
	}
	return ord.ID, nil
}

func truncatedTitle(s string) string {
	if len(s) > 20 {
		return s[:20]
	}
	return s
}

func (s *service) ListCampaigns(ctx context.Context) ([]*ent.VolunteerCampaign, error) {
	return s.campaigns.List(ctx)
}

func (s *service) GetCampaign(ctx context.Context, id string) (*CampaignDetail, error) {
	campaign, err := s.campaigns.GetByID(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, coreservice.ErrVolunteerCampaignNotFound
		}
		return nil, err
	}
	cps, err := s.campaigns.ListProducts(ctx, id)
	if err != nil {
		return nil, err
	}
	products := campaignProductsToViews(cps)

	reds, err := s.redemptions.ListByCampaign(ctx, id, 50)
	if err != nil {
		return nil, err
	}
	redViews := make([]RedemptionView, 0, len(reds))
	for _, r := range reds {
		redViews = append(redViews, RedemptionView{
			ID:        r.ID,
			OrderID:   r.OrderID,
			CreatedAt: r.CreatedAt,
		})
	}
	return &CampaignDetail{
		Campaign:    campaign,
		Products:    products,
		Redemptions: redViews,
	}, nil
}

func (s *service) UpdateCampaign(ctx context.Context, id string, input UpdateCampaignInput) (*ent.VolunteerCampaign, error) {
	if strings.TrimSpace(input.Name) == "" {
		return nil, errors.New("name_required")
	}

	// Atomicity comes from the outer per-request tenant tx; a nested Ent tx
	// here would bypass the request's app.tenant_id GUC and be blocked by RLS.
	existing, err := s.campaigns.GetByID(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, coreservice.ErrVolunteerCampaignNotFound
		}
		return nil, err
	}

	campaign, err := s.campaigns.Update(ctx, id, input.Name, existing.AccessCode, input.ValidFrom, input.ValidUntil, input.Status)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, coreservice.ErrVolunteerCampaignNotFound
		}
		return nil, err
	}

	if input.MaxRedemptions != nil {
		if *input.MaxRedemptions <= 0 {
			return nil, errors.New("max_redemptions_must_be_positive")
		}
		updated, ok, err := s.campaigns.UpdateMaxRedemptions(ctx, id, *input.MaxRedemptions)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, coreservice.ErrVolunteerMaxBelowCount
		}
		campaign = updated
	}

	if len(input.Products) > 0 {
		if err := validateProducts(input.Products); err != nil {
			return nil, err
		}
		if err := s.campaigns.ReplaceProducts(ctx, id, input.Products); err != nil {
			return nil, err
		}
	}
	return campaign, nil
}

func (s *service) EndCampaign(ctx context.Context, id string) error {
	if err := s.campaigns.SetStatus(ctx, id, volunteercampaign.StatusEnded); err != nil {
		if ent.IsNotFound(err) {
			return coreservice.ErrVolunteerCampaignNotFound
		}
		return err
	}
	return nil
}

func (s *service) RotateClaimToken(ctx context.Context, id string) (string, error) {
	return s.campaigns.RotateClaimToken(ctx, id)
}

func (s *service) VerifyAccess(ctx context.Context, token string, code string) (string, error) {
	campaign, err := s.campaigns.GetByClaimToken(ctx, token)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", coreservice.ErrVolunteerCampaignNotFound
		}
		return "", err
	}
	if campaign.Status != volunteercampaign.StatusActive {
		return "", coreservice.ErrVolunteerCampaignInactive
	}
	if !campaignWithinValidity(campaign, time.Now()) {
		return "", coreservice.ErrVolunteerCampaignOutsideValid
	}
	if code != campaign.AccessCode {
		return "", coreservice.ErrVolunteerAccessCodeInvalid
	}
	return s.newSessionID()
}

func (s *service) GetClaimView(ctx context.Context, token string) (*coreservice.VolunteerClaimView, error) {
	campaign, err := s.campaigns.GetByClaimToken(ctx, token)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, coreservice.ErrVolunteerCampaignNotFound
		}
		return nil, err
	}
	cps, err := s.campaigns.ListProducts(ctx, campaign.ID)
	if err != nil {
		return nil, err
	}
	return &coreservice.VolunteerClaimView{
		Campaign:  campaign,
		Products:  campaignProductsToViews(cps),
		QRPayload: buildQRPayload(campaign.ClaimToken),
	}, nil
}

func servesAny(stationServes map[string]struct{}, views []coreservice.VolunteerCampaignProductView) bool {
	for _, v := range views {
		if _, ok := stationServes[v.ProductID]; ok {
			return true
		}
	}
	return false
}

func validateProducts(items []VolunteerCampaignProductInput) error {
	seen := make(map[string]struct{}, len(items))
	for _, it := range items {
		if it.ChoiceGroup < 0 || it.OptionGroup < 0 {
			return coreservice.ErrVolunteerCampaignProductInvalid
		}
		if _, dup := seen[it.ProductID]; dup {
			return coreservice.ErrVolunteerCampaignProductInvalid
		}
		seen[it.ProductID] = struct{}{}
	}
	return nil
}

// resolveCampaignSelection reduces the campaign's products to the ones handed out
// for a single redemption. Choice group 0 is always included; every positive group
// contributes exactly one product — auto-picked when the group holds a single
// option, otherwise taken from selections.
func resolveCampaignSelection(
	products []coreservice.VolunteerCampaignProductView,
	selections []coreservice.VolunteerChoiceSelection,
) ([]coreservice.VolunteerCampaignProductView, error) {
	groups := make(map[int][]coreservice.VolunteerCampaignProductView)
	groupOrder := make([]int, 0, len(products))
	out := make([]coreservice.VolunteerCampaignProductView, 0, len(products))

	for _, p := range products {
		if p.ChoiceGroup <= 0 {
			out = append(out, p)
			continue
		}
		if _, ok := groups[p.ChoiceGroup]; !ok {
			groupOrder = append(groupOrder, p.ChoiceGroup)
		}
		groups[p.ChoiceGroup] = append(groups[p.ChoiceGroup], p)
	}
	sort.Ints(groupOrder)

	picked := make(map[int]string, len(selections))
	for _, sel := range selections {
		if _, ok := groups[sel.ChoiceGroup]; !ok {
			return nil, coreservice.ErrVolunteerChoiceInvalid
		}
		if prev, ok := picked[sel.ChoiceGroup]; ok && prev != sel.ProductID {
			return nil, coreservice.ErrVolunteerChoiceInvalid
		}
		picked[sel.ChoiceGroup] = sel.ProductID
	}

	var pending []coreservice.VolunteerChoiceGroup
	for _, g := range groupOrder {
		options := groups[g]
		productID, chosen := picked[g]
		if !chosen {
			if len(options) == 1 {
				out = append(out, options[0])
				continue
			}
			pending = append(pending, coreservice.VolunteerChoiceGroup{ChoiceGroup: g, Options: options})
			continue
		}
		idx := -1
		for i, o := range options {
			if o.ProductID == productID {
				idx = i
				break
			}
		}
		if idx < 0 {
			return nil, coreservice.ErrVolunteerChoiceInvalid
		}
		out = append(out, options[idx])
	}
	if len(pending) > 0 {
		return nil, &coreservice.VolunteerChoiceRequiredError{Groups: pending}
	}
	return out, nil
}

func (s *service) RedeemSharedQR(ctx context.Context, claimToken string, stationID string, idempotencyKey string, selections []coreservice.VolunteerChoiceSelection) (*coreservice.VolunteerRedemptionResult, error) {
	campaign, err := s.campaigns.GetByClaimToken(ctx, claimToken)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, coreservice.ErrVolunteerCampaignNotFound
		}
		return nil, err
	}

	if idempotencyKey != "" {
		if prev, err := s.redemptions.GetByIdempotencyKey(ctx, campaign.ID, idempotencyKey); err == nil && prev != nil {
			stationResp, redeemErr := s.stations.RedeemAssigned(ctx, stationID, prev.OrderID, idempotencyKey)
			if redeemErr != nil {
				return nil, redeemErr
			}
			return &coreservice.VolunteerRedemptionResult{
				OrderID:         prev.OrderID,
				RedemptionCount: campaign.RedemptionCount,
				MaxRedemptions:  campaign.MaxRedemptions,
				StationResult:   stationResp,
			}, nil
		}
	}

	if campaign.Status != volunteercampaign.StatusActive {
		return nil, coreservice.ErrVolunteerCampaignInactive
	}
	if !campaignWithinValidity(campaign, time.Now()) {
		return nil, coreservice.ErrVolunteerCampaignOutsideValid
	}

	cps, err := s.campaigns.ListProducts(ctx, campaign.ID)
	if err != nil {
		return nil, err
	}
	if len(cps) == 0 {
		return nil, coreservice.ErrVolunteerCampaignHasNoProducts
	}
	views := campaignProductsToViews(cps)

	stationProductIDs, err := s.stations.ListStationProductIDs(ctx, stationID)
	if err != nil {
		return nil, fmt.Errorf("list station products: %w", err)
	}
	stationServes := make(map[string]struct{}, len(stationProductIDs))
	for _, pid := range stationProductIDs {
		stationServes[pid] = struct{}{}
	}

	// Checked before the choice is resolved: a station serving nothing from this
	// campaign can never fulfil it, so it must not send the operator through a
	// picker only to reject the answer.
	if !servesAny(stationServes, views) {
		return nil, coreservice.ErrVolunteerStationNoMatchingProducts
	}

	selected, err := resolveCampaignSelection(views, selections)
	if err != nil {
		return nil, err
	}
	products := make([]productSnapshot, 0, len(selected))
	for _, v := range selected {
		qty := v.Quantity
		if qty <= 0 {
			qty = 1
		}
		products = append(products, productSnapshot{
			ID:       v.ProductID,
			Name:     v.ProductName,
			Quantity: qty,
		})
	}
	if !servesAny(stationServes, selected) {
		return nil, coreservice.ErrVolunteerStationNoMatchingProducts
	}

	incremented, err := s.campaigns.IncrementRedemptionAtomic(ctx, campaign.ID)
	if err != nil {
		return nil, err
	}
	if !incremented {
		return nil, coreservice.ErrVolunteerMaxRedemptionsReached
	}

	// Atomicity comes from the outer per-request tenant tx; a nested Ent tx
	// here would bypass the request's app.tenant_id GUC and be blocked by RLS.
	orderID, err := s.createGratisOrder(ctx, products)
	if err != nil {
		return nil, err
	}

	var idemPtr *string
	if idempotencyKey != "" {
		idemPtr = &idempotencyKey
	}
	var stationPtr *string
	if stationID != "" {
		stationPtr = &stationID
	}
	if _, err := s.redemptions.Create(ctx, campaign.ID, orderID, stationPtr, idemPtr); err != nil {
		return nil, fmt.Errorf("record redemption: %w", err)
	}

	stationResp, err := s.stations.RedeemAssigned(ctx, stationID, orderID, idempotencyKey)
	if err != nil {
		return nil, err
	}

	return &coreservice.VolunteerRedemptionResult{
		OrderID:         orderID,
		RedemptionCount: campaign.RedemptionCount + 1,
		MaxRedemptions:  campaign.MaxRedemptions,
		StationResult:   stationResp,
	}, nil
}

func campaignWithinValidity(c *ent.VolunteerCampaign, now time.Time) bool {
	if c.ValidFrom != nil && now.Before(*c.ValidFrom) {
		return false
	}
	if c.ValidUntil != nil && now.After(*c.ValidUntil) {
		return false
	}
	return true
}

// campaignProductsToViews orders standalone products first, then each choice
// group together, so every surface renders the same "A oder B" structure.
func campaignProductsToViews(cps []*ent.VolunteerCampaignProduct) []coreservice.VolunteerCampaignProductView {
	out := make([]coreservice.VolunteerCampaignProductView, 0, len(cps))
	for _, cp := range cps {
		name := ""
		var image *string
		if p, _ := cp.Edges.ProductOrErr(); p != nil {
			name = p.Name
			image = p.Image
		}
		out = append(out, coreservice.VolunteerCampaignProductView{
			ProductID:    cp.ProductID,
			ProductName:  name,
			ProductImage: image,
			Quantity:     cp.Quantity,
			ChoiceGroup:  cp.ChoiceGroup,
			OptionGroup:  cp.OptionGroup,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ChoiceGroup != out[j].ChoiceGroup {
			return out[i].ChoiceGroup < out[j].ChoiceGroup
		}
		if out[i].OptionGroup != out[j].OptionGroup {
			return out[i].OptionGroup < out[j].OptionGroup
		}
		return out[i].ProductName < out[j].ProductName
	})
	return out
}
