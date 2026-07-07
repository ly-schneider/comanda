package stationqr

import (
	"net"
	"net/http"
	"sync"
	"time"

	"backend/internal/generated/api/generated"
	"backend/internal/response"
)

const (
	enrollRateWindow = time.Minute
	enrollRateMax    = 10
)

type rateLimiter struct {
	mu      sync.Mutex
	windows map[string]*rateWindow
	window  time.Duration
	max     int
	now     func() time.Time
}

type rateWindow struct {
	count int
	reset time.Time
}

func newRateLimiter(window time.Duration, max int) *rateLimiter {
	return &rateLimiter{
		windows: map[string]*rateWindow{},
		window:  window,
		max:     max,
		now:     time.Now,
	}
}

func (l *rateLimiter) allow(key string) bool {
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

func (l *rateLimiter) evictExpired(now time.Time) {
	if len(l.windows) < 1024 {
		return
	}
	for k, w := range l.windows {
		if now.After(w.reset) {
			delete(l.windows, k)
		}
	}
}

func (l *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r)) {
			response.WriteJSON(w, http.StatusTooManyRequests, generated.Error{
				Code:    "rate_limited",
				Message: "Too many enrollment attempts. Try again shortly.",
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
