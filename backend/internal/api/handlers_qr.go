package api

import (
	"encoding/base64"
	"errors"
	"net/http"

	"backend/internal/generated/api/generated"
	"backend/internal/qrsign"
	"backend/internal/repository"
	"backend/internal/response"

	"go.uber.org/zap"
)

// GetQRConfig publishes the tenant's public Ed25519 verification key and payload
// metadata for offline pickup-token verification. No auth; tenant is resolved
// from the host. Read-only: the keypair is provisioned at boot and on first
// order, so a 503 here means it isn't ready yet.
// (GET /qr-config)
func (h *Handlers) GetQRConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tenantID, ok := repository.TenantIDFromContext(ctx)
	if !ok {
		writeError(w, http.StatusBadRequest, "no_tenant", "No tenant resolved for this host")
		return
	}

	pub, err := h.qrKeys.TenantPublicKey(ctx)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusServiceUnavailable, "qr_key_unprovisioned", "Signing key not provisioned yet")
		return
	}
	if err != nil {
		h.logger.Error("qr-config: load tenant key", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "qr_key_error", "Could not load signing key")
		return
	}

	response.WriteJSON(w, http.StatusOK, generated.QRConfig{
		TenantId:       tenantID,
		Alg:            "ed25519",
		PayloadVersion: qrsign.Version,
		PublicKey:      base64.RawURLEncoding.EncodeToString(pub),
	})
}
