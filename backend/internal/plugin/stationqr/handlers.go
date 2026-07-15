package stationqr

import (
	"encoding/json"
	"errors"
	"net/http"

	"backend/internal/auth"
	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/response"

	nanoid "backend/internal/id"
	"go.uber.org/zap"
)

type handlers struct {
	generated.Unimplemented
	rt *runtime
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	response.WriteJSON(w, status, generated.Error{Code: code, Message: message})
}

func toAPIPreset(p Preset) generated.StationqrPreset {
	ids := p.ProductIDs
	if ids == nil {
		ids = []string{}
	}
	return generated.StationqrPreset{
		Id:         p.ID,
		Name:       p.Name,
		Disabled:   p.Disabled,
		Token:      p.Token,
		ProductIds: ids,
		CreatedAt:  p.CreatedAt,
		UpdatedAt:  p.UpdatedAt,
	}
}

func (h *handlers) writePresetError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNameRequired):
		writeError(w, http.StatusBadRequest, "name_required", "Preset name is required")
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Preset not found")
	default:
		h.rt.logger.Error("stationqr preset error", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "stationqr_error", "Could not process the preset")
	}
}

func (h *handlers) StationqrListPresets(w http.ResponseWriter, r *http.Request) {
	presets, err := h.rt.svc.ListPresets(r.Context())
	if err != nil {
		h.rt.logger.Error("stationqr list presets failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "stationqr_error", "Could not load enrollment presets")
		return
	}
	items := make([]generated.StationqrPreset, 0, len(presets))
	for _, p := range presets {
		items = append(items, toAPIPreset(p))
	}
	w.Header().Set("Cache-Control", "no-store")
	response.WriteJSON(w, http.StatusOK, generated.StationqrPresetList{Items: items})
}

func (h *handlers) StationqrCreatePreset(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	var body generated.StationqrPresetCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	preset, err := h.rt.svc.CreatePreset(r.Context(), body.Name, deref(body.ProductIds), userID)
	if err != nil {
		h.writePresetError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.WriteJSON(w, http.StatusCreated, toAPIPreset(preset))
}

func (h *handlers) StationqrUpdatePreset(w http.ResponseWriter, r *http.Request, presetId string) {
	if !nanoid.Valid(presetId) {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid id")
		return
	}
	var body generated.StationqrPresetUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	preset, err := h.rt.svc.UpdatePreset(r.Context(), presetId, UpdatePresetInput{
		Name:       body.Name,
		Disabled:   body.Disabled,
		ProductIDs: body.ProductIds,
	})
	if err != nil {
		h.writePresetError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.WriteJSON(w, http.StatusOK, toAPIPreset(preset))
}

func (h *handlers) StationqrRotatePreset(w http.ResponseWriter, r *http.Request, presetId string) {
	if !nanoid.Valid(presetId) {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid id")
		return
	}
	preset, err := h.rt.svc.RotatePreset(r.Context(), presetId)
	if err != nil {
		h.writePresetError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.WriteJSON(w, http.StatusOK, toAPIPreset(preset))
}

func (h *handlers) StationqrDeletePreset(w http.ResponseWriter, r *http.Request, presetId string) {
	if !nanoid.Valid(presetId) {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid id")
		return
	}
	if err := h.rt.svc.DeletePreset(r.Context(), presetId); err != nil {
		h.writePresetError(w, err)
		return
	}
	response.WriteNoContent(w)
}

func (h *handlers) StationqrEnroll(w http.ResponseWriter, r *http.Request) {
	var body generated.StationqrEnrollRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	res, err := h.rt.svc.Enroll(
		r.Context(),
		body.Token,
		body.DeviceKey,
		strOrEmpty(body.Name),
		strOrEmpty(body.Model),
		strOrEmpty(body.Os),
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrDisabled):
			writeError(w, http.StatusForbidden, "enrollment_disabled", "Enrollment is currently disabled")
		case errors.Is(err, ErrInvalidToken):
			writeError(w, http.StatusBadRequest, "invalid_token", "Invalid enrollment code")
		default:
			h.rt.logger.Error("stationqr enroll failed", zap.Error(err))
			writeError(w, http.StatusBadRequest, "stationqr_error", "Enrollment failed")
		}
		return
	}
	response.WriteJSON(w, http.StatusOK, generated.StationqrEnrollResponse{
		Token:  res.Token,
		Device: toAPIDevice(res.Device),
	})
}

func deref(s *[]string) []string {
	if s == nil {
		return nil
	}
	return *s
}

func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func toAPIDevice(e *ent.Device) generated.Device {
	return generated.Device{
		Id:        e.ID,
		Name:      e.Name,
		Model:     e.Model,
		Os:        e.Os,
		Type:      generated.DeviceType(e.Type),
		Status:    generated.DeviceStatus(e.Status),
		DecidedBy: e.DecidedBy,
		DecidedAt: e.DecidedAt,
		CreatedAt: &e.CreatedAt,
		UpdatedAt: &e.UpdatedAt,
	}
}
