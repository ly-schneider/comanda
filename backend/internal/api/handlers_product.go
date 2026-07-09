package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/product"
	"backend/internal/response"
	"backend/internal/service"

	nanoid "backend/internal/id"
	"go.uber.org/zap"
)

// ListProducts returns products, optionally filtered by category.
// (GET /products)
func (h *Handlers) ListProducts(w http.ResponseWriter, r *http.Request, params generated.ListProductsParams) {
	ctx := r.Context()

	var products []*ent.Product
	var err error

	if params.CategoryId != nil {
		products, err = h.products.GetByCategory(ctx, string(*params.CategoryId))
	} else {
		products, err = h.products.GetAll(ctx)
	}
	if err != nil {
		writeEntError(w, err)
		return
	}

	apiProducts := toAPIProducts(products)

	// Enrich with inventory stock levels.
	ids := make([]string, len(products))
	for i, p := range products {
		ids[i] = p.ID
	}
	if stocks, err := h.products.GetStockBatch(ctx, ids); err == nil {
		for i, p := range products {
			if s, ok := stocks[p.ID]; ok {
				apiProducts[i].Stock = &s
			}
		}
	}
	if variantStocks, err := h.products.GetVariantStockBatch(ctx, ids); err == nil {
		for i := range apiProducts {
			setVariantStocks(&apiProducts[i], variantStocks)
		}
	}

	response.WriteJSON(w, http.StatusOK, generated.ProductList{
		Items: apiProducts,
	})
}

// CreateProduct creates a new product.
// (POST /products)
func (h *Handlers) CreateProduct(w http.ResponseWriter, r *http.Request) {
	var body generated.ProductCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	pType := product.TypeSimple
	if body.Type != nil {
		pType = product.Type(*body.Type)
	}

	var jetonID *string
	if body.JetonId != nil {
		id := string(*body.JetonId)
		jetonID = &id
	}

	description := body.Description
	if description != nil && *description == "" {
		description = nil
	}

	var depositCents int64
	if body.DepositCents != nil {
		depositCents = *body.DepositCents
	}

	prod, err := h.products.Create(
		r.Context(),
		body.CategoryId,
		pType,
		body.Name,
		body.PriceCents,
		depositCents,
		true, // isActive defaults to true
		body.Image,
		description,
		jetonID,
		normalizeVariantLabel(body.VariantLabel),
		toVariantInputs(body.Variants),
	)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusCreated, toAPIProduct(prod))
}

// nil leaves variants unchanged; an empty slice removes them all.
func toVariantInputs(variants *[]generated.VariantUpsert) []service.VariantInput {
	if variants == nil {
		return nil
	}
	inputs := make([]service.VariantInput, 0, len(*variants))
	for i, v := range *variants {
		in := service.VariantInput{Value: v.Value, IsActive: true, Sequence: i}
		if v.Id != nil {
			id := *v.Id
			in.ID = &id
		}
		if v.PriceAdjustmentCents != nil {
			in.PriceAdjustmentCents = *v.PriceAdjustmentCents
		}
		if v.IsActive != nil {
			in.IsActive = *v.IsActive
		}
		if v.Sequence != nil {
			in.Sequence = *v.Sequence
		}
		inputs = append(inputs, in)
	}
	return inputs
}

func normalizeVariantLabel(label *string) *string {
	if label == nil || *label == "" {
		return nil
	}
	return label
}

func setVariantStocks(p *generated.Product, stocks map[string]int) {
	if p.Variants == nil {
		return
	}
	variants := *p.Variants
	for i := range variants {
		s := stocks[variants[i].Id]
		variants[i].Stock = &s
	}
}

// GetProduct returns a single product by ID.
// (GET /products/{productId})
func (h *Handlers) GetProduct(w http.ResponseWriter, r *http.Request, productId string) {
	ctx := r.Context()
	id := productId
	prod, err := h.products.GetByID(ctx, id)
	if err != nil {
		writeEntError(w, err)
		return
	}
	ap := toAPIProduct(prod)
	if stock, err := h.products.GetStock(ctx, id); err == nil {
		s := int(stock)
		ap.Stock = &s
	}
	if variantStocks, err := h.products.GetVariantStockBatch(ctx, []string{id}); err == nil {
		setVariantStocks(&ap, variantStocks)
	}
	response.WriteJSON(w, http.StatusOK, ap)
}

// UpdateProduct partially updates a product.
// (PATCH /products/{productId})
func (h *Handlers) UpdateProduct(w http.ResponseWriter, r *http.Request, productId string) {
	ctx := r.Context()
	id := productId

	existing, err := h.products.GetByID(ctx, id)
	if err != nil {
		writeEntError(w, err)
		return
	}

	var body generated.ProductUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	name := existing.Name
	if body.Name != nil {
		name = *body.Name
	}
	priceCents := existing.PriceCents
	if body.PriceCents != nil {
		priceCents = *body.PriceCents
	}
	depositCents := existing.DepositCents
	if body.DepositCents != nil {
		depositCents = *body.DepositCents
	}
	isActive := existing.IsActive
	if body.IsActive != nil {
		isActive = *body.IsActive
	}
	categoryID := existing.CategoryID
	if body.CategoryId != nil {
		categoryID = string(*body.CategoryId)
	}
	image := existing.Image
	if body.Image != nil {
		image = body.Image
	}
	description := existing.Description
	if body.Description != nil {
		if *body.Description == "" {
			description = nil
		} else {
			description = body.Description
		}
	}
	jetonID := existing.JetonID
	if body.JetonId != nil {
		id := string(*body.JetonId)
		jetonID = &id
	}
	variantLabel := existing.VariantLabel
	if body.VariantLabel != nil {
		variantLabel = normalizeVariantLabel(body.VariantLabel)
	}

	prod, err := h.products.Update(
		ctx,
		id,
		categoryID,
		existing.Type,
		name,
		priceCents,
		depositCents,
		isActive,
		image,
		description,
		jetonID,
		variantLabel,
		toVariantInputs(body.Variants),
	)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPIProduct(prod))
}

// DeleteProduct removes a product.
// (DELETE /products/{productId})
func (h *Handlers) DeleteProduct(w http.ResponseWriter, r *http.Request, productId string) {
	if err := h.products.Delete(r.Context(), productId); err != nil {
		writeEntError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetProductInventory returns the current inventory for a product.
// (GET /products/{productId}/inventory)
func (h *Handlers) GetProductInventory(w http.ResponseWriter, r *http.Request, productId string) {
	ctx := r.Context()
	id := productId

	prod, err := h.products.GetByIDWithRelations(ctx, id)
	if err != nil {
		writeEntError(w, err)
		return
	}

	inv, err := h.buildInventory(ctx, prod)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, inv)
}

func (h *Handlers) buildInventory(ctx context.Context, prod *ent.Product) (generated.Inventory, error) {
	stock, err := h.products.GetStock(ctx, prod.ID)
	if err != nil {
		return generated.Inventory{}, err
	}
	inv := generated.Inventory{
		ProductId: prod.ID,
		Quantity:  int(stock),
	}
	if len(prod.Edges.Variants) > 0 {
		variantStocks, err := h.products.GetVariantStockBatch(ctx, []string{prod.ID})
		if err != nil {
			return generated.Inventory{}, err
		}
		items := make([]generated.VariantInventory, 0, len(prod.Edges.Variants))
		for _, v := range prod.Edges.Variants {
			items = append(items, generated.VariantInventory{
				VariantId: v.ID,
				Quantity:  variantStocks[v.ID],
			})
		}
		inv.Variants = &items
	}
	return inv, nil
}

// GetProductInventoryHistory returns paginated inventory ledger entries for a product.
// (GET /products/{productId}/inventory/history)
func (h *Handlers) GetProductInventoryHistory(w http.ResponseWriter, r *http.Request, productId string, params generated.GetProductInventoryHistoryParams) {
	ctx := r.Context()
	id := productId

	// Verify product exists.
	_, err := h.products.GetByID(ctx, id)
	if err != nil {
		writeEntError(w, err)
		return
	}

	limit := 50
	offset := 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}

	entries, err := h.products.ListInventoryHistory(ctx, id, limit, offset)
	if err != nil {
		writeEntError(w, err)
		return
	}

	items := make([]generated.InventoryLedgerEntry, 0, len(entries))
	for _, e := range entries {
		items = append(items, toAPIInventoryLedgerEntry(e))
	}
	response.WriteJSON(w, http.StatusOK, generated.InventoryLedgerList{Items: items})
}

// AdjustProductInventory adjusts the inventory for a product.
// (PATCH /products/{productId}/inventory)
func (h *Handlers) AdjustProductInventory(w http.ResponseWriter, r *http.Request, productId string) {
	var body generated.InventoryAdjustment
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	ctx := r.Context()
	id := productId

	prod, err := h.products.GetByIDWithRelations(ctx, id)
	if err != nil {
		writeEntError(w, err)
		return
	}

	if err := h.products.AdjustStock(ctx, id, body.VariantId, int64(body.Delta), string(body.Reason)); err != nil {
		writeEntError(w, err)
		return
	}

	inv, err := h.buildInventory(ctx, prod)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, inv)
}

// SetProductActive toggles isActive for a product or bundle from a POS device.
// PATCH /v1/pos/products/{productId}/active
func (h *Handlers) SetProductActive(w http.ResponseWriter, r *http.Request, productId string) {
	ctx := r.Context()

	id := productId
	if !nanoid.Valid(id) {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid product id")
		return
	}

	var body struct {
		IsActive *bool `json:"isActive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.IsActive == nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "isActive required")
		return
	}

	existing, err := h.products.GetByID(ctx, id)
	if err != nil {
		writeEntError(w, err)
		return
	}

	prod, err := h.products.Update(
		ctx,
		id,
		existing.CategoryID,
		existing.Type,
		existing.Name,
		existing.PriceCents,
		existing.DepositCents,
		*body.IsActive,
		existing.Image,
		existing.Description,
		existing.JetonID,
		existing.VariantLabel,
		nil,
	)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPIProduct(prod))
}

var allowedImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

func (h *Handlers) UploadProductImage(w http.ResponseWriter, r *http.Request, productId string) {
	if h.blobStore == nil {
		writeError(w, http.StatusNotImplemented, "not_configured", "Image uploads not configured")
		return
	}

	const maxSize = 5 << 20 // 5 MB
	if err := r.ParseMultipartForm(maxSize); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "File too large or invalid multipart form")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Missing file field")
		return
	}
	defer func() { _ = file.Close() }()

	buf := make([]byte, 512)
	n, err := io.ReadAtLeast(file, buf, 1)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Could not read file")
		return
	}
	contentType := http.DetectContentType(buf[:n])
	if !allowedImageTypes[contentType] {
		writeError(w, http.StatusBadRequest, "invalid_file_type", "Only JPEG, PNG, and WebP images are allowed")
		return
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
		return
	}

	ctx := r.Context()
	id := productId
	existing, err := h.products.GetByID(ctx, id)
	if err != nil {
		writeEntError(w, err)
		return
	}

	if existing.Image != nil && *existing.Image != "" {
		_ = h.blobStore.Delete(ctx, *existing.Image)
	}

	var allData bytes.Buffer
	if _, err := io.Copy(&allData, file); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
		return
	}

	reader := bytes.NewReader(allData.Bytes())
	key := "products/" + nanoid.New()
	url, err := h.blobStore.Upload(ctx, key, reader, contentType)
	if err != nil {
		h.logger.Error("failed to upload image", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "upload_failed", "Failed to upload image")
		return
	}

	_, err = h.products.Update(ctx, id, existing.CategoryID, existing.Type, existing.Name, existing.PriceCents, existing.DepositCents, existing.IsActive, &url, existing.Description, existing.JetonID, existing.VariantLabel, nil)
	if err != nil {
		writeEntError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, generated.ProductImageResponse{ImageUrl: url})
}

func (h *Handlers) DeleteProductImage(w http.ResponseWriter, r *http.Request, productId string) {
	ctx := r.Context()
	id := productId

	existing, err := h.products.GetByID(ctx, id)
	if err != nil {
		writeEntError(w, err)
		return
	}

	if existing.Image != nil && *existing.Image != "" {
		if h.blobStore != nil {
			_ = h.blobStore.Delete(ctx, *existing.Image)
		}
	}

	_, err = h.products.Update(ctx, id, existing.CategoryID, existing.Type, existing.Name, existing.PriceCents, existing.DepositCents, existing.IsActive, nil, existing.Description, existing.JetonID, existing.VariantLabel, nil)
	if err != nil {
		writeEntError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
