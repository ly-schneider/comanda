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
		repoOwner:     "ly-schneider",
		repoName:      "comanda",
		token:         "test-token",
		apiBaseURL:    apiBaseURL,
		publicBaseURL: "https://api.example.com",
		logger:        zap.NewNop(),
		cache:         make(map[string]cachedRelease),
		cacheTTL:      5 * time.Minute,
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
		_, _ = fmt.Fprintf(w, `[
			{"tag_name": "web-v2.0.0", "assets": []},
			{"tag_name": "android-production-v1.0.3", "assets": [
				{"name": "comanda-pos.apk", "url": "%[1]s/assets/1"},
				{"name": "comanda-pos.apk.sha256", "url": "%[1]s/assets/2"}
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

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestGetLatestReleaseUsesProxyDownloadURL(t *testing.T) {
	srv := newGitHubStub(t)
	s := newTestAndroidUpdateService(srv.URL)

	release, err := s.GetLatestRelease(context.Background(), "production")
	if err != nil {
		t.Fatalf("GetLatestRelease: %v", err)
	}
	if release == nil {
		t.Fatal("expected a release")
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

	body, size, err := s.OpenAPKDownload(context.Background(), "production")
	if err != nil {
		t.Fatalf("OpenAPKDownload: %v", err)
	}
	if body == nil {
		t.Fatal("expected a body")
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

	release, err := s.GetLatestRelease(context.Background(), "staging")
	if err != nil {
		t.Fatalf("GetLatestRelease: %v", err)
	}
	if release != nil {
		t.Errorf("expected no release, got %+v", release)
	}
}

func TestGetLatestReleaseInvalidChannel(t *testing.T) {
	s := newTestAndroidUpdateService("http://unused")
	if _, err := s.GetLatestRelease(context.Background(), "beta"); err == nil {
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
		if _, err := s.GetLatestRelease(context.Background(), "production"); err != nil {
			t.Fatalf("GetLatestRelease: %v", err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Errorf("GitHub API called %d times, want 1 (cached)", calls)
	}
}
