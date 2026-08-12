package api

import (
	"encoding/json"
	"net/http"
	"time"

	"backend/internal/response"
	"backend/internal/service"

	nanoid "backend/internal/id"
	"go.uber.org/zap"
)

const (
	volunteerSessionCookie = "hsclaim_session"
)

type verifyAccessRequest struct {
	Code string `json:"code"`
}

type claimProductItem struct {
	ProductID    string  `json:"productId"`
	ProductName  string  `json:"productName"`
	ProductImage *string `json:"productImage,omitempty"`
	Quantity     int     `json:"quantity"`
	ChoiceGroup  int     `json:"choiceGroup"`
}

type claimCampaignPublic struct {
	Name       string     `json:"name"`
	ValidFrom  *time.Time `json:"validFrom,omitempty"`
	ValidUntil *time.Time `json:"validUntil,omitempty"`
	Status     string     `json:"status"`
}

type claimCampaignResponse struct {
	Campaign  claimCampaignPublic `json:"campaign"`
	Products  []claimProductItem  `json:"products"`
	QRPayload string              `json:"qrPayload"`
}

func (h *Handlers) VerifyClaimAccess(w http.ResponseWriter, r *http.Request, token string) {
	if !nanoid.Valid(token) {
		writeError(w, http.StatusBadRequest, "invalid_token", "Invalid token")
		return
	}
	var req verifyAccessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	existing := readSessionCookie(r)
	sessionID, err := h.claims.VerifyAccess(r.Context(), token, req.Code)
	if err != nil {
		h.writeVolunteerError(w, err)
		return
	}
	if existing != "" {
		sessionID = existing
	}
	writeSessionCookie(w, r, sessionID)
	response.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handlers) GetClaimCampaign(w http.ResponseWriter, r *http.Request, token string) {
	if !nanoid.Valid(token) {
		writeError(w, http.StatusBadRequest, "invalid_token", "Invalid token")
		return
	}
	if readSessionCookie(r) == "" {
		writeError(w, http.StatusUnauthorized, "auth_required", "Access code required.")
		return
	}
	view, err := h.claims.GetClaimView(r.Context(), token)
	if err != nil {
		h.writeVolunteerError(w, err)
		return
	}
	products := make([]claimProductItem, 0, len(view.Products))
	for _, p := range view.Products {
		products = append(products, claimProductItem{
			ProductID:    p.ProductID,
			ProductName:  p.ProductName,
			ProductImage: p.ProductImage,
			Quantity:     p.Quantity,
			ChoiceGroup:  p.ChoiceGroup,
		})
	}
	response.WriteJSON(w, http.StatusOK, claimCampaignResponse{
		Campaign: claimCampaignPublic{
			Name:       view.Campaign.Name,
			ValidFrom:  view.Campaign.ValidFrom,
			ValidUntil: view.Campaign.ValidUntil,
			Status:     string(view.Campaign.Status),
		},
		Products:  products,
		QRPayload: view.QRPayload,
	})
}

func (h *Handlers) writeVolunteerError(w http.ResponseWriter, err error) {
	if status, code, msg, ok := service.VolunteerErrorResponse(err); ok {
		writeError(w, status, code, msg)
		return
	}
	h.logger.Error("volunteer error", zap.Error(err))
	writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
}

func readSessionCookie(r *http.Request) string {
	c, err := r.Cookie(volunteerSessionCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

func writeSessionCookie(w http.ResponseWriter, r *http.Request, sessionID string) {
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     volunteerSessionCookie,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((24 * time.Hour).Seconds()),
	})
}
