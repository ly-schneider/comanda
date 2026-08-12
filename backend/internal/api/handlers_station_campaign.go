package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"backend/internal/auth"
	"backend/internal/response"
	"backend/internal/service"

	nanoid "backend/internal/id"
)

type redeemCampaignRequest struct {
	ClaimToken string                    `json:"claimToken"`
	Selections []redeemCampaignSelection `json:"selections,omitempty"`
}

type redeemCampaignSelection struct {
	ChoiceGroup int    `json:"choiceGroup"`
	ProductID   string `json:"productId"`
}

type campaignChoiceOption struct {
	ProductID    string  `json:"productId"`
	ProductName  string  `json:"productName"`
	ProductImage *string `json:"productImage,omitempty"`
	Quantity     int     `json:"quantity"`
}

type campaignChoiceGroup struct {
	ChoiceGroup int                    `json:"choiceGroup"`
	Options     []campaignChoiceOption `json:"options"`
}

// campaignChoiceRequiredResponse extends the standard error body with the groups
// the station has to resolve before re-sending the redemption.
type campaignChoiceRequiredResponse struct {
	Code    string                `json:"code"`
	Message string                `json:"message"`
	Groups  []campaignChoiceGroup `json:"groups"`
}

// RedeemCampaignAtStation redeems a shared campaign QR at the current station.
// POST /v1/stations/redeem-campaign
func (h *Handlers) RedeemCampaignAtStation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	deviceID, ok := auth.GetDeviceID(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Device authentication required")
		return
	}

	var body redeemCampaignRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	token := body.ClaimToken
	if !nanoid.Valid(token) {
		writeError(w, http.StatusBadRequest, "invalid_claim_token", "Invalid claim token")
		return
	}

	idemKey := r.Header.Get("Idempotency-Key")

	selections := make([]service.VolunteerChoiceSelection, 0, len(body.Selections))
	for _, sel := range body.Selections {
		if !nanoid.Valid(sel.ProductID) {
			writeError(w, http.StatusBadRequest, "invalid_selection", "Invalid product selection")
			return
		}
		selections = append(selections, service.VolunteerChoiceSelection(sel))
	}

	result, err := h.claims.RedeemSharedQR(ctx, token, deviceID, idemKey, selections)
	if err != nil {
		var choiceErr *service.VolunteerChoiceRequiredError
		if errors.As(err, &choiceErr) {
			writeChoiceRequired(w, choiceErr)
			return
		}
		if errors.Is(err, service.ErrVolunteerCampaignNotFound) ||
			errors.Is(err, service.ErrVolunteerCampaignInactive) ||
			errors.Is(err, service.ErrVolunteerCampaignOutsideValid) ||
			errors.Is(err, service.ErrVolunteerMaxRedemptionsReached) ||
			errors.Is(err, service.ErrVolunteerCampaignHasNoProducts) ||
			errors.Is(err, service.ErrVolunteerStationNoMatchingProducts) ||
			errors.Is(err, service.ErrVolunteerChoiceInvalid) {
			h.writeVolunteerError(w, err)
			return
		}
		writeError(w, http.StatusBadRequest, "redeem_failed", err.Error())
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]any{
		"orderId":         result.OrderID,
		"redemptionCount": result.RedemptionCount,
		"maxRedemptions":  result.MaxRedemptions,
		"station":         result.StationResult,
	})
}

func writeChoiceRequired(w http.ResponseWriter, err *service.VolunteerChoiceRequiredError) {
	status, code, message, _ := service.VolunteerErrorResponse(err)
	groups := make([]campaignChoiceGroup, 0, len(err.Groups))
	for _, g := range err.Groups {
		options := make([]campaignChoiceOption, 0, len(g.Options))
		for _, o := range g.Options {
			options = append(options, campaignChoiceOption{
				ProductID:    o.ProductID,
				ProductName:  o.ProductName,
				ProductImage: o.ProductImage,
				Quantity:     o.Quantity,
			})
		}
		groups = append(groups, campaignChoiceGroup{ChoiceGroup: g.ChoiceGroup, Options: options})
	}
	response.WriteJSON(w, status, campaignChoiceRequiredResponse{Code: code, Message: message, Groups: groups})
}
