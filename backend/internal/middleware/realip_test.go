package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRealIP(t *testing.T) {
	cases := []struct {
		name string
		xff  string
		want string
	}{
		{"no header keeps peer addr", "", "192.0.2.1:1234"},
		{"single entry", "203.0.113.7", "203.0.113.7"},
		{"rightmost wins over spoofed leftmost", "6.6.6.6, 203.0.113.7", "203.0.113.7"},
		{"trims whitespace", "6.6.6.6 ,  203.0.113.7 ", "203.0.113.7"},
		{"empty rightmost keeps peer addr", "6.6.6.6,", "192.0.2.1:1234"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			h := RealIP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.RemoteAddr
			}))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = "192.0.2.1:1234"
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			h.ServeHTTP(httptest.NewRecorder(), req)
			if got != tc.want {
				t.Errorf("RemoteAddr = %q, want %q", got, tc.want)
			}
		})
	}
}
