package api

import (
	"net/http"

	"backend/internal/auth"
	"backend/internal/generated/api/generated"
	"backend/internal/response"
)

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
