package api

import (
	"encoding/json"
	"net/http"

	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/product"
	"backend/internal/response"

	nanoid "backend/internal/id"
)

// ListBundles returns all bundle-type products with their slots and options.
// (GET /bundles)
func (h *Handlers) ListBundles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	bundles, err := h.products.GetBundles(ctx)
	if err != nil {
		writeEntError(w, err)
		return
	}

	items := make([]generated.Bundle, 0, len(bundles))
	for _, m := range bundles {
		items = append(items, entProductToAPIBundle(m))
	}
	response.WriteJSON(w, http.StatusOK, generated.BundleList{Items: items})
}

// CreateBundle creates a new bundle (product with type=bundle).
// (POST /bundles)
func (h *Handlers) CreateBundle(w http.ResponseWriter, r *http.Request) {
	var body generated.BundleCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	var priceCents int64
	if body.PriceCents != nil {
		priceCents = *body.PriceCents
	}

	description := body.Description
	if description != nil && *description == "" {
		description = nil
	}

	prod, err := h.products.Create(
		r.Context(),
		body.CategoryId,
		product.TypeBundle,
		body.Name,
		priceCents,
		0,
		true,
		body.Image,
		description,
		nil,
		nil,
		nil,
	)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusCreated, entProductToAPIBundle(prod))
}

// GetBundle returns a single bundle by ID with slots and options loaded.
// (GET /bundles/{bundleId})
func (h *Handlers) GetBundle(w http.ResponseWriter, r *http.Request, bundleId string) {
	prod, err := h.products.GetByID(r.Context(), bundleId)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, entProductToAPIBundle(prod))
}

// UpdateBundle partially updates a bundle product.
// (PATCH /bundles/{bundleId})
func (h *Handlers) UpdateBundle(w http.ResponseWriter, r *http.Request, bundleId string) {
	ctx := r.Context()
	id := bundleId

	existing, err := h.products.GetByID(ctx, id)
	if err != nil {
		writeEntError(w, err)
		return
	}

	var body generated.BundleUpdate
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
	isActive := existing.IsActive
	if body.IsActive != nil {
		isActive = *body.IsActive
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

	prod, err := h.products.Update(
		ctx,
		id,
		existing.CategoryID,
		product.TypeBundle,
		name,
		priceCents,
		existing.DepositCents,
		isActive,
		image,
		description,
		nil,
		nil,
		nil,
	)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, entProductToAPIBundle(prod))
}

// DeleteBundle removes a bundle product.
// (DELETE /bundles/{bundleId})
func (h *Handlers) DeleteBundle(w http.ResponseWriter, r *http.Request, bundleId string) {
	if err := h.products.Delete(r.Context(), bundleId); err != nil {
		writeEntError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CreateBundleSlot creates a new slot on a bundle.
// (POST /bundles/{bundleId}/slots)
func (h *Handlers) CreateBundleSlot(w http.ResponseWriter, r *http.Request, bundleId string) {
	var body generated.BundleSlotCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	slot, err := h.products.CreateBundleSlot(r.Context(), bundleId, body.Name)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusCreated, toAPIBundleSlot(slot))
}

// UpdateBundleSlot renames a bundle slot.
// (PATCH /bundles/{bundleId}/slots/{slotId})
func (h *Handlers) UpdateBundleSlot(w http.ResponseWriter, r *http.Request, bundleId string, slotId string) {
	var body generated.BundleSlotUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	name := ""
	if body.Name != nil {
		name = *body.Name
	}

	slot, err := h.products.UpdateBundleSlot(r.Context(), bundleId, slotId, name)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPIBundleSlot(slot))
}

// DeleteBundleSlot removes a slot from a bundle.
// (DELETE /bundles/{bundleId}/slots/{slotId})
func (h *Handlers) DeleteBundleSlot(w http.ResponseWriter, r *http.Request, bundleId string, slotId string) {
	if err := h.products.DeleteBundleSlot(r.Context(), bundleId, slotId); err != nil {
		writeEntError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ReorderBundleSlots updates the sequence order of bundle slots.
// (PATCH /bundles/{bundleId}/slots/reorder)
func (h *Handlers) ReorderBundleSlots(w http.ResponseWriter, r *http.Request, bundleId string) {
	var body generated.BundleSlotReorder
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	positions := make(map[string]int, len(body.Positions))
	for idStr, seq := range body.Positions {
		if !nanoid.Valid(idStr) {
			writeError(w, http.StatusBadRequest, "invalid_slot_id", "Invalid slot id: "+idStr)
			return
		}
		positions[idStr] = seq
	}

	if err := h.products.ReorderBundleSlots(r.Context(), bundleId, positions); err != nil {
		writeEntError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AddSlotOption adds a product as an option to a bundle slot.
// (POST /bundles/{bundleId}/slots/{slotId}/options)
func (h *Handlers) AddSlotOption(w http.ResponseWriter, r *http.Request, bundleId string, slotId string) {
	var body generated.BundleSlotOptionCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	opt, err := h.products.AddSlotOption(r.Context(), bundleId, slotId, body.ProductId)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusCreated, toAPIBundleSlotOption(opt))
}

// RemoveSlotOption removes a product option from a bundle slot.
// (DELETE /bundles/{bundleId}/slots/{slotId}/options/{optionProductId})
func (h *Handlers) RemoveSlotOption(w http.ResponseWriter, r *http.Request, bundleId string, slotId string, optionProductId string) {
	if err := h.products.RemoveSlotOption(r.Context(), bundleId, slotId, optionProductId); err != nil {
		writeEntError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// entProductToAPIBundle converts an ent.Product (type=bundle) into a generated.Bundle.
func entProductToAPIBundle(e *ent.Product) generated.Bundle {
	m := generated.Bundle{
		Id:          e.ID,
		CategoryId:  e.CategoryID,
		Name:        e.Name,
		Image:       e.Image,
		Description: e.Description,
		PriceCents:  e.PriceCents,
		IsActive:    e.IsActive,
		CreatedAt:   ptr(e.CreatedAt),
		UpdatedAt:   ptr(e.UpdatedAt),
	}

	if slots, err := e.Edges.BundleSlotsOrErr(); err == nil && len(slots) > 0 {
		apiSlots := make([]generated.BundleSlot, 0, len(slots))
		for _, s := range slots {
			apiSlots = append(apiSlots, toAPIBundleSlot(s))
		}
		m.Slots = &apiSlots
	}

	return m
}
