package api

import (
	"io"
	"net/http"
	"strconv"
	"strings"

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

	release, err := h.androidUpdate.GetLatestRelease(r.Context(), channel, requestBaseURL(r))
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

	body, size, err := h.androidUpdate.OpenAPKDownload(r.Context(), channel)
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
	w.Header().Set("Content-Disposition", `attachment; filename="comanda-pos.apk"`)
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, body); err != nil {
		h.logger.Warn("android apk download aborted", zap.Error(err))
	}
}
