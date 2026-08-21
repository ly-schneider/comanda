package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"backend/internal/androidpkg"
	"backend/internal/config"

	"go.uber.org/zap"
)

type AndroidRelease struct {
	VersionName string `json:"versionName"`
	VersionCode int    `json:"versionCode"`
	DownloadURL string `json:"downloadUrl"`
	SHA256      string `json:"sha256,omitempty"`
}

type AndroidUpdateService interface {
	// baseURL is the origin the requesting device reached us on; the download
	// URL is built against it so the APK stays on a host the device can reach
	// (PUBLIC_BASE_URL is the web apex, not the API host).
	//
	// pkg selects the tenant build's asset; empty means the neutral build.
	GetLatestRelease(ctx context.Context, channel, pkg, baseURL string) (*AndroidRelease, error)
	// Returns a nil reader (no error) when no release is published.
	OpenAPKDownload(ctx context.Context, channel, pkg string) (io.ReadCloser, int64, error)
	// DownloadToken derives the token that authorizes fetching a package's APK.
	// Returns "" when no secret is configured.
	DownloadToken(channel, pkg string) string
	// VerifyDownloadToken reports whether token authorizes this package. Always
	// true while enforcement is off.
	VerifyDownloadToken(channel, pkg, token string) bool
}

var validChannels = map[string]struct{}{
	"staging":    {},
	"production": {},
}

// Private-repo assets are only reachable via the GitHub asset API with a
// token, so devices download through the proxy endpoint instead.
type releaseInfo struct {
	versionName string
	versionCode int
	sha256      string
	apkAssetURL string
}

type cachedRelease struct {
	info *releaseInfo
	at   time.Time
}

type androidUpdateService struct {
	repoOwner      string
	repoName       string
	token          string
	apiBaseURL     string
	downloadSecret string
	requireToken   bool
	logger         *zap.Logger

	mu       sync.Mutex
	cache    map[string]cachedRelease
	cacheTTL time.Duration
}

func NewAndroidUpdateService(cfg config.AndroidConfig, logger *zap.Logger) AndroidUpdateService {
	parts := strings.SplitN(cfg.GitHubRepo, "/", 2)
	owner, repo := parts[0], parts[1]
	return &androidUpdateService{
		repoOwner:      owner,
		repoName:       repo,
		token:          cfg.GitHubToken,
		apiBaseURL:     "https://api.github.com",
		downloadSecret: cfg.DownloadSecret,
		requireToken:   cfg.RequireDownloadToken,
		logger:         logger,
		cache:          make(map[string]cachedRelease),
		cacheTTL:       5 * time.Minute,
	}
}

func (s *androidUpdateService) GetLatestRelease(ctx context.Context, channel, pkg, baseURL string) (*AndroidRelease, error) {
	info, err := s.latestReleaseInfo(ctx, channel, pkg)
	if err != nil || info == nil {
		return nil, err
	}
	download := fmt.Sprintf("%s/v1/android/download?channel=%s", strings.TrimRight(baseURL, "/"), channel)
	if pkg != "" {
		download += "&pkg=" + pkg
	}
	// The app follows this URL verbatim, so it has to carry the credential the
	// download route will ask for.
	if tok := s.DownloadToken(channel, pkg); tok != "" {
		download += "&t=" + tok
	}
	return &AndroidRelease{
		VersionName: info.versionName,
		VersionCode: info.versionCode,
		DownloadURL: download,
		SHA256:      info.sha256,
	}, nil
}

func (s *androidUpdateService) DownloadToken(channel, pkg string) string {
	if s.downloadSecret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(s.downloadSecret))
	mac.Write([]byte(channel + "|" + pkg))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))[:32]
}

func (s *androidUpdateService) VerifyDownloadToken(channel, pkg, token string) bool {
	if !s.requireToken {
		return true
	}
	expected := s.DownloadToken(channel, pkg)
	if expected == "" {
		// Enforcement asked for without a secret to derive against would lock every
		// tablet out of updates. Refuse to pretend it is enforced.
		s.logger.Error("android update: ANDROID_REQUIRE_DOWNLOAD_TOKEN set without ANDROID_DOWNLOAD_SECRET; serving unauthenticated")
		return true
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(token)) == 1
}

func (s *androidUpdateService) OpenAPKDownload(ctx context.Context, channel, pkg string) (io.ReadCloser, int64, error) {
	info, err := s.latestReleaseInfo(ctx, channel, pkg)
	if err != nil {
		return nil, 0, err
	}
	if info == nil {
		return nil, 0, nil
	}

	req, err := s.newGitHubRequest(ctx, info.apkAssetURL, "application/octet-stream")
	if err != nil {
		return nil, 0, fmt.Errorf("android update: create download request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("android update: download apk: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, 0, fmt.Errorf("android update: apk download returned %d", resp.StatusCode)
	}
	return resp.Body, resp.ContentLength, nil
}

func (s *androidUpdateService) latestReleaseInfo(ctx context.Context, channel, pkg string) (*releaseInfo, error) {
	if _, ok := validChannels[channel]; !ok {
		return nil, fmt.Errorf("android update: invalid channel %q", channel)
	}
	if pkg != "" && !androidpkg.Valid(pkg) {
		return nil, fmt.Errorf("android update: invalid package %q", pkg)
	}

	cacheKey := channel + "|" + pkg

	s.mu.Lock()
	if entry, ok := s.cache[cacheKey]; ok && time.Since(entry.at) < s.cacheTTL {
		result := entry.info
		s.mu.Unlock()
		return result, nil
	}
	s.mu.Unlock()

	info, err := s.fetchLatestRelease(ctx, channel, pkg)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.cache[cacheKey] = cachedRelease{info: info, at: time.Now()}
	s.mu.Unlock()

	return info, nil
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func (s *androidUpdateService) newGitHubRequest(ctx context.Context, url, accept string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	return req, nil
}

func (s *androidUpdateService) fetchLatestRelease(ctx context.Context, channel, pkg string) (*releaseInfo, error) {
	tagPrefix := "android-" + channel + "-v"
	url := fmt.Sprintf("%s/repos/%s/%s/releases", s.apiBaseURL, s.repoOwner, s.repoName)

	req, err := s.newGitHubRequest(ctx, url, "application/vnd.github+json")
	if err != nil {
		return nil, fmt.Errorf("android update: create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("android update: fetch releases: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("android update: GitHub API returned %d", resp.StatusCode)
	}

	var releases []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("android update: decode releases: %w", err)
	}

	for _, rel := range releases {
		if !strings.HasPrefix(rel.TagName, tagPrefix) {
			continue
		}

		versionStr := strings.TrimPrefix(rel.TagName, tagPrefix)
		versionCode, err := parseVersionCode(versionStr)
		if err != nil {
			s.logger.Warn("android update: invalid version in tag", zap.String("tag", rel.TagName), zap.Error(err))
			continue
		}

		// Match the exact asset name, not a ".apk" suffix: a release carries one
		// APK per tenant package, and a suffix match would hand every device
		// whichever asset GitHub happened to list last.
		apkName := androidpkg.APKAssetName(pkg)
		var apkAssetURL string
		var sha256AssetURL string
		for _, asset := range rel.Assets {
			switch asset.Name {
			case apkName:
				apkAssetURL = asset.URL
			case apkName + ".sha256":
				sha256AssetURL = asset.URL
			}
		}

		if apkAssetURL == "" {
			continue
		}

		sha256 := ""
		if sha256AssetURL != "" {
			sha256 = s.fetchSHA256(ctx, sha256AssetURL)
		}

		return &releaseInfo{
			versionName: versionStr,
			versionCode: versionCode,
			sha256:      sha256,
			apkAssetURL: apkAssetURL,
		}, nil
	}

	return nil, nil
}

func (s *androidUpdateService) fetchSHA256(ctx context.Context, url string) string {
	req, err := s.newGitHubRequest(ctx, url, "application/octet-stream")
	if err != nil {
		return ""
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return ""
	}

	// sha256sum format: "<hash>  <filename>" or just "<hash>"
	parts := strings.Fields(strings.TrimSpace(string(body)))
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

func parseVersionCode(version string) (int, error) {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid version format: %s", version)
	}
	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	min, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	pat, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, err
	}
	return maj*10000 + min*100 + pat, nil
}
