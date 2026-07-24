package service

import (
	"testing"
	"time"
)

func TestReconcileGuardCooldownPerOrder(t *testing.T) {
	g := newReconcileGuard()
	now := time.Now()

	release, ok := g.tryBegin("order1", now)
	if !ok {
		t.Fatal("first pull must be allowed")
	}
	release()

	if _, ok := g.tryBegin("order1", now.Add(reconcilePullInterval/2)); ok {
		t.Error("a second pull inside the cooldown must be declined")
	}
	if _, ok := g.tryBegin("order2", now.Add(reconcilePullInterval/2)); !ok {
		t.Error("a different order must not be blocked by another order's cooldown")
	}
	if _, ok := g.tryBegin("order1", now.Add(reconcilePullInterval*2)); !ok {
		t.Error("pull must be allowed again after the cooldown")
	}
}

func TestReconcileGuardCapsConcurrency(t *testing.T) {
	g := newReconcileGuard()
	now := time.Now()

	releases := make([]func(), 0, reconcileMaxInFlight)
	for i := range reconcileMaxInFlight {
		release, ok := g.tryBegin(string(rune('a'+i)), now)
		if !ok {
			t.Fatalf("pull %d must fit within the in-flight cap", i)
		}
		releases = append(releases, release)
	}

	if _, ok := g.tryBegin("overflow", now); ok {
		t.Error("pull beyond the in-flight cap must be declined, not queued")
	}

	releases[0]()
	if _, ok := g.tryBegin("overflow", now); !ok {
		t.Error("a freed slot must admit the next pull")
	}
}

func TestReconcileGuardPrunesStaleEntries(t *testing.T) {
	g := newReconcileGuard()
	start := time.Now()

	release, _ := g.tryBegin("old", start)
	release()

	release, ok := g.tryBegin("new", start.Add(20*reconcilePullInterval))
	if !ok {
		t.Fatal("later pull must be allowed")
	}
	release()

	if _, tracked := g.last["old"]; tracked {
		t.Error("stale cooldown entry must be pruned")
	}
}
