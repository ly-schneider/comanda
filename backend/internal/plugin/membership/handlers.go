package membership

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"backend/internal/generated/api/generated"
	"backend/internal/response"
	coreservice "backend/internal/service"

	"go.uber.org/zap"
)

// handlers embeds generated.Unimplemented to satisfy the full ServerInterface;
// only the membership operationIds are real.
type handlers struct {
	generated.Unimplemented
	rt *runtime
}

func (h *handlers) svc() Service { return h.rt.svc }

func writeError(w http.ResponseWriter, status int, code, message string) {
	response.WriteJSON(w, status, generated.Error{Code: code, Message: message})
}

// writeProviderError keeps the provider's own wording out of the shop: a guest
// learns that it did not work, the log learns why.
func (h *handlers) writeProviderError(w http.ResponseWriter, err error) {
	h.rt.logger.Warn("membership provider call failed", zap.Error(err))
	writeError(w, http.StatusBadGateway, "provider_unavailable", "Mitgliederkarten sind gerade nicht erreichbar.")
}

func decode[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var body T
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return body, false
	}
	return body, true
}

func (h *handlers) MembershipLookup(w http.ResponseWriter, r *http.Request) {
	body, ok := decode[generated.MembershipLookupRequest](w, r)
	if !ok {
		return
	}
	code := strings.TrimSpace(body.Code)
	if code == "" {
		writeError(w, http.StatusBadRequest, "invalid_body", "Missing card content")
		return
	}

	look, err := h.svc().Lookup(r.Context(), code)
	if err != nil {
		h.writeProviderError(w, err)
		return
	}

	out := generated.MembershipLookupResult{
		Status:    generated.MembershipStatus(look.Status),
		CanRedeem: look.CanRedeem(),
		Message:   look.Message,
	}
	if look.MemberName != "" {
		out.Member = &generated.MembershipMember{Name: look.MemberName}
		if look.GroupName != "" {
			out.Member.GroupName = &look.GroupName
		}
	}
	if look.Max > 0 || look.Remaining > 0 {
		out.Entitlement = entitlementOf(look)
	}
	// Only a card that may redeem hands the till something to pay with.
	if look.CanRedeem() && look.MemberRef != "" {
		ref := look.MemberRef
		out.MemberRef = &ref
	}
	response.WriteJSON(w, http.StatusOK, out)
}

func (h *handlers) MembershipClaimView(w http.ResponseWriter, r *http.Request) {
	body, ok := decode[generated.MembershipLookupRequest](w, r)
	if !ok {
		return
	}
	code := strings.TrimSpace(body.Code)
	if code == "" {
		writeError(w, http.StatusBadRequest, "invalid_body", "Missing card content")
		return
	}

	view, err := h.svc().ClaimView(r.Context(), code)
	if err != nil {
		h.writeProviderError(w, err)
		return
	}

	products := make([]generated.MembershipClaimProduct, 0, len(view.Products))
	for _, p := range view.Products {
		products = append(products, generated.MembershipClaimProduct{Id: p.ID, Name: p.Name, Image: p.Image})
	}
	out := generated.MembershipClaimView{
		Status:    generated.MembershipStatus(view.Lookup.Status),
		CanRedeem: view.Lookup.CanRedeem() && len(products) > 0,
		Message:   view.Lookup.Message,
		Label:     view.Label,
		Products:  products,
	}
	if view.Lookup.MemberName != "" {
		out.Member = &generated.MembershipMember{Name: view.Lookup.MemberName}
		if view.Lookup.GroupName != "" {
			out.Member.GroupName = &view.Lookup.GroupName
		}
	}
	if view.Lookup.Max > 0 || view.Lookup.Remaining > 0 {
		out.Entitlement = entitlementOf(view.Lookup)
	}
	response.WriteJSON(w, http.StatusOK, out)
}

func (h *handlers) MembershipClaimRedeem(w http.ResponseWriter, r *http.Request) {
	body, ok := decode[generated.MembershipClaimRedeemRequest](w, r)
	if !ok {
		return
	}
	code := strings.TrimSpace(body.Code)
	if code == "" {
		writeError(w, http.StatusBadRequest, "invalid_body", "Missing card content")
		return
	}
	items := make([]ClaimItem, 0, len(body.Items))
	for _, it := range body.Items {
		items = append(items, ClaimItem{ProductID: it.ProductId, Quantity: it.Quantity})
	}

	result, err := h.svc().Claim(r.Context(), code, items)
	switch {
	case err == nil:
	case errors.Is(err, ErrNothingClaimed), errors.Is(err, ErrProductNotFree):
		writeError(w, http.StatusBadRequest, "not_claimable", "Das lässt sich nicht gratis beziehen.")
		return
	case errors.Is(err, ErrNotEligible):
		writeError(w, http.StatusConflict, "not_eligible", "Die Karte kann das gerade nicht einlösen.")
		return
	case errors.Is(err, ErrProviderUnavailable):
		h.writeProviderError(w, err)
		return
	default:
		var soldOut *coreservice.InsufficientInventoryError
		if errors.As(err, &soldOut) {
			writeError(w, http.StatusConflict, "sold_out", "Das ist leider ausverkauft.")
			return
		}
		h.rt.logger.Error("membership claim failed", zap.Error(err))
		writeError(w, http.StatusBadGateway, "claim_failed", "Das hat nicht geklappt. Bitte nochmals versuchen.")
		return
	}

	out := generated.MembershipClaimResult{
		OrderId:   result.OrderID,
		QrPayload: result.QRPayload,
		Message:   result.Member.Message,
	}
	if result.Member.MemberName != "" {
		out.Member = &generated.MembershipMember{Name: result.Member.MemberName}
		if result.Member.GroupName != "" {
			out.Member.GroupName = &result.Member.GroupName
		}
	}
	if result.Member.Max > 0 {
		out.Entitlement = entitlementOf(result.Member)
	}
	response.WriteJSON(w, http.StatusOK, out)
}

func (h *handlers) MembershipGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.svc().GetSettings(r.Context())
	if err != nil {
		h.rt.logger.Error("membership settings read failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "membership_error", "Settings could not be read")
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPISettings(settings))
}

func (h *handlers) MembershipUpdateSettings(w http.ResponseWriter, r *http.Request) {
	body, ok := decode[generated.MembershipSettingsUpdate](w, r)
	if !ok {
		return
	}
	settings, err := h.svc().SetSettings(r.Context(), body.FreeProductIds)
	if err != nil {
		writeError(w, http.StatusBadRequest, "membership_error", err.Error())
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPISettings(settings))
}

func entitlementOf(l Lookup) *generated.MembershipEntitlement {
	out := &generated.MembershipEntitlement{Remaining: l.Remaining, Max: l.Max}
	if l.Period != "" {
		period := l.Period
		out.Period = &period
	}
	return out
}

func toAPISettings(s Settings) generated.MembershipSettings {
	ids := s.FreeProductIDs
	if ids == nil {
		ids = []string{}
	}
	return generated.MembershipSettings{Label: s.Label, FreeProductIds: ids, Synced: s.Synced}
}
