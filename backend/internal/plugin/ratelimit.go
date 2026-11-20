package plugin

import (
	"net"
	"net/http"
	"sync"
	"time"

	"backend/internal/generated/api/generated"
	"backend/internal/response"
)

// RateLimiter keeps a public plugin endpoint — one reachable without a session
// or a paired device — from being hammered from a single address. It counts per
// client IP in a fixed window and holds nothing between restarts.
type RateLimiter struct {
	mu      sync.Mutex
	windows map[string]*rateWindow
	window  time.Duration
	max     int
	message string
	now     func() time.Time
}

type rateWindow struct {
	count int
	reset time.Time
}

func NewRateLimiter(window time.Duration, max int, message string) *RateLimiter {
	return &RateLimiter{
		windows: map[string]*rateWindow{},
		window:  window,
		max:     max,
		message: message,
		now:     time.Now,
	}
}

func (l *RateLimiter) allow(key string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	w := l.windows[key]
	if w == nil || now.After(w.reset) {
		l.windows[key] = &rateWindow{count: 1, reset: now.Add(l.window)}
		l.evictExpired(now)
		return true
	}
	if w.count >= l.max {
		return false
	}
	w.count++
	return true
}

func (l *RateLimiter) evictExpired(now time.Time) {
	if len(l.windows) < 1024 {
		return
	}
	for k, w := range l.windows {
		if now.After(w.reset) {
			delete(l.windows, k)
		}
	}
}

// Middleware rejects a client over its limit with 429 before the handler runs.
func (l *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r)) {
			response.WriteJSON(w, http.StatusTooManyRequests, generated.Error{
				Code:    "rate_limited",
				Message: l.message,
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
