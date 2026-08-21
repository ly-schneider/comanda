// Package ratelimit provides a fixed-window, in-process attempt limiter for
// unauthenticated endpoints.
//
// In-process means the window is per pod, so a limit of N across a replica set
// of R tolerates up to N*R attempts. That is fine for slowing a guess-the-PIN
// loop to uselessness; it is not a distributed quota.
package ratelimit

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type Limiter struct {
	mu      sync.Mutex
	windows map[string]*window
	period  time.Duration
	max     int
	now     func() time.Time
}

type window struct {
	count int
	reset time.Time
}

func New(period time.Duration, max int) *Limiter {
	return &Limiter{
		windows: map[string]*window{},
		period:  period,
		max:     max,
		now:     time.Now,
	}
}

func (l *Limiter) Allow(key string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	w := l.windows[key]
	if w == nil || now.After(w.reset) {
		l.windows[key] = &window{count: 1, reset: now.Add(l.period)}
		l.evictExpired(now)
		return true
	}
	if w.count >= l.max {
		return false
	}
	w.count++
	return true
}

func (l *Limiter) evictExpired(now time.Time) {
	if len(l.windows) < 1024 {
		return
	}
	for k, w := range l.windows {
		if now.After(w.reset) {
			delete(l.windows, k)
		}
	}
}

// Middleware rejects with 429 once a client IP exhausts its window.
func (l *Limiter) Middleware(message string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.Allow(ClientIP(r)) {
				http.Error(w, message, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func ClientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
