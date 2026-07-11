package staffperks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/volunteercampaign"
	"backend/internal/pdf"
	"backend/internal/response"
	coreservice "backend/internal/service"

	nanoid "backend/internal/id"
	"go.uber.org/zap"
)

const (
	printSlipDefaultCount = 30
	printSlipMinCount     = 1
	printSlipMaxCount     = 500
)

type handlers struct {
	generated.Unimplemented
	rt *runtime
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	response.WriteJSON(w, status, generated.Error{Code: code, Message: message})
}

type volunteerCampaignProductPayload struct {
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
}

type createVolunteerCampaignRequest struct {
	Name           string                            `json:"name"`
	ValidFrom      *time.Time                        `json:"validFrom,omitempty"`
	ValidUntil     *time.Time                        `json:"validUntil,omitempty"`
	Products       []volunteerCampaignProductPayload `json:"products"`
	MaxRedemptions int                               `json:"maxRedemptions"`
}

type updateVolunteerCampaignRequest struct {
	Name           string                            `json:"name"`
	ValidFrom      *time.Time                        `json:"validFrom,omitempty"`
	ValidUntil     *time.Time                        `json:"validUntil,omitempty"`
	Status         string                            `json:"status"`
	Products       []volunteerCampaignProductPayload `json:"products,omitempty"`
	MaxRedemptions *int                              `json:"maxRedemptions,omitempty"`
}

type adminCampaignResponse struct {
	ID              string     `json:"id"`
	ClaimToken      string     `json:"claimToken"`
	Name            string     `json:"name"`
	AccessCode      string     `json:"accessCode"`
	ValidFrom       *time.Time `json:"validFrom,omitempty"`
	ValidUntil      *time.Time `json:"validUntil,omitempty"`
	Status          string     `json:"status"`
	MaxRedemptions  int        `json:"maxRedemptions"`
	RedemptionCount int        `json:"redemptionCount"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type adminCampaignDetailResponse struct {
	adminCampaignResponse
	Products    []adminCampaignProductItem    `json:"products"`
	Redemptions []adminCampaignRedemptionItem `json:"redemptions"`
}

type adminCampaignProductItem struct {
	ProductID    string  `json:"productId"`
	ProductName  string  `json:"productName"`
	ProductImage *string `json:"productImage,omitempty"`
	Quantity     int     `json:"quantity"`
}

type adminCampaignRedemptionItem struct {
	ID        string    `json:"id"`
	OrderID   string    `json:"orderId"`
	CreatedAt time.Time `json:"createdAt"`
}

func (h *handlers) CreateVolunteerCampaign(w http.ResponseWriter, r *http.Request) {
	var req createVolunteerCampaignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	products := make([]VolunteerCampaignProductInput, 0, len(req.Products))
	for _, p := range req.Products {
		products = append(products, VolunteerCampaignProductInput(p))
	}
	campaign, err := h.rt.svc.CreateCampaign(r.Context(), CreateCampaignInput{
		Name:           req.Name,
		ValidFrom:      req.ValidFrom,
		ValidUntil:     req.ValidUntil,
		Products:       products,
		MaxRedemptions: req.MaxRedemptions,
	})
	if err != nil {
		h.writeCampaignError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusCreated, campaignToAdminResponse(campaign))
}

func (h *handlers) ListVolunteerCampaigns(w http.ResponseWriter, r *http.Request) {
	campaigns, err := h.rt.svc.ListCampaigns(r.Context())
	if err != nil {
		h.rt.logger.Error("list volunteer campaigns", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	items := make([]adminCampaignResponse, 0, len(campaigns))
	for _, c := range campaigns {
		items = append(items, campaignToAdminResponse(c))
	}
	response.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *handlers) GetVolunteerCampaign(w http.ResponseWriter, r *http.Request, campaignId string) {
	id := campaignId
	if !nanoid.Valid(id) {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid id")
		return
	}
	detail, err := h.rt.svc.GetCampaign(r.Context(), id)
	if err != nil {
		h.writeCampaignError(w, err)
		return
	}
	products := make([]adminCampaignProductItem, 0, len(detail.Products))
	for _, cp := range detail.Products {
		products = append(products, adminCampaignProductItem{
			ProductID:    cp.ProductID,
			ProductName:  cp.ProductName,
			ProductImage: cp.ProductImage,
			Quantity:     cp.Quantity,
		})
	}
	redemptions := make([]adminCampaignRedemptionItem, 0, len(detail.Redemptions))
	for _, rv := range detail.Redemptions {
		redemptions = append(redemptions, adminCampaignRedemptionItem(rv))
	}
	resp := adminCampaignDetailResponse{
		adminCampaignResponse: campaignToAdminResponse(detail.Campaign),
		Products:              products,
		Redemptions:           redemptions,
	}
	response.WriteJSON(w, http.StatusOK, resp)
}

func (h *handlers) UpdateVolunteerCampaign(w http.ResponseWriter, r *http.Request, campaignId string) {
	id := campaignId
	if !nanoid.Valid(id) {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid id")
		return
	}
	var req updateVolunteerCampaignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	status := volunteercampaign.StatusActive
	if req.Status != "" {
		if err := volunteercampaign.StatusValidator(volunteercampaign.Status(req.Status)); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_status", err.Error())
			return
		}
		status = volunteercampaign.Status(req.Status)
	}
	products := make([]VolunteerCampaignProductInput, 0, len(req.Products))
	for _, p := range req.Products {
		products = append(products, VolunteerCampaignProductInput(p))
	}
	campaign, err := h.rt.svc.UpdateCampaign(r.Context(), id, UpdateCampaignInput{
		Name:           req.Name,
		ValidFrom:      req.ValidFrom,
		ValidUntil:     req.ValidUntil,
		Status:         status,
		Products:       products,
		MaxRedemptions: req.MaxRedemptions,
	})
	if err != nil {
		h.writeCampaignError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, campaignToAdminResponse(campaign))
}

func (h *handlers) EndVolunteerCampaign(w http.ResponseWriter, r *http.Request, campaignId string) {
	id := campaignId
	if !nanoid.Valid(id) {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid id")
		return
	}
	if err := h.rt.svc.EndCampaign(r.Context(), id); err != nil {
		h.writeCampaignError(w, err)
		return
	}
	response.WriteNoContent(w)
}

func (h *handlers) RotateVolunteerCampaignToken(w http.ResponseWriter, r *http.Request, campaignId string) {
	id := campaignId
	if !nanoid.Valid(id) {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid id")
		return
	}
	newToken, err := h.rt.svc.RotateClaimToken(r.Context(), id)
	if err != nil {
		h.writeCampaignError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, map[string]any{"claimToken": newToken})
}

// PrintStaffPerkSlips streams a PDF of printable QR slips for a campaign.
func (h *handlers) PrintStaffPerkSlips(w http.ResponseWriter, r *http.Request, campaignId string) {
	id := campaignId
	if !nanoid.Valid(id) {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid id")
		return
	}

	count := printSlipDefaultCount
	if v := r.URL.Query().Get("count"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < printSlipMinCount || n > printSlipMaxCount {
			writeError(w, http.StatusBadRequest, "invalid_count",
				fmt.Sprintf("count must be between %d and %d", printSlipMinCount, printSlipMaxCount))
			return
		}
		count = n
	}

	detail, err := h.rt.svc.GetCampaign(r.Context(), id)
	if err != nil {
		h.writeCampaignError(w, err)
		return
	}

	products := make([]pdf.SlipProduct, 0, len(detail.Products))
	for _, p := range detail.Products {
		products = append(products, pdf.SlipProduct{
			Name:     p.ProductName,
			Quantity: p.Quantity,
		})
	}

	body, err := pdf.RenderStaffPerkSlips(pdf.SlipInput{
		CampaignName: detail.Campaign.Name,
		QRPayload:    buildQRPayload(detail.Campaign.ClaimToken),
		Products:     products,
		Count:        count,
	})
	if err != nil {
		h.rt.logger.Error("render staff perk slips", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "render_failed", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="staff-perk-%s.pdf"`, detail.Campaign.ID))
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	_, _ = w.Write(body)
}

func (h *handlers) writeCampaignError(w http.ResponseWriter, err error) {
	if status, code, msg, ok := coreservice.VolunteerErrorResponse(err); ok {
		writeError(w, status, code, msg)
		return
	}
	h.rt.logger.Error("volunteer error", zap.Error(err))
	writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
}

func campaignToAdminResponse(c *ent.VolunteerCampaign) adminCampaignResponse {
	return adminCampaignResponse{
		ID:              c.ID,
		ClaimToken:      c.ClaimToken,
		Name:            c.Name,
		AccessCode:      c.AccessCode,
		ValidFrom:       c.ValidFrom,
		ValidUntil:      c.ValidUntil,
		Status:          string(c.Status),
		MaxRedemptions:  c.MaxRedemptions,
		RedemptionCount: c.RedemptionCount,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
	}
}
