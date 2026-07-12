package api

import (
	"context"
	"encoding/json"
	"net/http"

	"backend/internal/auth"
	nanoid "backend/internal/id"
)

func (h *Handlers) StreamInventory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	rc := http.NewResponseController(w)

	tenantID, ok := auth.GetTenantID(r.Context())
	if !ok {
		_, _ = w.Write([]byte("event: error\ndata: {\"error\":\"no tenant\"}\n\n"))
		_ = rc.Flush()
		return
	}

	// Snapshot in a short tx and release the connection before streaming — a
	// long-lived SSE holding a pooled tx would exhaust the pool. Live updates
	// come from the in-memory Hub, which holds no DB connection.
	snapshot, err := h.inventorySnapshot(r.Context(), tenantID)
	if err != nil {
		_, _ = w.Write([]byte("event: error\ndata: {\"error\":\"failed to load stock\"}\n\n"))
		_ = rc.Flush()
		return
	}

	data, _ := json.Marshal(snapshot)
	_, _ = w.Write(append(append([]byte("event: inventory-snapshot\ndata: "), data...), '\n', '\n'))
	_ = rc.Flush()

	subID := nanoid.New()
	ch := h.inventoryHub.Subscribe(subID, tenantID)
	defer h.inventoryHub.Unsubscribe(subID)

	ctx := r.Context()
	for {
		select {
		case update, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(update)
			_, _ = w.Write(append(append([]byte("event: inventory-update\ndata: "), data...), '\n', '\n'))
			_ = rc.Flush()
		case <-ctx.Done():
			return
		}
	}
}

func (h *Handlers) inventorySnapshot(ctx context.Context, tenantID string) (map[string]int, error) {
	snapCtx, finalize, err := h.beginTenantTx(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			finalize(false)
		}
	}()

	products, err := h.products.GetAll(snapCtx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(products))
	for i, p := range products {
		ids[i] = p.ID
	}
	stocks, err := h.products.GetStockBatch(snapCtx, ids)
	if err != nil {
		return nil, err
	}
	// Variant ids join the snapshot map; the nanoid key spaces are disjoint.
	variantStocks, err := h.products.GetVariantStockBatch(snapCtx, ids)
	if err != nil {
		return nil, err
	}

	snapshot := make(map[string]int, len(stocks)+len(variantStocks))
	for id, stock := range stocks {
		snapshot[id] = stock
	}
	for _, p := range products {
		for _, v := range p.Edges.Variants {
			snapshot[v.ID] = variantStocks[v.ID]
		}
	}

	finalize(true)
	committed = true
	return snapshot, nil
}
