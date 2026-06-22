package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"backend/internal/enttenant"
	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/repository"
	"backend/internal/response"
	"backend/internal/service"
)

// requirePlatformContext rejects platform-admin calls that did not arrive on a
// platform host. RequirePlatformAdmin already gates the RLS bypass, but a
// platform admin hitting these on a tenant host would have app.tenant_id bound
// to that tenant; refusing non-platform contexts keeps the routes strictly
// cross-tenant and unambiguous.
func (h *Handlers) requirePlatformContext(w http.ResponseWriter, r *http.Request) bool {
	if !enttenant.IsPlatform(r.Context()) {
		writeError(w, http.StatusBadRequest, "not_platform_host",
			"Platform admin routes are only available on the platform host")
		return false
	}
	return true
}

// ListTenants returns every tenant. (GET /platform/tenants)
func (h *Handlers) ListTenants(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlatformContext(w, r) {
		return
	}
	list, err := h.tenants.List(r.Context())
	if err != nil {
		writeEntError(w, err)
		return
	}
	items := make([]generated.Tenant, 0, len(list))
	for _, t := range list {
		items = append(items, toAPITenant(t))
	}
	response.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

// CreateTenant provisions a new tenant. (POST /platform/tenants)
func (h *Handlers) CreateTenant(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlatformContext(w, r) {
		return
	}
	var body generated.TenantCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	created, err := h.tenants.Create(r.Context(), body.Slug, body.Name)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidSlug),
			errors.Is(err, service.ErrReservedSlug),
			errors.Is(err, service.ErrInvalidName):
			writeError(w, http.StatusBadRequest, "invalid_tenant", err.Error())
		case errors.Is(err, repository.ErrConflict):
			writeError(w, http.StatusConflict, "slug_taken", "A tenant with that slug already exists")
		default:
			writeEntError(w, err)
		}
		return
	}
	response.WriteJSON(w, http.StatusCreated, toAPITenant(created))
}

// GetTenant returns one tenant. (GET /platform/tenants/{tenantId})
func (h *Handlers) GetTenant(w http.ResponseWriter, r *http.Request, tenantID string) {
	if !h.requirePlatformContext(w, r) {
		return
	}
	t, err := h.tenants.Get(r.Context(), tenantID)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPITenant(t))
}

// UpdateTenant activates or suspends a tenant. (PATCH /platform/tenants/{tenantId})
func (h *Handlers) UpdateTenant(w http.ResponseWriter, r *http.Request, tenantID string) {
	if !h.requirePlatformContext(w, r) {
		return
	}
	var body generated.TenantStatusUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	updated, err := h.tenants.SetStatus(r.Context(), tenantID, body.Status)
	if err != nil {
		if errors.Is(err, service.ErrInvalidStatus) {
			writeError(w, http.StatusBadRequest, "invalid_status", "status must be active or suspended")
			return
		}
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPITenant(updated))
}

func toAPITenant(t *ent.Tenant) generated.Tenant {
	return generated.Tenant{
		Id:        t.ID,
		Slug:      t.Slug,
		Name:      t.Name,
		Status:    string(t.Status),
		CreatedAt: t.CreatedAt,
		UpdatedAt: ptr(t.UpdatedAt),
	}
}
