package ratelimit

import (
	"testing"
	"time"
)

func TestBlocksAfterMaxFailures(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	l := New(5*time.Minute, 3)
	l.now = func() time.Time { return base }

	for i := range 3 {
		if l.Blocked("1.2.3.4") {
			t.Fatalf("blocked after only %d failures", i)
		}
		l.Fail("1.2.3.4")
	}
	if !l.Blocked("1.2.3.4") {
		t.Error("not blocked after exhausting the budget")
	}
	if l.Blocked("5.6.7.8") {
		t.Error("a different client hit another client's window")
	}
}

// A venue NATs every tablet behind one IP, so provisioning a rack with the
// correct PIN must never consume budget.
func TestSuccessesDoNotConsumeBudget(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	l := New(5*time.Minute, 3)
	l.now = func() time.Time { return base }

	for range 50 {
		if l.Blocked("1.2.3.4") {
			t.Fatal("blocked without a single failure")
		}
	}
}

func TestWindowResets(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	now := base
	l := New(time.Minute, 1)
	l.now = func() time.Time { return now }

	l.Fail("1.2.3.4")
	if !l.Blocked("1.2.3.4") {
		t.Fatal("not blocked inside the window")
	}

	now = base.Add(61 * time.Second)
	if l.Blocked("1.2.3.4") {
		t.Error("still blocked after the window expired")
	}
}
