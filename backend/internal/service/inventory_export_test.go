package service

import (
	"testing"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/inventoryledger"
)

func TestClassifySeparatesSalesFromExpiredReservations(t *testing.T) {
	cases := []struct {
		name         string
		reason       inventoryledger.Reason
		hasOrder     bool
		hasCreatedBy bool
		want         ledgerClass
	}{
		{"paid sale keeps its order", inventoryledger.ReasonSale, true, false, classSold},
		{"abandoned checkout lost its order", inventoryledger.ReasonSale, false, false, classReservation},
		{"reservation release has no author", inventoryledger.ReasonCorrection, false, false, classReservation},
		{"staff correction has an author", inventoryledger.ReasonCorrection, false, true, classManual},
		{"manual adjust", inventoryledger.ReasonManualAdjust, false, true, classManual},
		{"opening balance counts as manual", inventoryledger.ReasonOpeningBalance, false, true, classManual},
		{"refund", inventoryledger.ReasonRefund, true, false, classRefunded},
		{"cancellation", inventoryledger.ReasonCancellation, true, false, classCancelled},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classify(c.reason, c.hasOrder, c.hasCreatedBy); got != c.want {
				t.Fatalf("classify(%s, order=%v, author=%v) = %d, want %d", c.reason, c.hasOrder, c.hasCreatedBy, got, c.want)
			}
		})
	}
}

func TestReasonLabelMarksReservations(t *testing.T) {
	if got := reasonLabel(inventoryledger.ReasonSale, classReservation); got != "Reservierung" {
		t.Fatalf("unpaid reservation label = %q", got)
	}
	if got := reasonLabel(inventoryledger.ReasonCorrection, classReservation); got != "Reservierung verfallen" {
		t.Fatalf("release label = %q", got)
	}
	if got := reasonLabel(inventoryledger.ReasonSale, classSold); got != "Verkauf" {
		t.Fatalf("paid sale label = %q", got)
	}
}

func TestCategoryPathResolvesTwoLevels(t *testing.T) {
	parentID := "cat000000001"
	parent := &ent.Category{ID: parentID, Name: "Getränke"}
	child := &ent.Category{ID: "cat000000002", Name: "Bar Ost", ParentID: &parentID}
	byID := map[string]*ent.Category{parent.ID: parent, child.ID: child}

	onChild := &ent.Product{ID: "prod00000001", CategoryID: child.ID}
	onChild.Edges.Category = child
	if cat, sub := categoryPath(onChild, byID); cat != "Getränke" || sub != "Bar Ost" {
		t.Fatalf("subcategory product = (%q, %q)", cat, sub)
	}

	onParent := &ent.Product{ID: "prod00000002", CategoryID: parent.ID}
	onParent.Edges.Category = parent
	if cat, sub := categoryPath(onParent, byID); cat != "Getränke" || sub != "" {
		t.Fatalf("top-level product = (%q, %q)", cat, sub)
	}

	// Edge not loaded — fall back to the id map.
	unloaded := &ent.Product{ID: "prod00000003", CategoryID: child.ID}
	if cat, sub := categoryPath(unloaded, byID); cat != "Getränke" || sub != "Bar Ost" {
		t.Fatalf("unloaded edge = (%q, %q)", cat, sub)
	}

	if cat, sub := categoryPath(nil, byID); cat != "" || sub != "" {
		t.Fatalf("nil product must be blank, got (%q, %q)", cat, sub)
	}
}
