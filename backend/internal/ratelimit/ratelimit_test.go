package ratelimit

import (
	"testing"
	"time"
)

func TestAllowWithinWindow(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	l := New(5*time.Minute, 3)
	l.now = func() time.Time { return base }

	for i := range 3 {
		if !l.Allow("1.2.3.4") {
			t.Fatalf("attempt %d denied inside the window", i+1)
		}
	}
	if l.Allow("1.2.3.4") {
		t.Error("fourth attempt allowed past the limit")
	}
	if !l.Allow("5.6.7.8") {
		t.Error("a different client hit another client's window")
	}
}

func TestWindowResets(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	now := base
	l := New(time.Minute, 1)
	l.now = func() time.Time { return now }

	if !l.Allow("1.2.3.4") {
		t.Fatal("first attempt denied")
	}
	if l.Allow("1.2.3.4") {
		t.Fatal("second attempt allowed inside the window")
	}

	now = base.Add(61 * time.Second)
	if !l.Allow("1.2.3.4") {
		t.Error("attempt denied after the window expired")
	}
}
