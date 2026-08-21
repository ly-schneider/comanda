package api

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"backend/internal/androidpkg"
	"backend/internal/response"

	"go.uber.org/zap"
)

func androidChannel(r *http.Request) (string, bool) {
	channel := r.URL.Query().Get("channel")
	if channel == "" {
		channel = "production"
	}
	return channel, channel == "staging" || channel == "production"
}

// androidPackage reads the tenant build selector. Empty selects the neutral
// build, which is what pre-tenant-build tablets in the field still ask for.
func androidPackage(r *http.Request) (string, bool) {
	pkg := strings.TrimSpace(r.URL.Query().Get("pkg"))
	if pkg == "" {
		return "", true
	}
	return pkg, androidpkg.Valid(pkg)
}

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (h *Handlers) GetAndroidLatestVersion(w http.ResponseWriter, r *http.Request) {
	channel, ok := androidChannel(r)
	if !ok {
		response.WriteError(w, http.StatusBadRequest, "invalid channel")
		return
	}
	pkg, ok := androidPackage(r)
	if !ok {
		response.WriteError(w, http.StatusBadRequest, "invalid package")
		return
	}
	if !h.androidUpdate.VerifyDownloadToken(channel, pkg, r.URL.Query().Get("t")) {
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

func (h *Handlers) DownloadAndroidApk(w http.ResponseWriter, r *http.Request) {
	channel, ok := androidChannel(r)
	if !ok {
		response.WriteError(w, http.StatusBadRequest, "invalid channel")
		return
	}
	pkg, ok := androidPackage(r)
	if !ok {
		response.WriteError(w, http.StatusBadRequest, "invalid package")
		return
	}
	if !h.androidUpdate.VerifyDownloadToken(channel, pkg, r.URL.Query().Get("t")) {
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

// VerifyAndroidSetupPin checks the operator PIN that gates Device Setup on the
// tablet.
//
// The PIN lives here rather than in the APK, where it was a readable string
// constant in a publicly downloadable binary. This does not stop a patched app —
// nothing server-side can, since the screen it guards is local — but it closes
// the path that matters: a PIN lifted from a downloaded APK and typed into the
// real one.
func (h *Handlers) VerifyAndroidSetupPin(w http.ResponseWriter, r *http.Request) {
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
		response.WriteError(w, http.StatusUnauthorized, "Falsche PIN.")
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
