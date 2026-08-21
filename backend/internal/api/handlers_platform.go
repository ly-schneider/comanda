package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

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

// GetTenantAndroidInstall returns what the admin UI needs to put a tenant's app
// on a tablet: the package id and the token that authorizes the download.
//
// The install link is a credential — it is the only thing standing between the
// public internet and an APK carrying the SumUp affiliate key — so it is minted
// here behind the platform-admin gate rather than exposed on the tenant host.
func (h *Handlers) GetTenantAndroidInstall(
	w http.ResponseWriter,
	r *http.Request,
	tenantID string,
	params generated.GetTenantAndroidInstallParams,
) {
	if !h.requirePlatformContext(w, r) {
		return
	}

	channel := string(generated.Production)
	if params.Channel != nil {
		channel = string(*params.Channel)
	}

	t, err := h.tenants.Get(r.Context(), tenantID)
	if err != nil {
		writeEntError(w, err)
		return
	}
	if t.AndroidPackageID == nil || *t.AndroidPackageID == "" {
		writeError(w, http.StatusConflict, "no_android_package", "Tenant has no Android package id assigned")
		return
	}

	pkg := *t.AndroidPackageID
	token := h.androidUpdate.DownloadToken(channel, pkg)

	query := url.Values{}
	query.Set("pkg", pkg)
	query.Set("channel", channel)
	if token != "" {
		query.Set("t", token)
	}

	response.WriteJSON(w, http.StatusOK, generated.AndroidInstallInfo{
		PackageId:   pkg,
		Channel:     channel,
		InstallPath: "/install?" + query.Encode(),
		Token:       ptr(token),
	})
}
