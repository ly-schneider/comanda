package repository

import (
	"context"
	"testing"

	"backend/internal/inventory"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestInventoryNotifier_EmptyTenantIsNoop(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	NewInventoryNotifier(zap.New(core)).Publish(context.Background(), inventory.Update{ProductID: "p"})

	if logs.Len() != 0 {
		t.Fatalf("empty-tenant publish should be a silent no-op, got %d logs", logs.Len())
	}
}

func TestInventoryNotifier_MissingRequestTxWarnsAndDrops(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	NewInventoryNotifier(zap.New(core)).Publish(
		context.Background(), inventory.Update{TenantID: "t", ProductID: "prod123"})

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected one warn when no request tx is bound, got %d", len(entries))
	}
	if got := entries[0].ContextMap()["productId"]; got != "prod123" {
		t.Fatalf("warn missing productId field, got %v", got)
	}
}
