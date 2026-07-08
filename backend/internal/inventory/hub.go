package inventory

import (
	"sync"
	"time"
)

type Update struct {
	TenantID  string    `json:"-"`
	ProductID string    `json:"productId"`
	VariantID *string   `json:"variantId,omitempty"`
	NewStock  int       `json:"newStock"`
	Delta     int       `json:"delta"`
	Timestamp time.Time `json:"timestamp"`
}

type subscriber struct {
	tenantID string
	ch       chan Update
}

type Hub struct {
	mu          sync.RWMutex
	subscribers map[string]subscriber
}

func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[string]subscriber),
	}
}

func (h *Hub) Subscribe(id, tenantID string) <-chan Update {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan Update, 64)
	h.subscribers[id] = subscriber{tenantID: tenantID, ch: ch}
	return ch
}

func (h *Hub) Unsubscribe(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if sub, ok := h.subscribers[id]; ok {
		close(sub.ch)
		delete(h.subscribers, id)
	}
}

func (h *Hub) Publish(update Update) {
	if update.TenantID == "" {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, sub := range h.subscribers {
		if sub.tenantID != update.TenantID {
			continue
		}
		select {
		case sub.ch <- update:
		default:
		}
	}
}

func (h *Hub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers)
}
