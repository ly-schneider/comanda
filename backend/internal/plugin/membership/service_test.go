package membership

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"testing"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/orderline"
	"backend/internal/plugin"
	"backend/internal/repository"
	coreservice "backend/internal/service"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubPlugins struct {
	plugin.Service
	config  json.RawMessage
	secrets map[string]string
}

func (s *stubPlugins) Config(context.Context, string) (json.RawMessage, error) {
	if len(s.config) == 0 {
		return json.RawMessage("{}"), nil
	}
	return s.config, nil
}

func (s *stubPlugins) SetConfig(_ context.Context, _ string, cfg json.RawMessage) (json.RawMessage, error) {
	s.config = cfg
	return cfg, nil
}

func (s *stubPlugins) Secrets(context.Context, string) (map[string]string, error) {
	if s.secrets == nil {
		return map[string]string{"baseUrl": "https://provider.example/v1", "apiKey": "k"}, nil
	}
	return s.secrets, nil
}

type fakeProvider struct {
	lookup      Decision
	lookupErr   error
	redeem      Decision
	redeemErr   error
	void        Decision
	voidErr     error
	catalog     Decision
	catalogErr  error
	redeemed    []RedeemRequest
	voided      []string
	lookupSeen  []string
	catalogSeen [][]CatalogItem
	freeSeen    [][]string
}

func (f *fakeProvider) Lookup(_ context.Context, _ Credentials, code string) (Decision, error) {
	f.lookupSeen = append(f.lookupSeen, code)
	return f.lookup, f.lookupErr
}

func (f *fakeProvider) Redeem(_ context.Context, _ Credentials, in RedeemRequest) (Decision, error) {
	f.redeemed = append(f.redeemed, in)
	return f.redeem, f.redeemErr
}

func (f *fakeProvider) Void(_ context.Context, _ Credentials, reference string) (Decision, error) {
	f.voided = append(f.voided, reference)
	return f.void, f.voidErr
}

func (f *fakeProvider) Catalog(_ context.Context, _ Credentials, items []CatalogItem) (Decision, error) {
	f.catalogSeen = append(f.catalogSeen, items)
	return f.catalog, f.catalogErr
}

func (f *fakeProvider) SetFree(_ context.Context, _ Credentials, skus []string) (Decision, error) {
	f.freeSeen = append(f.freeSeen, skus)
	return f.catalog, f.catalogErr
}

type fakeRedemptions struct {
	rows      []*ent.MembershipRedemption
	created   []*ent.MembershipRedemption
	released  []string
	createErr error
}

func (f *fakeRedemptions) Create(_ context.Context, memberRef, memberName, orderID string, quantity int) (*ent.MembershipRedemption, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	row := &ent.MembershipRedemption{ID: "r1", MemberRef: memberRef, MemberName: memberName, OrderID: orderID, Quantity: quantity}
	f.created = append(f.created, row)
	return row, nil
}

func (f *fakeRedemptions) GetByOrderID(context.Context, string) ([]*ent.MembershipRedemption, error) {
	return f.rows, nil
}

func (f *fakeRedemptions) MarkReleased(_ context.Context, id string, _ time.Time) error {
	f.released = append(f.released, id)
	return nil
}

type fakeProducts struct {
	existing map[string]*ent.Product
	buyable  map[string]bool
}

func (f fakeProducts) GetByIDs(_ context.Context, ids []string) ([]*ent.Product, error) {
	out := []*ent.Product{}
	for _, id := range ids {
		if p, ok := f.existing[id]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f fakeProducts) GetAllVisible(_ context.Context) ([]*ent.Product, error) {
	out := []*ent.Product{}
	for _, p := range f.existing {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f fakeProducts) FilterBuyable(_ context.Context, ids []string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	for _, id := range ids {
		if f.buyable == nil || f.buyable[id] {
			out[id] = struct{}{}
		}
	}
	return out, nil
}

type fakeStock struct{ stock map[string]int }

func (f fakeStock) GetCurrentStockBatch(_ context.Context, ids []string) (map[string]int, error) {
	out := map[string]int{}
	for _, id := range ids {
		out[id] = f.stock[id]
	}
	return out, nil
}

type fakeOrderLines struct {
	repository.OrderLineRepository
	lines []*ent.OrderLine
}

func (f fakeOrderLines) GetByOrderID(context.Context, string) ([]*ent.OrderLine, error) {
	return f.lines, nil
}

type fakeMinter struct {
	prep *coreservice.CheckoutPreparation
	err  error
	in   coreservice.CreateCheckoutInput
}

func (f *fakeMinter) PrepareAndCreateOrder(_ context.Context, in coreservice.CreateCheckoutInput, _ *string, _ *string) (*coreservice.CheckoutPreparation, error) {
	f.in = in
	if f.err != nil {
		return nil, f.err
	}
	return f.prep, nil
}

type fakeSettler struct {
	settled []string
	err     error
}

func (f *fakeSettler) SetPosPaymentGratisPlugin(_ context.Context, orderID string, _ *string, _ int64, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.settled = append(f.settled, orderID)
	return nil
}

type harness struct {
	svc         Service
	provider    *fakeProvider
	redemptions *fakeRedemptions
	minter      *fakeMinter
	settler     *fakeSettler
	plugins     *stubPlugins
}

func newHarness(t *testing.T, cfg string, lines []*ent.OrderLine) *harness {
	t.Helper()
	h := &harness{
		provider:    &fakeProvider{},
		redemptions: &fakeRedemptions{},
		minter:      &fakeMinter{prep: &coreservice.CheckoutPreparation{OrderID: "ord1", QRPayload: "qr", TotalCents: 900}},
		settler:     &fakeSettler{},
		plugins:     &stubPlugins{config: json.RawMessage(cfg)},
	}
	products := fakeProducts{existing: map[string]*ent.Product{
		"burger": {ID: "burger", Name: "Chicken Burger"},
		"fries":  {ID: "fries", Name: "Pommes"},
	}}
	stock := fakeStock{stock: map[string]int{"burger": 5, "fries": 5}}
	h.svc = NewService(h.plugins, h.redemptions, h.provider, products, stock,
		fakeOrderLines{lines: lines}, h.minter, h.settler, zap.NewNop())
	return h
}

func line(id, productID, title string, qty int, price int64, lineType orderline.LineType, parent *string) *ent.OrderLine {
	return &ent.OrderLine{
		ID:             id,
		ProductID:      productID,
		Title:          title,
		Quantity:       qty,
		UnitPriceCents: price,
		LineType:       lineType,
		ParentLineID:   parent,
	}
}

func okLookup() Decision {
	return Decision{
		Status:      StatusOK,
		Message:     "100er Club: noch 1 Artikel",
		Member:      &Member{Ref: "BT42", Name: "Eva Muster", GroupName: "100er Club"},
		Entitlement: &Entitlement{Remaining: 1, Max: 1, Period: "Monat"},
	}
}

func TestValidateOrderClaimTakesTheDearestEligibleUnits(t *testing.T) {
	parent := "l1"
	h := newHarness(t, `{"freeProductIds":["burger","fries"]}`, []*ent.OrderLine{
		line("l1", "fries", "Pommes", 2, 500, orderline.LineTypeSimple, nil),
		line("l2", "burger", "Chicken Burger", 1, 900, orderline.LineTypeSimple, nil),
		line("l3", "cup", "Depot", 1, 200, orderline.LineTypeDeposit, &parent),
	})

	items, err := h.svc.ValidateOrderClaim(context.Background(), "ord1", 1)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, RedeemItem{SKU: "burger", Name: "Chicken Burger", Quantity: 1}, items[0])
}

func TestValidateOrderClaimRejectsMoreUnitsThanTheOrderHolds(t *testing.T) {
	h := newHarness(t, `{"freeProductIds":["burger"]}`, []*ent.OrderLine{
		line("l1", "burger", "Chicken Burger", 1, 900, orderline.LineTypeSimple, nil),
		line("l2", "fries", "Pommes", 3, 500, orderline.LineTypeSimple, nil),
	})

	_, err := h.svc.ValidateOrderClaim(context.Background(), "ord1", 2)
	require.ErrorIs(t, err, ErrProductNotFree)
}

func TestValidateOrderClaimNeedsConfiguredProducts(t *testing.T) {
	h := newHarness(t, `{}`, []*ent.OrderLine{
		line("l1", "burger", "Chicken Burger", 1, 900, orderline.LineTypeSimple, nil),
	})

	_, err := h.svc.ValidateOrderClaim(context.Background(), "ord1", 1)
	require.ErrorIs(t, err, ErrProductNotFree)
}

func TestBookSendsTheOrderIdAsReferenceAndRecordsIt(t *testing.T) {
	h := newHarness(t, `{"freeProductIds":["burger"]}`, nil)
	h.provider.redeem = Decision{Status: StatusOK}

	err := h.svc.Book(context.Background(), "ord1", "BT42", "Eva Muster", 1,
		[]RedeemItem{{SKU: "burger", Name: "Chicken Burger", Quantity: 1}})
	require.NoError(t, err)

	require.Len(t, h.provider.redeemed, 1)
	require.Equal(t, "ord1", h.provider.redeemed[0].Reference)
	require.Equal(t, "BT42", h.provider.redeemed[0].MemberRef)
	require.Equal(t, 1, h.provider.redeemed[0].Quantity)
	require.Len(t, h.redemptions.created, 1)
	require.Empty(t, h.provider.voided)
}

func TestBookRefusedByProviderRecordsNothing(t *testing.T) {
	h := newHarness(t, `{"freeProductIds":["burger"]}`, nil)
	h.provider.redeem = Decision{Status: StatusExhausted, Message: "Kontingent aufgebraucht"}

	err := h.svc.Book(context.Background(), "ord1", "BT42", "Eva", 1, nil)
	require.ErrorIs(t, err, ErrNotEligible)
	require.Empty(t, h.redemptions.created)
	// A refusal is an answer, not a failed call: nothing to give back.
	require.Empty(t, h.provider.voided)
}

func TestBookReleasesWhenTheProviderNeverAnswers(t *testing.T) {
	h := newHarness(t, `{"freeProductIds":["burger"]}`, nil)
	h.provider.redeemErr = ErrProviderUnavailable
	h.provider.void = Decision{Status: StatusOK}

	err := h.svc.Book(context.Background(), "ord1", "BT42", "Eva", 1, nil)
	require.ErrorIs(t, err, ErrProviderUnavailable)
	require.Empty(t, h.redemptions.created)
	// The call may still have booked, and the order is about to roll back.
	require.Equal(t, []string{"ord1"}, h.provider.voided)
}

func TestBookReleasesWhenTheRowCannotBeWritten(t *testing.T) {
	h := newHarness(t, `{"freeProductIds":["burger"]}`, nil)
	h.provider.redeem = Decision{Status: StatusOK}
	h.redemptions.createErr = errors.New("db is down")
	h.provider.void = Decision{Status: StatusOK}

	err := h.svc.Book(context.Background(), "ord1", "BT42", "Eva", 1, nil)
	require.Error(t, err)
	require.Equal(t, []string{"ord1"}, h.provider.voided)
}

func TestReleaseWaitsUntilTheCompedUnitsCameBack(t *testing.T) {
	h := newHarness(t, `{}`, nil)
	h.redemptions.rows = []*ent.MembershipRedemption{{ID: "r1", OrderID: "ord1", Quantity: 2}}
	h.provider.void = Decision{Status: StatusOK}
	ord := &ent.Order{ID: "ord1"}

	err := h.svc.Release(context.Background(), ord, plugin.RefundedOrder{RefundedUnits: map[string]int{"l1": 1}})
	require.NoError(t, err)
	require.Empty(t, h.provider.voided, "one of two comped units is not the comped item back")

	err = h.svc.Release(context.Background(), ord, plugin.RefundedOrder{RefundedUnits: map[string]int{"l1": 2}})
	require.NoError(t, err)
	require.Equal(t, []string{"ord1"}, h.provider.voided)
	require.Equal(t, []string{"r1"}, h.redemptions.released)
}

func TestReleaseOnCancellationIgnoresUnitCounts(t *testing.T) {
	h := newHarness(t, `{}`, nil)
	h.redemptions.rows = []*ent.MembershipRedemption{{ID: "r1", OrderID: "ord1", Quantity: 2}}
	h.provider.void = Decision{Status: StatusOK}

	err := h.svc.Release(context.Background(), &ent.Order{ID: "ord1"}, plugin.RefundedOrder{Cancelled: true})
	require.NoError(t, err)
	require.Equal(t, []string{"ord1"}, h.provider.voided)
}

func TestReleaseSkipsWhatWasAlreadyGivenBack(t *testing.T) {
	h := newHarness(t, `{}`, nil)
	released := time.Now()
	h.redemptions.rows = []*ent.MembershipRedemption{{ID: "r1", OrderID: "ord1", Quantity: 1, ReleasedAt: &released}}

	err := h.svc.Release(context.Background(), &ent.Order{ID: "ord1"}, plugin.RefundedOrder{Cancelled: true})
	require.NoError(t, err)
	require.Empty(t, h.provider.voided)
}

func TestReleaseKeepsTheRowOpenWhenTheProviderIsUnreachable(t *testing.T) {
	h := newHarness(t, `{}`, nil)
	h.redemptions.rows = []*ent.MembershipRedemption{{ID: "r1", OrderID: "ord1", Quantity: 1}}
	h.provider.voidErr = ErrProviderUnavailable

	err := h.svc.Release(context.Background(), &ent.Order{ID: "ord1"}, plugin.RefundedOrder{Cancelled: true})
	require.ErrorIs(t, err, ErrProviderUnavailable)
	require.Empty(t, h.redemptions.released, "an unconfirmed release must be retried")
}

func TestClaimViewHidesTheMemberReferenceAndSoldOutProducts(t *testing.T) {
	h := newHarness(t, `{"label":"100er Club","freeProductIds":["burger","fries"]}`, nil)
	h.provider.lookup = okLookup()
	h.svc.(*service).stock = fakeStock{stock: map[string]int{"burger": 3, "fries": 0}}

	view, err := h.svc.ClaimView(context.Background(), "card-code")
	require.NoError(t, err)
	require.Equal(t, "100er Club", view.Label)
	require.Equal(t, "Eva Muster", view.Lookup.MemberName)
	require.Empty(t, view.Lookup.MemberRef, "the guest's phone never sees the provider's handle")
	require.Len(t, view.Products, 1)
	require.Equal(t, "burger", view.Products[0].ID)
}

func TestClaimViewOffersNothingWhenTheBudgetIsUsedUp(t *testing.T) {
	h := newHarness(t, `{"freeProductIds":["burger"]}`, nil)
	h.provider.lookup = Decision{
		Status:      StatusExhausted,
		Message:     "Kontingent aufgebraucht",
		Member:      &Member{Ref: "BT42", Name: "Eva Muster"},
		Entitlement: &Entitlement{Remaining: 0, Max: 1},
	}

	view, err := h.svc.ClaimView(context.Background(), "card-code")
	require.NoError(t, err)
	require.False(t, view.Lookup.CanRedeem())
	require.Empty(t, view.Products)
}

func TestClaimMintsSettlesAndBooks(t *testing.T) {
	h := newHarness(t, `{"freeProductIds":["burger"]}`, nil)
	h.provider.lookup = okLookup()
	h.provider.redeem = Decision{Status: StatusOK}

	result, err := h.svc.Claim(context.Background(), "card-code", []ClaimItem{{ProductID: "burger", Quantity: 1}})
	require.NoError(t, err)
	require.Equal(t, "ord1", result.OrderID)
	require.Equal(t, "qr", result.QRPayload)
	require.Empty(t, result.Member.MemberRef)

	require.Equal(t, []string{"ord1"}, h.settler.settled)
	require.Len(t, h.minter.in.Items, 1)
	require.Equal(t, "burger", h.minter.in.Items[0].ProductID)
	require.Len(t, h.provider.redeemed, 1)
	require.Equal(t, "ord1", h.provider.redeemed[0].Reference)
}

func TestClaimRefusesProductsTheTenantDoesNotGiveAway(t *testing.T) {
	h := newHarness(t, `{"freeProductIds":["burger"]}`, nil)
	h.provider.lookup = okLookup()

	_, err := h.svc.Claim(context.Background(), "card-code", []ClaimItem{{ProductID: "fries", Quantity: 1}})
	require.ErrorIs(t, err, ErrProductNotFree)
	require.Empty(t, h.provider.redeemed)
	require.Empty(t, h.settler.settled)
}

func TestClaimRefusesMoreThanTheBudget(t *testing.T) {
	h := newHarness(t, `{"freeProductIds":["burger"]}`, nil)
	h.provider.lookup = okLookup()

	_, err := h.svc.Claim(context.Background(), "card-code", []ClaimItem{{ProductID: "burger", Quantity: 2}})
	require.ErrorIs(t, err, ErrNotEligible)
	require.Empty(t, h.minter.in.Items)
}

func TestSettingsKeepOnlyProductsThatStillExist(t *testing.T) {
	h := newHarness(t, `{"freeProductIds":["burger","gone"]}`, nil)
	// The provider still names a product this tenant has deleted since.
	h.provider.catalog = Decision{Status: StatusOK, Program: &Program{Label: defaultLabel, FreeSkus: []string{"burger", "gone"}}}

	settings, err := h.svc.GetSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"burger"}, settings.FreeProductIDs)
	require.Equal(t, defaultLabel, settings.Label)
	require.True(t, settings.Synced)
}

func TestSettingsReportTheCatalogAndTakeTheAnswer(t *testing.T) {
	h := newHarness(t, `{}`, nil)
	h.provider.catalog = Decision{Status: StatusOK, Program: &Program{Label: "100er Club", FreeSkus: []string{"burger"}}}

	settings, err := h.svc.GetSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, "100er Club", settings.Label)
	require.Equal(t, []string{"burger"}, settings.FreeProductIDs)

	// What this tenant sells travelled, so the member system can show names.
	require.Len(t, h.provider.catalogSeen, 1)
	require.Equal(t, "burger", h.provider.catalogSeen[0][0].SKU)
	require.Equal(t, "Chicken Burger", h.provider.catalogSeen[0][0].Name)

	var stored config
	require.NoError(t, json.Unmarshal(h.plugins.config, &stored))
	require.Equal(t, "100er Club", stored.Label)
	require.Equal(t, []string{"burger"}, stored.FreeProductIDs)
}

func TestSettingsSurviveAnUnreachableProvider(t *testing.T) {
	h := newHarness(t, `{"label":"100er Club","freeProductIds":["burger"]}`, nil)
	h.provider.catalogErr = ErrProviderUnavailable

	settings, err := h.svc.GetSettings(context.Background())
	// The screen shows what was last agreed, and says it is not in sync --
	// better than an error page in front of a stand that is open.
	require.NoError(t, err)
	require.False(t, settings.Synced)
	require.Equal(t, []string{"burger"}, settings.FreeProductIDs)
}

func TestSetSettingsHandsTheSelectionToTheProvider(t *testing.T) {
	h := newHarness(t, `{}`, nil)
	h.provider.catalog = Decision{Status: StatusOK, Program: &Program{Label: "100er Club", FreeSkus: []string{"burger"}}}

	settings, err := h.svc.SetSettings(context.Background(), []string{"burger", ""})
	require.NoError(t, err)
	require.Equal(t, []string{"burger"}, settings.FreeProductIDs)
	// Empty ids never travel, and what comes back wins over what was sent.
	require.Equal(t, [][]string{{"burger"}}, h.provider.freeSeen)

	var stored config
	require.NoError(t, json.Unmarshal(h.plugins.config, &stored))
	require.Equal(t, "100er Club", stored.Label)
}

func TestAnOrdinaryScanRefreshesTheOffer(t *testing.T) {
	h := newHarness(t, `{"label":"alt","freeProductIds":["gone"]}`, nil)
	lookup := okLookup()
	lookup.Program = &Program{Label: "100er Club", FreeSkus: []string{"burger"}}
	h.provider.lookup = lookup

	_, err := h.svc.Lookup(context.Background(), "card-code")
	require.NoError(t, err)

	var stored config
	require.NoError(t, json.Unmarshal(h.plugins.config, &stored))
	require.Equal(t, "100er Club", stored.Label)
	require.Equal(t, []string{"burger"}, stored.FreeProductIDs)
}
