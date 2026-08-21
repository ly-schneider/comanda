package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

func newTestAndroidUpdateService(apiBaseURL string) *androidUpdateService {
	return &androidUpdateService{
		repoOwner:  "ly-schneider",
		repoName:   "comanda",
		token:      "test-token",
		apiBaseURL: apiBaseURL,
		logger:     zap.NewNop(),
		cache:      make(map[string]cachedRelease),
		cacheTTL:   5 * time.Minute,
	}
}

func newGitHubStub(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server

	mux.HandleFunc("/repos/ly-schneider/comanda/releases", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("releases request Authorization = %q", got)
		}
		// The tenant APKs are listed after the neutral one on purpose: a suffix
		// match would return the last ".apk" seen and hand every device the wrong
		// tenant's build.
		_, _ = fmt.Fprintf(w, `[
			{"tag_name": "web-v2.0.0", "assets": []},
			{"tag_name": "android-production-v1.0.3", "assets": [
				{"name": "comanda-pos.apk", "url": "%[1]s/assets/1"},
				{"name": "comanda-pos.apk.sha256", "url": "%[1]s/assets/2"},
				{"name": "comanda-pos-t_acme.apk", "url": "%[1]s/assets/4"},
				{"name": "comanda-pos-t_acme.apk.sha256", "url": "%[1]s/assets/5"}
			]},
			{"tag_name": "android-staging-v1.0.2", "assets": [
				{"name": "comanda-pos.apk", "url": "%[1]s/assets/3"}
			]}
		]`, srv.URL)
	})
	mux.HandleFunc("/assets/1", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/octet-stream" {
			t.Errorf("apk asset Accept = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("apk asset Authorization = %q", got)
		}
		_, _ = w.Write([]byte("apk-bytes"))
	})
	mux.HandleFunc("/assets/2", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("abc123  comanda-pos.apk\n"))
	})
	mux.HandleFunc("/assets/4", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("acme-apk-bytes"))
	})
	mux.HandleFunc("/assets/5", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("def456  comanda-pos-t_acme.apk\n"))
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestGetLatestReleaseUsesProxyDownloadURL(t *testing.T) {
	srv := newGitHubStub(t)
	s := newTestAndroidUpdateService(srv.URL)

	release, err := s.GetLatestRelease(context.Background(), "production", "", "https://api.example.com")
	if err != nil {
		t.Fatalf("GetLatestRelease: %v", err)
	}
	if release == nil {
		// Unreachable: staticcheck doesn't model t.Fatal as terminating and
		// flags the derefs below as a nil deref (SA5011).
		t.Fatal("expected a release")
		return
	}
	if release.VersionName != "1.0.3" || release.VersionCode != 10003 {
		t.Errorf("got version %s (%d)", release.VersionName, release.VersionCode)
	}
	if want := "https://api.example.com/v1/android/download?channel=production"; release.DownloadURL != want {
		t.Errorf("DownloadURL = %q, want %q", release.DownloadURL, want)
	}
	if release.SHA256 != "abc123" {
		t.Errorf("SHA256 = %q", release.SHA256)
	}
}

func TestOpenAPKDownloadStreamsAsset(t *testing.T) {
	srv := newGitHubStub(t)
	s := newTestAndroidUpdateService(srv.URL)

	body, size, err := s.OpenAPKDownload(context.Background(), "production", "")
	if err != nil {
		t.Fatalf("OpenAPKDownload: %v", err)
	}
	if body == nil {
		t.Fatal("expected a body")
		return
	}
	defer func() { _ = body.Close() }()

	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(data) != "apk-bytes" {
		t.Errorf("body = %q", data)
	}
	if size != int64(len("apk-bytes")) {
		t.Errorf("size = %d", size)
	}
}

func TestGetLatestReleaseNoReleaseForChannel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/ly-schneider/comanda/releases", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `[{"tag_name": "web-v2.0.0", "assets": []}]`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	s := newTestAndroidUpdateService(srv.URL)

	release, err := s.GetLatestRelease(context.Background(), "staging", "", "https://api.example.com")
	if err != nil {
		t.Fatalf("GetLatestRelease: %v", err)
	}
	if release != nil {
		t.Errorf("expected no release, got %+v", release)
	}
}

func TestGetLatestReleaseInvalidChannel(t *testing.T) {
	s := newTestAndroidUpdateService("http://unused")
	if _, err := s.GetLatestRelease(context.Background(), "beta", "", "https://api.example.com"); err == nil {
		t.Fatal("expected error for invalid channel")
	}
}

func TestLatestReleaseInfoCaches(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/ly-schneider/comanda/releases", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		_, _ = fmt.Fprint(w, `[{"tag_name": "android-production-v1.0.3", "assets": [{"name": "comanda-pos.apk", "url": "http://unused/assets/1"}]}]`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	s := newTestAndroidUpdateService(srv.URL)

	for range 2 {
		if _, err := s.GetLatestRelease(context.Background(), "production", "", "https://api.example.com"); err != nil {
			t.Fatalf("GetLatestRelease: %v", err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Errorf("GitHub API called %d times, want 1 (cached)", calls)
	}
}

func TestGetLatestReleasePicksPackageAsset(t *testing.T) {
	srv := newGitHubStub(t)
	s := newTestAndroidUpdateService(srv.URL)

	release, err := s.GetLatestRelease(context.Background(), "production", "t_acme", "https://api.example.com")
	if err != nil {
		t.Fatalf("GetLatestRelease: %v", err)
	}
	if release == nil {
		t.Fatal("expected a release")
		return
	}
	if want := "https://api.example.com/v1/android/download?channel=production&pkg=t_acme"; release.DownloadURL != want {
		t.Errorf("DownloadURL = %q, want %q", release.DownloadURL, want)
	}
	// The neutral build's sha256 is abc123; picking it here would mean the asset
	// match fell back to the wrong APK.
	if release.SHA256 != "def456" {
		t.Errorf("SHA256 = %q, want the t_acme asset's def456", release.SHA256)
	}
}

func TestOpenAPKDownloadStreamsPackageAsset(t *testing.T) {
	srv := newGitHubStub(t)
	s := newTestAndroidUpdateService(srv.URL)

	body, _, err := s.OpenAPKDownload(context.Background(), "production", "t_acme")
	if err != nil {
		t.Fatalf("OpenAPKDownload: %v", err)
	}
	if body == nil {
		t.Fatal("expected a body")
		return
	}
	defer func() { _ = body.Close() }()

	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(data) != "acme-apk-bytes" {
		t.Errorf("body = %q, want the t_acme asset", data)
	}
}

func TestGetLatestReleaseUnknownPackageHasNoRelease(t *testing.T) {
	srv := newGitHubStub(t)
	s := newTestAndroidUpdateService(srv.URL)

	release, err := s.GetLatestRelease(context.Background(), "production", "t_nobody", "https://api.example.com")
	if err != nil {
		t.Fatalf("GetLatestRelease: %v", err)
	}
	if release != nil {
		t.Errorf("expected no release for an unbuilt package, got %+v", release)
	}
}

func TestGetLatestReleaseInvalidPackage(t *testing.T) {
	s := newTestAndroidUpdateService("http://unused")
	for _, pkg := range []string{"t_ACME", "acme", "t_acme-1", "t_", "../etc"} {
		if _, err := s.GetLatestRelease(context.Background(), "production", pkg, "https://api.example.com"); err == nil {
			t.Errorf("expected error for package %q", pkg)
		}
	}
}

func TestDownloadTokenEnforcement(t *testing.T) {
	s := newTestAndroidUpdateService("http://unused")

	// No secret configured: no token to hand out, and nothing to enforce.
	if got := s.DownloadToken("production", "t_acme"); got != "" {
		t.Errorf("DownloadToken without a secret = %q, want empty", got)
	}
	if !s.VerifyDownloadToken("production", "t_acme", "") {
		t.Error("verification should pass while enforcement is off")
	}

	s.downloadSecret = "s3cret"
	token := s.DownloadToken("production", "t_acme")
	if token == "" {
		t.Fatal("expected a derived token")
	}
	if s.DownloadToken("production", "t_acme") != token {
		t.Error("token derivation must be deterministic — the APK bakes it at build time")
	}
	if s.DownloadToken("production", "t_other") == token {
		t.Error("token must differ per package")
	}
	if s.DownloadToken("staging", "t_acme") == token {
		t.Error("token must differ per channel")
	}

	// Still not enforcing: a wrong token is accepted so tablets in the field keep
	// updating until the fleet ships a build that sends one.
	if !s.VerifyDownloadToken("production", "t_acme", "wrong") {
		t.Error("wrong token should pass while enforcement is off")
	}

	s.requireToken = true
	if !s.VerifyDownloadToken("production", "t_acme", token) {
		t.Error("correct token rejected under enforcement")
	}
	if s.VerifyDownloadToken("production", "t_acme", "wrong") {
		t.Error("wrong token accepted under enforcement")
	}
	if s.VerifyDownloadToken("production", "t_acme", "") {
		t.Error("missing token accepted under enforcement")
	}
	if s.VerifyDownloadToken("production", "t_other", token) {
		t.Error("another package's token accepted")
	}

	// Enforcement without a secret must not lock the fleet out.
	s.downloadSecret = ""
	if !s.VerifyDownloadToken("production", "t_acme", "") {
		t.Error("enforcement without a secret should fail open, not brick updates")
	}
}
