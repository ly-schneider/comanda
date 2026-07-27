package service

import (
	"testing"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/inventoryledger"
	"backend/internal/generated/ent/product"
)

func simpleLine(id, productID string, qty int, variantID *string) *ent.OrderLine {
	line := &ent.OrderLine{ID: id, ProductID: productID, Quantity: qty, VariantID: variantID}
	line.Edges.Product = &ent.Product{ID: productID, Type: product.TypeSimple}
	return line
}

func TestRestoreEntriesCarriesVariantID(t *testing.T) {
	variantID := "var123456789"
	lines := []*ent.OrderLine{
		simpleLine("line00000001", "prod00000001", 2, &variantID),
		simpleLine("line00000002", "prod00000002", 1, nil),
	}

	entries := restoreEntries(lines, "ord000000001", inventoryledger.ReasonCancellation)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].VariantID == nil || *entries[0].VariantID != variantID {
		t.Fatalf("variant line must restore onto the variant, got %v", entries[0].VariantID)
	}
	if entries[1].VariantID != nil {
		t.Fatalf("non-variant line must stay product-level, got %v", entries[1].VariantID)
	}
	if entries[0].Delta != 2 || entries[1].Delta != 1 {
		t.Fatalf("delta must be the positive line quantity, got %d and %d", entries[0].Delta, entries[1].Delta)
	}
	if entries[0].Reason != inventoryledger.ReasonCancellation {
		t.Fatalf("reason must pass through, got %s", entries[0].Reason)
	}
	if entries[0].OrderID == nil || *entries[0].OrderID != "ord000000001" {
		t.Fatalf("order id must be set, got %v", entries[0].OrderID)
	}
	if entries[0].OrderLineID == nil || *entries[0].OrderLineID != "line00000001" {
		t.Fatalf("order line id must point at its own line, got %v", entries[0].OrderLineID)
	}
}

func TestRestoreEntriesSkipsNonRestorableLines(t *testing.T) {
	bundleLine := &ent.OrderLine{ID: "line00000003", ProductID: "prod00000003", Quantity: 1}
	bundleLine.Edges.Product = &ent.Product{ID: "prod00000003", Type: product.TypeBundle}

	noProductLine := &ent.OrderLine{ID: "line00000004", ProductID: "prod00000004", Quantity: 1}

	lines := []*ent.OrderLine{
		bundleLine,
		noProductLine,
		simpleLine("line00000005", "prod00000005", 0, nil),
	}

	if entries := restoreEntries(lines, "ord000000001", inventoryledger.ReasonRefund); len(entries) != 0 {
		t.Fatalf("bundle, unloaded-product and zero-quantity lines must not restore, got %d entries", len(entries))
	}
}
