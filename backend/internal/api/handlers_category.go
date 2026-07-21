package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"backend/internal/generated/api/generated"
	nanoid "backend/internal/id"
	"backend/internal/response"
	"backend/internal/service"
)

// ListCategories returns all categories.
// (GET /categories)
func (h *Handlers) ListCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.categories.GetAll(r.Context())
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, generated.CategoryList{
		Items: toAPICategories(cats),
	})
}

// CreateCategory creates a new category.
// (POST /categories)
func (h *Handlers) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var body generated.CategoryCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	position := 0
	if body.Position != nil {
		position = *body.Position
	}

	cat, err := h.categories.Create(r.Context(), body.Name, position, body.ParentId)
	if err != nil {
		writeCategoryError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusCreated, toAPICategory(cat))
}

func writeCategoryError(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrInvalidParent) {
		writeError(w, http.StatusBadRequest, "invalid_parent", "The selected parent category is invalid.")
		return
	}
	writeEntError(w, err)
}

// GetCategory returns a single category by ID.
// (GET /categories/{categoryId})
func (h *Handlers) GetCategory(w http.ResponseWriter, r *http.Request, categoryId string) {
	cat, err := h.categories.GetByID(r.Context(), categoryId)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPICategory(cat))
}

// UpdateCategory partially updates a category.
// (PATCH /categories/{categoryId})
func (h *Handlers) UpdateCategory(w http.ResponseWriter, r *http.Request, categoryId string) {
	// Fetch current state so we can merge partial updates.
	existing, err := h.categories.GetByID(r.Context(), categoryId)
	if err != nil {
		writeEntError(w, err)
		return
	}

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	var body generated.CategoryUpdate
	if err := json.Unmarshal(raw, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	name := existing.Name
	if body.Name != nil {
		name = *body.Name
	}
	position := existing.Position
	if body.Position != nil {
		position = *body.Position
	}
	isActive := existing.IsActive
	if body.IsActive != nil {
		isActive = *body.IsActive
	}

	// parentId is tri-state; a *string can't distinguish absent from explicit
	// null, so probe the raw body for the key.
	var probe map[string]json.RawMessage
	_ = json.Unmarshal(raw, &probe)
	_, setParent := probe["parentId"]

	cat, err := h.categories.Update(r.Context(), categoryId, name, position, isActive, body.ParentId, setParent)
	if err != nil {
		writeCategoryError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPICategory(cat))
}

// SetCategoryActive toggles a category's active flag from a POS device.
// (PATCH /pos/categories/{categoryId}/active)
func (h *Handlers) SetCategoryActive(w http.ResponseWriter, r *http.Request, categoryId string) {
	if !nanoid.Valid(categoryId) {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid category id")
		return
	}

	var body struct {
		IsActive *bool `json:"isActive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.IsActive == nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "isActive required")
		return
	}

	existing, err := h.categories.GetByID(r.Context(), categoryId)
	if err != nil {
		writeEntError(w, err)
		return
	}

	cat, err := h.categories.Update(
		r.Context(),
		categoryId,
		existing.Name,
		existing.Position,
		*body.IsActive,
		existing.ParentID,
		false,
	)
	if err != nil {
		writeCategoryError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPICategory(cat))
}

// DeleteCategory removes a category.
// (DELETE /categories/{categoryId})
func (h *Handlers) DeleteCategory(w http.ResponseWriter, r *http.Request, categoryId string) {
	if err := h.categories.Delete(r.Context(), categoryId); err != nil {
		writeEntError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
