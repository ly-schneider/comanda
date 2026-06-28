package api

import (
	"net/http"

	"backend/internal/auth"
	"backend/internal/generated/api/generated"
	"backend/internal/response"
)

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

	items := make([]generated.ManageableTenant, 0, len(tenants))
	for _, t := range tenants {
		item := generated.ManageableTenant{Id: t.ID, Slug: t.Slug, Name: t.Name, DisplayName: t.DisplayName}
		if t.LogoURL != "" {
			item.LogoUrl = ptr(t.LogoURL)
		}
		if t.Role != "" {
			item.Role = ptr(t.Role)
		}
		items = append(items, item)
	}
	response.WriteJSON(w, http.StatusOK, generated.ManageableTenantList{Items: items})
}

// GetAccountTenant returns the tenant the request resolved to. The slug is
// already bound by ResolveVendorTenant, so the path param is unused here.
func (h *Handlers) GetAccountTenant(w http.ResponseWriter, r *http.Request, _ generated.AccountTenantParam) {
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
