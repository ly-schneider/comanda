package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/inventoryledger"
	"backend/internal/generated/ent/product"
	nanoid "backend/internal/id"
	"backend/internal/repository"
	"backend/internal/trace"
	"backend/internal/xlsx"
)

type InventoryExportScope string

const (
	InventoryScopeProduct     InventoryExportScope = "product"
	InventoryScopeSubcategory InventoryExportScope = "subcategory"
	InventoryScopeCategory    InventoryExportScope = "category"
)

var ErrInvalidExportScope = errors.New("invalid inventory export scope")

type InventoryExportService interface {
	Build(ctx context.Context, scope InventoryExportScope, scopeID string, from, to time.Time) (xlsx.InventoryReport, error)
}

type inventoryExportService struct {
	productRepo   *repository.ProductRepository
	categoryRepo  repository.CategoryRepository
	inventoryRepo repository.InventoryLedgerRepository
}

func NewInventoryExportService(
	productRepo *repository.ProductRepository,
	categoryRepo repository.CategoryRepository,
	inventoryRepo repository.InventoryLedgerRepository,
) InventoryExportService {
	return &inventoryExportService{
		productRepo:   productRepo,
		categoryRepo:  categoryRepo,
		inventoryRepo: inventoryRepo,
	}
}

var reasonLabels = map[inventoryledger.Reason]string{
	inventoryledger.ReasonOpeningBalance: "Eröffnung",
	inventoryledger.ReasonSale:           "Verkauf",
	inventoryledger.ReasonRefund:         "Erstattung",
	inventoryledger.ReasonCancellation:   "Stornierung",
	inventoryledger.ReasonManualAdjust:   "Manuelle Anpassung",
	inventoryledger.ReasonCorrection:     "Korrektur",
}

type ledgerClass int

const (
	classSold ledgerClass = iota
	classRefunded
	classCancelled
	classManual
	classReservation
)

// Deleting an abandoned order nulls order_id (ON DELETE SET NULL), so an unpaid
// reservation and its release are told apart by created_by instead: checkout never
// sets it, every manual adjustment does. Only pending orders are ever deleted, so a
// sale keeping its order_id means it was paid.
func classify(reason inventoryledger.Reason, hasOrder, hasCreatedBy bool) ledgerClass {
	switch reason {
	case inventoryledger.ReasonSale:
		if !hasOrder {
			return classReservation
		}
		return classSold
	case inventoryledger.ReasonRefund:
		return classRefunded
	case inventoryledger.ReasonCancellation:
		return classCancelled
	case inventoryledger.ReasonCorrection:
		if !hasCreatedBy {
			return classReservation
		}
		return classManual
	default:
		return classManual
	}
}

func reasonLabel(reason inventoryledger.Reason, class ledgerClass) string {
	if class == classReservation {
		if reason == inventoryledger.ReasonSale {
			return "Reservierung"
		}
		return "Reservierung verfallen"
	}
	if label, ok := reasonLabels[reason]; ok {
		return label
	}
	return string(reason)
}

func (s *inventoryExportService) Build(ctx context.Context, scope InventoryExportScope, scopeID string, from, to time.Time) (xlsx.InventoryReport, error) {
	ctx, finish := trace.StartSpan(ctx, "service", "inventory.build_export")
	defer finish()
	trace.Data(ctx, "export.scope", string(scope))
	trace.Data(ctx, "export.scope_id", scopeID)

	if !to.After(from) {
		return xlsx.InventoryReport{}, fmt.Errorf("%w: range end must be after start", ErrInvalidExportScope)
	}

	categories, err := s.categoryRepo.GetAll(ctx)
	if err != nil {
		return xlsx.InventoryReport{}, err
	}
	byID := make(map[string]*ent.Category, len(categories))
	for _, c := range categories {
		byID[c.ID] = c
	}

	products, scopeLabel, err := s.resolveScope(ctx, scope, scopeID, categories)
	if err != nil {
		return xlsx.InventoryReport{}, err
	}

	stocked := make([]*ent.Product, 0, len(products))
	for _, p := range products {
		if p.Type == product.TypeSimple {
			stocked = append(stocked, p)
		}
	}

	ids := make([]string, len(stocked))
	for i, p := range stocked {
		ids[i] = p.ID
	}

	openingByProduct, err := s.inventoryRepo.SumBeforeByProduct(ctx, ids, from)
	if err != nil {
		return xlsx.InventoryReport{}, err
	}
	openingByVariant, err := s.inventoryRepo.SumBeforeByVariant(ctx, ids, from)
	if err != nil {
		return xlsx.InventoryReport{}, err
	}
	closingByProduct, err := s.inventoryRepo.SumBeforeByProduct(ctx, ids, to)
	if err != nil {
		return xlsx.InventoryReport{}, err
	}
	closingByVariant, err := s.inventoryRepo.SumBeforeByVariant(ctx, ids, to)
	if err != nil {
		return xlsx.InventoryReport{}, err
	}

	entries, err := s.inventoryRepo.ListInRange(ctx, ids, from, to)
	if err != nil {
		return xlsx.InventoryReport{}, err
	}

	tallies := make(map[string]*movementTotals, len(ids))
	productByID := make(map[string]*ent.Product, len(stocked))
	for _, p := range stocked {
		productByID[p.ID] = p
	}

	movements := make([]xlsx.MovementRow, 0, len(entries))
	for _, e := range entries {
		class := classify(e.Reason, e.OrderID != nil, e.CreatedBy != nil)

		key := e.ProductID
		if e.VariantID != nil {
			key = *e.VariantID
		}
		if class != classReservation {
			t, ok := tallies[key]
			if !ok {
				t = &movementTotals{}
				tallies[key] = t
			}
			switch class {
			case classSold:
				t.sold -= e.Delta
			case classRefunded:
				t.refunded += e.Delta
			case classCancelled:
				t.cancelled += e.Delta
			case classManual:
				t.manual += e.Delta
			}
		}

		p := productByID[e.ProductID]
		productName := e.ProductID
		if p != nil {
			productName = p.Name
		}
		category, subcategory := categoryPath(p, byID)

		variantValue := ""
		if e.Edges.Variant != nil {
			variantValue = e.Edges.Variant.Value
		}
		orderID := ""
		if e.OrderID != nil {
			orderID = *e.OrderID
		}
		createdBy, createdByID := "", ""
		if e.CreatedBy != nil {
			createdByID = *e.CreatedBy
			createdBy = createdByID
		}
		if e.Edges.CreatedByUser != nil && e.Edges.CreatedByUser.Name != nil {
			createdBy = *e.Edges.CreatedByUser.Name
		}

		movements = append(movements, xlsx.MovementRow{
			At:          e.CreatedAt,
			Category:    category,
			Subcategory: subcategory,
			Product:     productName,
			Variant:     variantValue,
			Delta:       e.Delta,
			Reason:      reasonLabel(e.Reason, class),
			OrderID:     orderID,
			CreatedBy:   createdBy,
			CreatedByID: createdByID,
		})
	}

	summary := make([]xlsx.SummaryRow, 0, len(stocked))
	for _, p := range stocked {
		category, subcategory := categoryPath(p, byID)
		base := xlsx.SummaryRow{Category: category, Subcategory: subcategory, Product: p.Name}

		if len(p.Edges.Variants) == 0 {
			summary = append(summary, fillSummary(base, "", openingByProduct[p.ID], closingByProduct[p.ID], tallies[p.ID]))
			continue
		}
		for _, v := range p.Edges.Variants {
			summary = append(summary, fillSummary(base, v.Value, openingByVariant[v.ID], closingByVariant[v.ID], tallies[v.ID]))
		}
	}

	return xlsx.InventoryReport{
		ScopeLabel: scopeLabel,
		From:       from,
		To:         to,
		Summary:    summary,
		Movements:  movements,
	}, nil
}

type movementTotals struct{ sold, refunded, cancelled, manual int }

func fillSummary(row xlsx.SummaryRow, variant string, opening, closing int, t *movementTotals) xlsx.SummaryRow {
	row.Variant = variant
	row.Opening = opening
	row.Closing = closing
	if t != nil {
		row.Sold = t.sold
		row.Refunded = t.refunded
		row.Cancelled = t.cancelled
		row.Manual = t.manual
	}
	return row
}

// Categories nest exactly two levels.
func categoryPath(p *ent.Product, byID map[string]*ent.Category) (category, subcategory string) {
	if p == nil {
		return "", ""
	}
	cat := p.Edges.Category
	if cat == nil {
		cat = byID[p.CategoryID]
	}
	if cat == nil {
		return "", ""
	}
	if cat.ParentID == nil {
		return cat.Name, ""
	}
	if parent := byID[*cat.ParentID]; parent != nil {
		return parent.Name, cat.Name
	}
	return "", cat.Name
}

func (s *inventoryExportService) resolveScope(ctx context.Context, scope InventoryExportScope, scopeID string, categories []*ent.Category) ([]*ent.Product, string, error) {
	if !nanoid.Valid(scopeID) {
		return nil, "", fmt.Errorf("%w: invalid scopeId %q", ErrInvalidExportScope, scopeID)
	}

	switch scope {
	case InventoryScopeProduct:
		p, err := s.productRepo.GetByIDWithRelations(ctx, scopeID)
		if err != nil {
			return nil, "", err
		}
		return []*ent.Product{p}, p.Name, nil

	case InventoryScopeSubcategory:
		cat, err := s.categoryRepo.GetByID(ctx, scopeID)
		if err != nil {
			return nil, "", err
		}
		products, err := s.productRepo.GetByCategory(ctx, cat.ID)
		if err != nil {
			return nil, "", err
		}
		return products, cat.Name, nil

	case InventoryScopeCategory:
		cat, err := s.categoryRepo.GetByID(ctx, scopeID)
		if err != nil {
			return nil, "", err
		}
		ids := []string{cat.ID}
		for _, c := range categories {
			if c.ParentID != nil && *c.ParentID == cat.ID {
				ids = append(ids, c.ID)
			}
		}
		var products []*ent.Product
		for _, id := range ids {
			batch, err := s.productRepo.GetByCategory(ctx, id)
			if err != nil {
				return nil, "", err
			}
			products = append(products, batch...)
		}
		return products, cat.Name, nil

	default:
		return nil, "", fmt.Errorf("%w: %q", ErrInvalidExportScope, scope)
	}
}
