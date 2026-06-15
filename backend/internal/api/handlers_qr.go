package api

import (
	"encoding/base64"
	"net/http"

	"backend/internal/generated/api/generated"
	"backend/internal/qrsign"
	"backend/internal/repository"
	"backend/internal/response"

	"go.uber.org/zap"
)

// GetQRConfig publishes the tenant's public Ed25519 verification key and payload
// metadata for offline pickup-token verification. No auth; tenant is resolved
// from the host. Ensures the keypair exists so setup always yields a usable key.
// (GET /qr-config)
func (h *Handlers) GetQRConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tenantID, ok := repository.TenantIDFromContext(ctx)
	if !ok {
		writeError(w, http.StatusBadRequest, "no_tenant", "No tenant resolved for this host")
		return
	}

	_, pub, err := h.qrKeys.EnsureTenantKey(ctx)
	if err != nil {
		h.logger.Error("qr-config: ensure tenant key", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "qr_key_error", "Could not provision signing key")
		return
	}

	response.WriteJSON(w, http.StatusOK, generated.QRConfig{
		TenantId:       tenantID,
		Alg:            "ed25519",
		PayloadVersion: qrsign.Version,
		PublicKey:      base64.RawURLEncoding.EncodeToString(pub),
	})
}
