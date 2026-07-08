package inventory

import (
	"testing"
	"time"
)

func recv(t *testing.T, ch <-chan Update) (Update, bool) {
	t.Helper()
	select {
	case u, ok := <-ch:
		return u, ok
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for update")
		return Update{}, false
	}
}

func TestHub_PublishIsTenantScoped(t *testing.T) {
	h := NewHub()
	chA := h.Subscribe("a", "tenant-a")
	chB := h.Subscribe("b", "tenant-b")

	h.Publish(Update{TenantID: "tenant-a", ProductID: "p1", NewStock: 3})

	got, _ := recv(t, chA)
	if got.ProductID != "p1" {
		t.Fatalf("tenant A got %q, want p1", got.ProductID)
	}

	select {
	case leaked := <-chB:
		t.Fatalf("tenant B leaked tenant A update: %+v", leaked)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHub_DropsUntaggedUpdate(t *testing.T) {
	h := NewHub()
	ch := h.Subscribe("a", "tenant-a")

	h.Publish(Update{ProductID: "p1"})

	select {
	case leaked := <-ch:
		t.Fatalf("untagged update was delivered: %+v", leaked)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHub_UnsubscribeClosesChannel(t *testing.T) {
	h := NewHub()
	ch := h.Subscribe("a", "tenant-a")
	h.Unsubscribe("a")

	if _, ok := recv(t, ch); ok {
		t.Fatal("channel should be closed after Unsubscribe")
	}
	if h.SubscriberCount() != 0 {
		t.Fatalf("SubscriberCount = %d, want 0", h.SubscriberCount())
	}
}
