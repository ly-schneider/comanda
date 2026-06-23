package api

import (
	"bytes"
	"io"
	"net/http"

	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/repository"
	"backend/internal/response"
	"backend/internal/service"

	nanoid "backend/internal/id"
	"go.uber.org/zap"
)

// GetBranding returns the tenant's public presentation data. No auth; tenant is
// resolved from the host. On a platform host it returns neutral defaults.
// (GET /branding)
func (h *Handlers) GetBranding(w http.ResponseWriter, r *http.Request) {
	view, err := h.settings.GetBranding(r.Context())
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPIBranding(view))
}

func toAPIBranding(v *service.BrandingView) generated.Branding {
	return generated.Branding{
		DisplayName: v.DisplayName,
		LogoUrl:     v.LogoURL,
		FaviconUrl:  v.FaviconURL,
		Colors: generated.BrandingColors{
			Primary:           v.ColorPrimary,
			PrimaryForeground: v.ColorPrimaryForeground,
			Secondary:         v.ColorSecondary,
			Background:        v.ColorBackground,
			Foreground:        v.ColorForeground,
		},
		FontKey:   v.FontKey,
		RadiusKey: v.RadiusKey,
		Contact: generated.BrandingContact{
			Email: v.ContactEmail,
			Phone: v.ContactPhone,
		},
		Legal: generated.BrandingLegal{
			Name:    v.LegalName,
			Address: v.LegalAddress,
		},
		Currency: v.Currency,
	}
}

// allowedBrandingTypes are the content types accepted for logo/favicon uploads.
// SVG is intentionally excluded (script-injection risk on a public bucket; not
// renderable on the thermal receipt).
var allowedBrandingTypes = map[string]string{
	"image/png":                ".png",
	"image/jpeg":               ".jpg",
	"image/webp":               ".webp",
	"image/x-icon":             ".ico",
	"image/vnd.microsoft.icon": ".ico",
}

// UploadBrandingLogo stores a logo or favicon and records its URL in settings.
// (POST /settings/logo)
func (h *Handlers) UploadBrandingLogo(w http.ResponseWriter, r *http.Request) {
	if h.blobStore == nil {
		writeError(w, http.StatusNotImplemented, "not_configured", "Asset uploads not configured")
		return
	}

	ctx := r.Context()
	tenantID, ok := repository.TenantIDFromContext(ctx)
	if !ok {
		writeError(w, http.StatusBadRequest, "no_tenant", "No tenant in context")
		return
	}

	const maxSize = 5 << 20 // 5 MB
	if err := r.ParseMultipartForm(maxSize); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "File too large or invalid multipart form")
		return
	}

	kind := r.FormValue("kind")
	if kind == "" {
		kind = "logo"
	}
	if kind != "logo" && kind != "favicon" {
		writeError(w, http.StatusBadRequest, "invalid_body", "kind must be logo or favicon")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Missing file field")
		return
	}
	defer func() { _ = file.Close() }()

	buf := make([]byte, 512)
	n, err := io.ReadAtLeast(file, buf, 1)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Could not read file")
		return
	}
	contentType := http.DetectContentType(buf[:n])
	ext, allowed := allowedBrandingTypes[contentType]
	if !allowed {
		writeError(w, http.StatusBadRequest, "invalid_file_type", "Only PNG, JPEG, WebP, and ICO images are allowed")
		return
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
		return
	}

	var data bytes.Buffer
	if _, err := io.Copy(&data, file); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
		return
	}

	current, err := h.settings.GetSettings(ctx)
	if err != nil {
		writeEntError(w, err)
		return
	}
	previous := currentBrandingURL(current, kind)

	key := "tenants/" + tenantID + "/branding/" + kind + "-" + nanoid.New() + ext
	reader := bytes.NewReader(data.Bytes())
	url, err := h.blobStore.Upload(ctx, key, reader, contentType)
	if err != nil {
		h.logger.Error("failed to upload branding asset", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "upload_failed", "Failed to upload asset")
		return
	}

	in := service.BrandingInput{}
	if kind == "favicon" {
		in.FaviconURL = &url
	} else {
		in.LogoURL = &url
	}
	if err := h.settings.UpdateBranding(ctx, in); err != nil {
		writeError(w, http.StatusInternalServerError, "update_failed", err.Error())
		return
	}

	if previous != nil && *previous != "" {
		_ = h.blobStore.Delete(ctx, *previous)
	}

	response.WriteJSON(w, http.StatusOK, generated.LogoUploadResponse{Url: url})
}

// DeleteBrandingLogo removes a logo or favicon and nulls its column.
// (DELETE /settings/logo)
func (h *Handlers) DeleteBrandingLogo(w http.ResponseWriter, r *http.Request, params generated.DeleteBrandingLogoParams) {
	ctx := r.Context()

	kind := "logo"
	if params.Kind != nil {
		kind = string(*params.Kind)
	}

	current, err := h.settings.GetSettings(ctx)
	if err != nil {
		writeEntError(w, err)
		return
	}
	previous := currentBrandingURL(current, kind)

	empty := ""
	in := service.BrandingInput{}
	if kind == "favicon" {
		in.FaviconURL = &empty
	} else {
		in.LogoURL = &empty
	}
	if err := h.settings.UpdateBranding(ctx, in); err != nil {
		writeError(w, http.StatusInternalServerError, "update_failed", err.Error())
		return
	}

	if h.blobStore != nil && previous != nil && *previous != "" {
		_ = h.blobStore.Delete(ctx, *previous)
	}

	w.WriteHeader(http.StatusNoContent)
}

func currentBrandingURL(s *ent.Settings, kind string) *string {
	if kind == "favicon" {
		return s.FaviconURL
	}
	return s.LogoURL
}
