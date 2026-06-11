package middleware

import (
	"net/http"
	"strings"
)

// RealIP sets r.RemoteAddr to the rightmost X-Forwarded-For entry — the one
// appended by the trusted proxy in front (Traefik). The leftmost entries are
// client-supplied and spoofable, which is why chi's RealIP is deprecated.
func RealIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
				r.RemoteAddr = ip
			}
		}
		next.ServeHTTP(w, r)
	})
}
