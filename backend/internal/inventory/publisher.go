package inventory

import (
	"context"
	"encoding/json"
	"time"
)

const NotifyChannel = "inventory_update"

// Publisher emits an inventory update to every pod, not just the one handling
// the write. Production issues a Postgres NOTIFY on the request tx; each pod's
// listener turns it back into a Hub.Publish.
type Publisher interface {
	Publish(ctx context.Context, u Update)
}

type PublisherFunc func(ctx context.Context, u Update)

func (f PublisherFunc) Publish(ctx context.Context, u Update) { f(ctx, u) }

// TenantID is json:"-" on the client-facing Update, so the cross-pod payload
// carries it explicitly to route to the right tenant's subscribers.
type wirePayload struct {
	TenantID  string    `json:"t"`
	ProductID string    `json:"p"`
	VariantID *string   `json:"v,omitempty"`
	NewStock  int       `json:"s"`
	Delta     int       `json:"d"`
	Timestamp time.Time `json:"ts"`
}

func MarshalUpdate(u Update) (string, error) {
	b, err := json.Marshal(wirePayload(u))
	return string(b), err
}

func ParseUpdate(payload string) (Update, error) {
	var w wirePayload
	if err := json.Unmarshal([]byte(payload), &w); err != nil {
		return Update{}, err
	}
	return Update(w), nil
}
