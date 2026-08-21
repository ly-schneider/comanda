package api

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"backend/internal/androidpkg"
	"backend/internal/generated/api/generated"
	"backend/internal/ratelimit"
	"backend/internal/response"

	"go.uber.org/zap"
)

// oapi-codegen binds these but does not enforce the enum or the pattern.
func androidChannel(raw *string) (string, bool) {
	if raw == nil || *raw == "" {
		return "production", true
	}
	channel := *raw
	return channel, channel == "staging" || channel == "production"
}

// Empty selects the neutral build, which is what tablets in the field still ask for.
func androidPackage(raw *string) (string, bool) {
	if raw == nil {
		return "", true
	}
	pkg := strings.TrimSpace(*raw)
	if pkg == "" {
		return "", true
	}
	return pkg, androidpkg.Valid(pkg)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (h *Handlers) GetAndroidLatestVersion(w http.ResponseWriter, r *http.Request, params generated.GetAndroidLatestVersionParams) {
	channel, ok := androidChannel((*string)(params.Channel))
	if !ok {
		response.WriteError(w, http.StatusBadRequest, "invalid channel")
		return
	}
	pkg, ok := androidPackage(params.Pkg)
	if !ok {
		response.WriteError(w, http.StatusBadRequest, "invalid package")
		return
	}
	if !h.androidUpdate.VerifyDownloadToken(channel, pkg, deref(params.T)) {
		response.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	release, err := h.androidUpdate.GetLatestRelease(r.Context(), channel, pkg, requestBaseURL(r))
	if err != nil {
		h.logger.Error("failed to fetch latest android release", zap.Error(err))
		response.WriteError(w, http.StatusBadGateway, "Failed to check for updates")
		return
	}

	if release == nil {
		response.WriteNoContent(w)
		return
	}

	response.WriteJSON(w, http.StatusOK, release)
}

func (h *Handlers) DownloadAndroidApk(w http.ResponseWriter, r *http.Request, params generated.DownloadAndroidApkParams) {
	channel, ok := androidChannel((*string)(params.Channel))
	if !ok {
		response.WriteError(w, http.StatusBadRequest, "invalid channel")
		return
	}
	pkg, ok := androidPackage(params.Pkg)
	if !ok {
		response.WriteError(w, http.StatusBadRequest, "invalid package")
		return
	}
	if !h.androidUpdate.VerifyDownloadToken(channel, pkg, deref(params.T)) {
		response.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	body, size, err := h.androidUpdate.OpenAPKDownload(r.Context(), channel, pkg)
	if err != nil {
		h.logger.Error("failed to download android apk", zap.Error(err))
		response.WriteError(w, http.StatusBadGateway, "Failed to download update")
		return
	}
	if body == nil {
		response.WriteError(w, http.StatusNotFound, "No release available")
		return
	}
	defer func() { _ = body.Close() }()

	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", `attachment; filename="`+androidpkg.APKAssetName(pkg)+`"`)
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, body); err != nil {
		h.logger.Warn("android apk download aborted", zap.Error(err))
	}
}

// The PIN lives here rather than in the APK, where it was a readable string
// constant in a publicly downloadable binary.
func (h *Handlers) VerifyAndroidSetupPin(w http.ResponseWriter, r *http.Request) {
	ip := ratelimit.ClientIP(r)
	if h.setupPinLimit.Blocked(ip) {
		response.WriteError(w, http.StatusTooManyRequests, "Zu viele Fehlversuche. Bitte später erneut versuchen.")
		return
	}

	var body struct {
		Pin string `json:"pin"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&body); err != nil {
		response.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}

	expected := h.config.Android.SetupPIN
	if expected == "" {
		h.logger.Error("android setup pin requested but ANDROID_SETUP_PIN is unset")
		response.WriteError(w, http.StatusNotImplemented, "Setup PIN not configured")
		return
	}

	if subtle.ConstantTimeCompare([]byte(expected), []byte(strings.TrimSpace(body.Pin))) != 1 {
		h.setupPinLimit.Fail(ip)
		response.WriteError(w, http.StatusUnauthorized, "Falsche PIN.")
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
