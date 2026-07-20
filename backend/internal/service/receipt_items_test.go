package service

import (
	"testing"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/orderline"
)

// A deposit line rides as a child (ParentLineID set) of its product line. It must
// nest under the product as a priced "Depot" sub-line (LEY-430) — not repeat the
// product name as a top-level line, and not lose its price like config children.
func TestBuildReceiptItemsNestsPricedDeposit(t *testing.T) {
	product := &ent.OrderLine{ID: "line_beer", Title: "Bier", Quantity: 2, UnitPriceCents: 500, LineType: orderline.LineTypeSimple}
	deposit := &ent.OrderLine{ID: "line_dep", Title: "Depot — Bier", Quantity: 2, UnitPriceCents: 200, LineType: orderline.LineTypeDeposit, ParentLineID: strptr("line_beer")}

	items := buildReceiptItems([]*ent.OrderLine{product, deposit})

	if len(items) != 1 {
		t.Fatalf("deposit must nest, not add a top-level line; got %d lines", len(items))
	}
	if len(items[0].Children) != 1 {
		t.Fatalf("want 1 nested deposit child, got %+v", items[0].Children)
	}
	dep := items[0].Children[0]
	if dep.Title != "Depot" || dep.Cents != 200 || dep.Quantity != 2 {
		t.Fatalf("deposit child should be a priced 'Depot' line: %+v", dep)
	}
}

// Genuine bundle/config children stay nested as title-only sub-lines.
func TestBuildReceiptItemsKeepsConfigChildren(t *testing.T) {
	product := &ent.OrderLine{ID: "line_fries", Title: "Pommes", Quantity: 1, UnitPriceCents: 500, LineType: orderline.LineTypeBundle}
	extra := &ent.OrderLine{ID: "line_sauce", Title: "Extra Sauce", Quantity: 1, LineType: orderline.LineTypeComponent, ParentLineID: strptr("line_fries")}

	items := buildReceiptItems([]*ent.OrderLine{product, extra})

	if len(items) != 1 {
		t.Fatalf("config child must not become a top-level line; got %d lines", len(items))
	}
	if len(items[0].Children) != 1 || items[0].Children[0].Title != "Extra Sauce" {
		t.Fatalf("config child not nested under parent: %+v", items[0])
	}
	if items[0].Children[0].Cents != 0 {
		t.Fatalf("config child must stay title-only (no price), got %+v", items[0].Children[0])
	}
}
