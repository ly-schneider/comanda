package api

import (
	"net/http"

	"backend/internal/auth"
	"backend/internal/generated/api/generated"
	"backend/internal/response"
)

type accountTenant struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	LogoURL     string `json:"logoUrl,omitempty"`
	Role        string `json:"role,omitempty"`
}

// ListAccountTenants lists the tenants the user may manage. Runs before any
// tenant is bound, so it skips ResolveVendorTenant.
func (h *Handlers) ListAccountTenants(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !auth.IsPlatformContext(ctx) {
		writeError(w, http.StatusBadRequest, "not_platform_host", "Account routes are only available on the platform host")
		return
	}
	userID, ok := auth.GetUserID(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	tenants, err := h.account.ListManageableTenants(ctx, userID, auth.IsPlatformAdmin(ctx))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	items := make([]accountTenant, 0, len(tenants))
	for _, t := range tenants {
		items = append(items, accountTenant{
			ID:          t.ID,
			Slug:        t.Slug,
			Name:        t.Name,
			DisplayName: t.DisplayName,
			LogoURL:     t.LogoURL,
			Role:        t.Role,
		})
	}
	response.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

// GetAccountTenant returns the tenant the request resolved to via the
// user→tenant resolver (the logged-in vendor's active organization).
func (h *Handlers) GetAccountTenant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID, ok := auth.GetTenantID(ctx)
	if !ok {
		writeError(w, http.StatusForbidden, "no_tenant", "No tenant resolved for this request")
		return
	}
	t, err := h.tenants.Get(ctx, tenantID)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, generated.AccountTenant{
		Id:   t.ID,
		Slug: t.Slug,
		Name: t.Name,
	})
}
