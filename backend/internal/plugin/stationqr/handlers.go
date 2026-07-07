package stationqr

import (
	"encoding/json"
	"errors"
	"net/http"

	"backend/internal/auth"
	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/response"

	"go.uber.org/zap"
)

type handlers struct {
	generated.Unimplemented
	rt *runtime
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	response.WriteJSON(w, status, generated.Error{Code: code, Message: message})
}

func toAPICode(c Code) generated.StationqrCode {
	out := generated.StationqrCode{Configured: c.Configured, Disabled: c.Disabled}
	if c.Configured && c.Token != "" {
		t := c.Token
		out.Token = &t
	}
	return out
}

func (h *handlers) StationqrGetCode(w http.ResponseWriter, r *http.Request) {
	code, err := h.rt.svc.Get(r.Context())
	if err != nil {
		h.rt.logger.Error("stationqr get code failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "stationqr_error", "Could not load enrollment code")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.WriteJSON(w, http.StatusOK, toAPICode(code))
}

func (h *handlers) StationqrRotateCode(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	code, err := h.rt.svc.Rotate(r.Context(), userID)
	if err != nil {
		h.rt.logger.Error("stationqr rotate code failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "stationqr_error", "Could not rotate enrollment code")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.WriteJSON(w, http.StatusOK, toAPICode(code))
}

func (h *handlers) StationqrSetCodeState(w http.ResponseWriter, r *http.Request) {
	var body generated.StationqrCodeStateUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	code, err := h.rt.svc.SetDisabled(r.Context(), body.Disabled)
	if err != nil {
		if errors.Is(err, ErrNotConfigured) {
			writeError(w, http.StatusBadRequest, "not_configured", "No enrollment code configured")
			return
		}
		h.rt.logger.Error("stationqr set code state failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "stationqr_error", "Could not update enrollment code")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.WriteJSON(w, http.StatusOK, toAPICode(code))
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
		case errors.Is(err, ErrNotConfigured), errors.Is(err, ErrInvalidToken):
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
