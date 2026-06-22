package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/response"
	"backend/internal/service"

	"go.uber.org/zap"
)

func toAPIDomain(d *ent.TenantDomain) generated.Domain {
	return generated.Domain{
		Id:         d.ID,
		Hostname:   d.Hostname,
		IsPrimary:  d.IsPrimary,
		VerifiedAt: d.VerifiedAt,
		CertStatus: generated.DomainCertStatus(d.CertStatus),
		CreatedAt:  d.CreatedAt,
	}
}

// ListDomains returns the tenant's custom domains.
// (GET /domains)
func (h *Handlers) ListDomains(w http.ResponseWriter, r *http.Request) {
	rows, err := h.domains.List(r.Context())
	if err != nil {
		writeEntError(w, err)
		return
	}
	items := make([]generated.Domain, 0, len(rows))
	for _, d := range rows {
		items = append(items, toAPIDomain(d))
	}
	response.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func toAPIDomainWithInstructions(view *service.DomainView) generated.DomainWithInstructions {
	return generated.DomainWithInstructions{
		Domain:      toAPIDomain(view.Domain),
		CnameTarget: view.CNAMETarget,
		TxtName:     view.TXTName,
		TxtValue:    view.TXTValue,
	}
}

// GetDomain returns one domain with its DNS instructions so an admin can recover
// the CNAME/TXT records after navigating away from the create response.
// (GET /domains/{id})
func (h *Handlers) GetDomain(w http.ResponseWriter, r *http.Request, id string) {
	view, err := h.domains.GetView(r.Context(), id)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPIDomainWithInstructions(view))
}

// CreateDomain registers a custom domain and returns the DNS records to publish.
// (POST /domains)
func (h *Handlers) CreateDomain(w http.ResponseWriter, r *http.Request) {
	var body generated.CreateDomainJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	view, err := h.domains.Create(r.Context(), body.Hostname)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidHostname):
			writeError(w, http.StatusBadRequest, "invalid_hostname", "The hostname is not a valid domain.")
		case errors.Is(err, service.ErrPlatformSubdomain):
			writeError(w, http.StatusBadRequest, "platform_subdomain", "This hostname is a platform subdomain and cannot be added as a custom domain.")
		default:
			writeEntError(w, err)
		}
		return
	}

	response.WriteJSON(w, http.StatusCreated, toAPIDomainWithInstructions(view))
}

// DeleteDomain removes a custom domain.
// (DELETE /domains/{id})
func (h *Handlers) DeleteDomain(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.domains.Delete(r.Context(), id); err != nil {
		switch {
		case errors.Is(err, service.ErrPlatformDomain):
			writeError(w, http.StatusBadRequest, "platform_domain", "Platform-assigned domains cannot be removed.")
		default:
			writeEntError(w, err)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// VerifyDomain runs the ownership check immediately for one domain.
// (POST /domains/{id}/verify)
func (h *Handlers) VerifyDomain(w http.ResponseWriter, r *http.Request, id string) {
	d, err := h.domains.VerifyNow(r.Context(), id)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPIDomain(d))
}

// HandleTrustedOrigins returns every verified custom-domain hostname across all
// tenants for the web tier to feed into Better Auth's trustedOrigins. It runs on
// the privileged client with no tenant tx (it is intentionally cross-tenant) and
// is gated by a shared internal token. The route is only registered when the
// token is configured, so absent a secret this feed does not exist.
func (h *Handlers) HandleTrustedOrigins(w http.ResponseWriter, r *http.Request) {
	expected := h.config.CustomDomain.InternalAPIToken
	if expected == "" || !validInternalToken(r, expected) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid internal token")
		return
	}

	hostnames, err := h.domains.ListAllVerifiedHostnames(r.Context(), h.privileged)
	if err != nil {
		h.logger.Error("trusted-origins: list verified hostnames failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read domains")
		return
	}
	response.WriteJSON(w, http.StatusOK, map[string]any{"hostnames": hostnames})
}

// validInternalToken compares the request's bearer token to the configured
// internal token in constant time.
func validInternalToken(r *http.Request, expected string) bool {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	got := strings.TrimSpace(auth[len(prefix):])
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}
