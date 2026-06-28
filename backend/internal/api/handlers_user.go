package api

import (
	"encoding/json"
	"net/http"

	"backend/internal/generated/api/generated"
	"backend/internal/response"
)

// ListUsers returns the users.
// (GET /users)
func (h *Handlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	users, _, err := h.users.List(ctx)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, generated.UserList{
		Items: toAPIUsers(users),
	})
}

// GetUser returns a single user by their Better Auth user ID (string).
// (GET /users/{userId})
func (h *Handlers) GetUser(w http.ResponseWriter, r *http.Request, userId string) {
	user, err := h.users.GetByID(r.Context(), userId)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPIUser(user))
}

// UpdateUser partially updates a user.
// (PATCH /users/{userId})
func (h *Handlers) UpdateUser(w http.ResponseWriter, r *http.Request, userId string) {
	var body generated.UserUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	ctx := r.Context()

	if body.Name != nil {
		if err := h.users.UpdateName(ctx, userId, *body.Name); err != nil {
			writeEntError(w, err)
			return
		}
	}

	user, err := h.users.GetByID(ctx, userId)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPIUser(user))
}

// DeleteUser removes the user from the current tenant (its member row).
// (DELETE /users/{userId})
func (h *Handlers) DeleteUser(w http.ResponseWriter, r *http.Request, userId string) {
	if err := h.users.Delete(r.Context(), userId); err != nil {
		writeEntError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
