package service

import (
	"context"
	"fmt"
	"time"

	"backend/internal/auth"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/inventoryledger"
	"backend/internal/generated/ent/product"
	"backend/internal/inventory"
	"backend/internal/repository"

	nanoid "backend/internal/id"
)

type ProductService interface {
	ListProducts(ctx context.Context, categoryID *string, limit, offset int) ([]*ent.Product, error)
	GetByID(ctx context.Context, id string) (*ent.Product, error)
	GetByIDWithRelations(ctx context.Context, id string) (*ent.Product, error)
	GetAll(ctx context.Context) ([]*ent.Product, error)
	GetByCategory(ctx context.Context, categoryID string) ([]*ent.Product, error)
	GetByIDs(ctx context.Context, ids []string) ([]*ent.Product, error)
	Create(ctx context.Context, categoryID string, productType product.Type, name string, priceCents, depositCents int64, isActive bool, image *string, description *string, jetonID *string, variantLabel *string, variants []VariantInput) (*ent.Product, error)
	Update(ctx context.Context, id, categoryID string, productType product.Type, name string, priceCents, depositCents int64, isActive bool, image *string, description *string, jetonID *string, variantLabel *string, variants []VariantInput) (*ent.Product, error)
	Delete(ctx context.Context, id string) error
	CountActiveWithoutJeton(ctx context.Context) (int64, error)
	CountByJetonIDs(ctx context.Context, ids []string) (map[string]int64, error)
	UpdateJeton(ctx context.Context, id string, jetonID *string) error

	// Inventory
	GetStock(ctx context.Context, id string) (int64, error)
	GetStockBatch(ctx context.Context, ids []string) (map[string]int, error)
	GetVariantStockBatch(ctx context.Context, ids []string) (map[string]int, error)
	AdjustStock(ctx context.Context, id string, variantID *string, delta int64, reason string) error
	ListInventoryHistory(ctx context.Context, productID string, limit, offset int) ([]*ent.InventoryLedger, error)

	// Menus
	GetMenus(ctx context.Context) ([]*ent.Product, error)

	// Menu slots
	CreateMenuSlot(ctx context.Context, menuID string, name string) (*ent.MenuSlot, error)
	UpdateMenuSlot(ctx context.Context, menuID, slotID string, name string) (*ent.MenuSlot, error)
	DeleteMenuSlot(ctx context.Context, menuID, slotID string) error
	ReorderMenuSlots(ctx context.Context, menuID string, positions map[string]int) error

	// Menu slot options
	AddSlotOption(ctx context.Context, menuID, slotID string, productID string) (*ent.MenuSlotOption, error)
	RemoveSlotOption(ctx context.Context, menuID, slotID, optionProductID string) error
}

// VariantInput describes one variant in a create/update payload. Entries with
// an ID update the existing row; entries without one are created; existing
// rows absent from the payload are deleted, or archived when order lines or
// ledger rows still reference them.
type VariantInput struct {
	ID                   *string
	Value                string
	PriceAdjustmentCents int64
	IsActive             bool
	Sequence             int
}

type productService struct {
	productRepo        *repository.ProductRepository
	variantRepo        repository.ProductVariantRepository
	categoryRepo       repository.CategoryRepository
	menuSlotRepo       repository.MenuSlotRepository
	menuSlotOptionRepo repository.MenuSlotOptionRepository
	inventoryRepo      repository.InventoryLedgerRepository
	jetonRepo          repository.JetonRepository
	inventoryHub       *inventory.Hub
	cache              *catalogCache
}

func NewProductService(
	productRepo *repository.ProductRepository,
	variantRepo repository.ProductVariantRepository,
	categoryRepo repository.CategoryRepository,
	menuSlotRepo repository.MenuSlotRepository,
	menuSlotOptionRepo repository.MenuSlotOptionRepository,
	inventoryRepo repository.InventoryLedgerRepository,
	jetonRepo repository.JetonRepository,
	inventoryHub *inventory.Hub,
) ProductService {
	return &productService{
		productRepo:        productRepo,
		variantRepo:        variantRepo,
		categoryRepo:       categoryRepo,
		menuSlotRepo:       menuSlotRepo,
		menuSlotOptionRepo: menuSlotOptionRepo,
		inventoryRepo:      inventoryRepo,
		jetonRepo:          jetonRepo,
		inventoryHub:       inventoryHub,
		cache:              newCatalogCache(),
	}
}

func (s *productService) ListProducts(ctx context.Context, categoryID *string, limit, offset int) ([]*ent.Product, error) {
	if categoryID != nil {
		catID := *categoryID
		if !nanoid.Valid(catID) {
			return nil, fmt.Errorf("invalid category id: %s", catID)
		}
		return s.cache.load(ctx, catalogKeyByCategory(catID), func(ctx context.Context) ([]*ent.Product, error) {
			return s.productRepo.GetByCategory(ctx, catID)
		})
	}
	return s.cache.load(ctx, catalogKeyAll(), func(ctx context.Context) ([]*ent.Product, error) {
		return s.productRepo.GetAll(ctx)
	})
}

func (s *productService) GetByID(ctx context.Context, id string) (*ent.Product, error) {
	return s.productRepo.GetByID(ctx, id)
}

func (s *productService) GetByIDWithRelations(ctx context.Context, id string) (*ent.Product, error) {
	return s.productRepo.GetByIDWithRelations(ctx, id)
}

func (s *productService) GetAll(ctx context.Context) ([]*ent.Product, error) {
	return s.cache.load(ctx, catalogKeyAll(), func(ctx context.Context) ([]*ent.Product, error) {
		return s.productRepo.GetAll(ctx)
	})
}

func (s *productService) GetByCategory(ctx context.Context, categoryID string) ([]*ent.Product, error) {
	return s.cache.load(ctx, catalogKeyByCategory(categoryID), func(ctx context.Context) ([]*ent.Product, error) {
		return s.productRepo.GetByCategory(ctx, categoryID)
	})
}

func (s *productService) GetByIDs(ctx context.Context, ids []string) ([]*ent.Product, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	all, err := s.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*ent.Product, len(all))
	for _, p := range all {
		byID[p.ID] = p
	}
	result := make([]*ent.Product, 0, len(ids))
	missing := make([]string, 0)
	for _, id := range ids {
		if p, ok := byID[id]; ok {
			result = append(result, p)
			continue
		}
		missing = append(missing, id)
	}
	if len(missing) > 0 {
		fetched, err := s.productRepo.GetByIDs(ctx, missing)
		if err != nil {
			return nil, err
		}
		result = append(result, fetched...)
	}
	return result, nil
}

func (s *productService) Create(ctx context.Context, categoryID string, productType product.Type, name string, priceCents, depositCents int64, isActive bool, image *string, description *string, jetonID *string, variantLabel *string, variants []VariantInput) (*ent.Product, error) {
	if err := validateVariantInputs(productType, variants); err != nil {
		return nil, err
	}
	created, err := s.productRepo.Create(ctx, categoryID, productType, name, priceCents, depositCents, isActive, image, description, jetonID, variantLabel)
	if err != nil {
		return nil, err
	}
	if len(variants) > 0 {
		if err := s.reconcileVariants(ctx, created.ID, variants); err != nil {
			return nil, err
		}
	}
	s.cache.invalidate()
	return s.productRepo.GetByIDWithRelations(ctx, created.ID)
}

func (s *productService) Update(ctx context.Context, id, categoryID string, productType product.Type, name string, priceCents, depositCents int64, isActive bool, image *string, description *string, jetonID *string, variantLabel *string, variants []VariantInput) (*ent.Product, error) {
	if variants != nil {
		if err := validateVariantInputs(productType, variants); err != nil {
			return nil, err
		}
	}
	updated, err := s.productRepo.Update(ctx, id, categoryID, productType, name, priceCents, depositCents, isActive, image, description, jetonID, variantLabel)
	if err != nil {
		return nil, err
	}
	// nil leaves existing variants untouched; an empty slice removes them all.
	if variants != nil {
		if err := s.reconcileVariants(ctx, id, variants); err != nil {
			return nil, err
		}
	}
	s.cache.invalidate()
	return s.productRepo.GetByIDWithRelations(ctx, updated.ID)
}

func validateVariantInputs(productType product.Type, variants []VariantInput) error {
	if len(variants) == 0 {
		return nil
	}
	if productType != product.TypeSimple {
		return fmt.Errorf("variants are only allowed on simple products")
	}
	seen := make(map[string]struct{}, len(variants))
	for _, v := range variants {
		if v.Value == "" {
			return fmt.Errorf("variant value must not be empty")
		}
		if len(v.Value) > 20 {
			return fmt.Errorf("variant value exceeds 20 characters: %s", v.Value)
		}
		if _, dup := seen[v.Value]; dup {
			return fmt.Errorf("duplicate variant value: %s", v.Value)
		}
		seen[v.Value] = struct{}{}
	}
	return nil
}

// reconcileVariants applies the desired variant set: inputs with an ID update,
// inputs without one create, and existing rows missing from the payload are
// deleted — or archived when order lines or ledger rows still reference them.
func (s *productService) reconcileVariants(ctx context.Context, productID string, variants []VariantInput) error {
	if len(variants) > 0 {
		usedAsOption, err := s.menuSlotOptionRepo.CountByOptionProductID(ctx, productID)
		if err != nil {
			return err
		}
		if usedAsOption > 0 {
			return fmt.Errorf("products used as menu options cannot have variants")
		}
	}

	existing, err := s.variantRepo.GetByProductID(ctx, productID)
	if err != nil {
		return err
	}
	existingByID := make(map[string]*ent.ProductVariant, len(existing))
	for _, v := range existing {
		existingByID[v.ID] = v
	}

	kept := make(map[string]struct{}, len(variants))
	for _, in := range variants {
		if in.ID != nil {
			cur, ok := existingByID[*in.ID]
			if !ok {
				return fmt.Errorf("variant does not belong to product: %s", *in.ID)
			}
			kept[cur.ID] = struct{}{}
			if _, err := s.variantRepo.Update(ctx, cur.ID, in.Value, in.PriceAdjustmentCents, in.IsActive, in.Sequence); err != nil {
				return err
			}
			continue
		}
		if _, err := s.variantRepo.Create(ctx, productID, in.Value, in.PriceAdjustmentCents, in.IsActive, in.Sequence); err != nil {
			return err
		}
	}

	for _, cur := range existing {
		if _, ok := kept[cur.ID]; ok {
			continue
		}
		referenced, err := s.variantRepo.IsReferenced(ctx, cur.ID)
		if err != nil {
			return err
		}
		if referenced {
			if _, err := s.variantRepo.Update(ctx, cur.ID, cur.Value, cur.PriceAdjustmentCents, false, cur.Sequence); err != nil {
				return err
			}
			continue
		}
		if err := s.variantRepo.Delete(ctx, cur.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *productService) Delete(ctx context.Context, id string) error {
	slots, _ := s.menuSlotRepo.GetByMenuProductID(ctx, id)
	for _, slot := range slots {
		_ = s.menuSlotOptionRepo.DeleteByMenuSlotID(ctx, slot.ID)
	}
	if len(slots) > 0 {
		_ = s.menuSlotRepo.DeleteByMenuProductID(ctx, id)
	}
	variants, _ := s.variantRepo.GetByProductID(ctx, id)
	for _, v := range variants {
		if err := s.variantRepo.Delete(ctx, v.ID); err != nil {
			return err
		}
	}
	err := s.productRepo.Delete(ctx, id)
	if err == nil {
		s.cache.invalidate()
	}
	return err
}

func (s *productService) CountActiveWithoutJeton(ctx context.Context) (int64, error) {
	return s.productRepo.CountActiveWithoutJeton(ctx)
}

func (s *productService) CountByJetonIDs(ctx context.Context, ids []string) (map[string]int64, error) {
	return s.productRepo.CountByJetonIDs(ctx, ids)
}

func (s *productService) UpdateJeton(ctx context.Context, id string, jetonID *string) error {
	err := s.productRepo.UpdateJeton(ctx, id, jetonID)
	if err == nil {
		s.cache.invalidate()
	}
	return err
}

// ---------------------------------------------------------------------------
// Inventory
// ---------------------------------------------------------------------------

func (s *productService) GetStock(ctx context.Context, id string) (int64, error) {
	stock, err := s.inventoryRepo.GetCurrentStock(ctx, id)
	if err != nil {
		return 0, err
	}
	return int64(stock), nil
}

func (s *productService) GetStockBatch(ctx context.Context, ids []string) (map[string]int, error) {
	return s.inventoryRepo.GetCurrentStockBatch(ctx, ids)
}

func (s *productService) GetVariantStockBatch(ctx context.Context, ids []string) (map[string]int, error) {
	return s.inventoryRepo.GetCurrentStockByVariantBatch(ctx, ids)
}

func (s *productService) ListInventoryHistory(ctx context.Context, productID string, limit, offset int) ([]*ent.InventoryLedger, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	return s.inventoryRepo.GetByProductIDWithPagination(ctx, productID, limit, offset)
}

func (s *productService) AdjustStock(ctx context.Context, id string, variantID *string, delta int64, reason string) error {
	hasVariants, err := s.variantRepo.ExistsForProduct(ctx, id)
	if err != nil {
		return err
	}
	if hasVariants && variantID == nil {
		return fmt.Errorf("variantId is required for products with variants")
	}
	if !hasVariants && variantID != nil {
		return fmt.Errorf("variantId is not allowed for products without variants")
	}
	if variantID != nil {
		variant, err := s.variantRepo.GetByID(ctx, *variantID)
		if err != nil {
			return err
		}
		if variant.ProductID != id {
			return fmt.Errorf("variant does not belong to product: %s", *variantID)
		}
	}

	r := inventoryledger.ReasonManualAdjust
	switch reason {
	case string(inventoryledger.ReasonOpeningBalance):
		r = inventoryledger.ReasonOpeningBalance
	case string(inventoryledger.ReasonSale):
		r = inventoryledger.ReasonSale
	case string(inventoryledger.ReasonRefund):
		r = inventoryledger.ReasonRefund
	case string(inventoryledger.ReasonCorrection):
		r = inventoryledger.ReasonCorrection
	case string(inventoryledger.ReasonManualAdjust):
		r = inventoryledger.ReasonManualAdjust
	}
	var createdBy *string
	if uid, ok := auth.GetUserID(ctx); ok {
		createdBy = &uid
	}
	if _, err := s.inventoryRepo.Create(ctx, id, int(delta), r, nil, nil, nil, createdBy, variantID); err != nil {
		return err
	}
	if s.inventoryHub != nil {
		newStock := 0
		var stockErr error
		if variantID != nil {
			variantStocks, err := s.inventoryRepo.GetCurrentStockByVariantBatch(ctx, []string{id})
			newStock, stockErr = variantStocks[*variantID], err
		} else {
			newStock, stockErr = s.inventoryRepo.GetCurrentStock(ctx, id)
		}
		if stockErr == nil {
			s.inventoryHub.Publish(inventory.Update{
				ProductID: id,
				VariantID: variantID,
				NewStock:  newStock,
				Delta:     int(delta),
				Timestamp: time.Now(),
			})
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Menus
// ---------------------------------------------------------------------------

func (s *productService) GetMenus(ctx context.Context) ([]*ent.Product, error) {
	menus, _, err := s.productRepo.GetMenus(ctx, nil, nil, 1000, 0)
	return menus, err
}

// ---------------------------------------------------------------------------
// Menu Slots
// ---------------------------------------------------------------------------

func (s *productService) CreateMenuSlot(ctx context.Context, menuID string, name string) (*ent.MenuSlot, error) {
	existing, err := s.menuSlotRepo.GetByMenuProductID(ctx, menuID)
	if err != nil {
		return nil, err
	}
	seq := 0
	for _, slot := range existing {
		if slot.Sequence >= seq {
			seq = slot.Sequence + 1
		}
	}
	created, err := s.menuSlotRepo.Create(ctx, menuID, name, seq)
	if err == nil {
		s.cache.invalidate()
	}
	return created, err
}

func (s *productService) UpdateMenuSlot(ctx context.Context, menuID, slotID string, name string) (*ent.MenuSlot, error) {
	slot, err := s.menuSlotRepo.GetByID(ctx, slotID)
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = slot.Name
	}
	updated, err := s.menuSlotRepo.Update(ctx, slotID, menuID, name, slot.Sequence)
	if err == nil {
		s.cache.invalidate()
	}
	return updated, err
}

func (s *productService) DeleteMenuSlot(ctx context.Context, menuID, slotID string) error {
	// Verify the slot belongs to this menu.
	slot, err := s.menuSlotRepo.GetByID(ctx, slotID)
	if err != nil {
		return err
	}
	if slot.MenuProductID != menuID {
		return fmt.Errorf("slot does not belong to menu")
	}
	// Delete options first, then the slot.
	if err := s.menuSlotOptionRepo.DeleteByMenuSlotID(ctx, slotID); err != nil {
		return err
	}
	if err := s.menuSlotRepo.Delete(ctx, slotID); err != nil {
		return err
	}
	// Re-normalize remaining slot sequences to eliminate gaps.
	remaining, err := s.menuSlotRepo.GetByMenuProductID(ctx, menuID)
	if err != nil {
		return err
	}
	for i, sl := range remaining {
		if sl.Sequence != i {
			if _, err := s.menuSlotRepo.Update(ctx, sl.ID, menuID, sl.Name, i); err != nil {
				return err
			}
		}
	}
	s.cache.invalidate()
	return nil
}

func (s *productService) ReorderMenuSlots(ctx context.Context, menuID string, positions map[string]int) error {
	slots, err := s.menuSlotRepo.GetByMenuProductID(ctx, menuID)
	if err != nil {
		return err
	}
	for _, slot := range slots {
		if newSeq, ok := positions[slot.ID]; ok {
			if _, err := s.menuSlotRepo.Update(ctx, slot.ID, menuID, slot.Name, newSeq); err != nil {
				return err
			}
		}
	}
	s.cache.invalidate()
	return nil
}

func (s *productService) AddSlotOption(ctx context.Context, menuID, slotID string, productID string) (*ent.MenuSlotOption, error) {
	hasVariants, err := s.variantRepo.ExistsForProduct(ctx, productID)
	if err != nil {
		return nil, err
	}
	if hasVariants {
		return nil, fmt.Errorf("products with variants cannot be menu options")
	}
	opt, err := s.menuSlotOptionRepo.Create(ctx, slotID, productID)
	if err == nil {
		s.cache.invalidate()
	}
	return opt, err
}

func (s *productService) RemoveSlotOption(ctx context.Context, menuID, slotID, optionProductID string) error {
	err := s.menuSlotOptionRepo.Delete(ctx, slotID, optionProductID)
	if err == nil {
		s.cache.invalidate()
	}
	return err
}
