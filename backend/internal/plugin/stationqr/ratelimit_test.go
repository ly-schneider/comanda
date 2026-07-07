package stationqr

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRateLimiterAllowsUpToMaxThenBlocks(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(time.Minute, 3)
	l.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		require.Truef(t, l.allow("1.2.3.4"), "request %d within limit should pass", i)
	}
	require.False(t, l.allow("1.2.3.4"), "fourth request in the window is blocked")

	require.True(t, l.allow("5.6.7.8"))

	now = now.Add(time.Minute + time.Second)
	require.True(t, l.allow("1.2.3.4"), "the window has rolled over")
}
