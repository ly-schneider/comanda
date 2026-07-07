package club100

import (
	"encoding/json"
	"net/http"

	"backend/internal/generated/api/generated"
	"backend/internal/response"

	"go.uber.org/zap"
)

// embeds generated.Unimplemented to satisfy the full ServerInterface; only the
// club100 operationIds are real.
type handlers struct {
	generated.Unimplemented
	rt *runtime
}

func (h *handlers) svc() Service { return h.rt.svc }

func writeError(w http.ResponseWriter, status int, code, message string) {
	response.WriteJSON(w, status, generated.Error{Code: code, Message: message})
}

func (h *handlers) Club100ListPeople(w http.ResponseWriter, r *http.Request) {
	people, err := h.svc().GetPeopleWithRedemptions(r.Context())
	if err != nil {
		h.rt.logger.Error("club100 list people failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "club100_error", err.Error())
		return
	}
	items := make([]generated.Club100Person, 0, len(people))
	for _, p := range people {
		items = append(items, generated.Club100Person{
			Id:        p.ID,
			FirstName: p.FirstName,
			LastName:  p.LastName,
			Remaining: p.Remaining,
			Max:       p.Max,
		})
	}
	response.WriteJSON(w, http.StatusOK, generated.Club100PersonList{Items: items})
}

func (h *handlers) Club100GetRemaining(w http.ResponseWriter, r *http.Request, elvantoPersonId string) {
	remaining, max, err := h.svc().GetRemainingRedemptions(r.Context(), elvantoPersonId)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "club100_error", err.Error())
		return
	}
	response.WriteJSON(w, http.StatusOK, generated.Club100Remaining{
		ElvantoPersonId: elvantoPersonId,
		Remaining:       remaining,
		Max:             max,
	})
}

func (h *handlers) Club100GetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.svc().GetSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "club100_error", err.Error())
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPISettings(settings))
}

func (h *handlers) Club100UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var body generated.Club100SettingsUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	settings, err := h.svc().SetSettings(r.Context(), body.FreeProductIds, body.MaxRedemptions)
	if err != nil {
		writeError(w, http.StatusBadRequest, "club100_error", err.Error())
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPISettings(settings))
}

func toAPISettings(s Settings) generated.Club100Settings {
	ids := s.FreeProductIDs
	if ids == nil {
		ids = []string{}
	}
	return generated.Club100Settings{
		FreeProductIds: ids,
		MaxRedemptions: s.MaxRedemptions,
	}
}
