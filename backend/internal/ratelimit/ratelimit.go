// Package ratelimit provides a fixed-window, in-process attempt limiter.
// The window is per pod, so a limit of N across R replicas tolerates N*R.
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

// Blocked reports whether key has spent its budget. It does not consume any.
func (l *Limiter) Blocked(key string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	w := l.windows[key]
	return w != nil && !now.After(w.reset) && w.count >= l.max
}

// Fail records one failed attempt. Only failures count: a whole venue shares one
// NAT'd IP, so charging successes would lock the operator out partway through
// provisioning a rack of tablets with the correct PIN.
func (l *Limiter) Fail(key string) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	w := l.windows[key]
	if w == nil || now.After(w.reset) {
		l.windows[key] = &window{count: 1, reset: now.Add(l.period)}
		l.evictExpired(now)
		return
	}
	w.count++
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

func ClientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
