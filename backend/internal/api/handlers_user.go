package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/response"
	"backend/internal/service"
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

	items := toAPIUsers(users)
	ids := make([]string, len(items))
	for i := range items {
		ids[i] = items[i].Id
	}
	roles, err := h.users.RolesByUserIDs(ctx, ids)
	if err != nil {
		writeEntError(w, err)
		return
	}
	for i := range items {
		if role, ok := roles[items[i].Id]; ok {
			items[i].Role = ptr(generated.TenantRole(role))
		}
	}

	response.WriteJSON(w, http.StatusOK, generated.UserList{Items: items})
}

// GetUser returns a single user by their Better Auth user ID (string).
// (GET /users/{userId})
func (h *Handlers) GetUser(w http.ResponseWriter, r *http.Request, userId string) {
	ctx := r.Context()

	user, err := h.users.GetByID(ctx, userId)
	if err != nil {
		writeEntError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, h.userWithRole(ctx, user))
}

// UpdateUser partially updates a member's name and/or tenant role.
// (PATCH /users/{userId})
func (h *Handlers) UpdateUser(w http.ResponseWriter, r *http.Request, userId string) {
	var body generated.UserUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	ctx := r.Context()

	if body.Role != nil {
		if err := h.users.UpdateRole(ctx, userId, string(*body.Role)); err != nil {
			writeRoleError(w, err)
			return
		}
	}

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
	response.WriteJSON(w, http.StatusOK, h.userWithRole(ctx, user))
}

// DeleteUser removes the user from the current tenant (its member row).
// (DELETE /users/{userId})
func (h *Handlers) DeleteUser(w http.ResponseWriter, r *http.Request, userId string) {
	if err := h.users.Delete(r.Context(), userId); err != nil {
		writeRoleError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) userWithRole(ctx context.Context, e *ent.User) generated.User {
	u := toAPIUser(e)
	roles, err := h.users.RolesByUserIDs(ctx, []string{e.ID})
	if err == nil {
		if role, ok := roles[e.ID]; ok {
			u.Role = ptr(generated.TenantRole(role))
		}
	}
	return u
}

// writeRoleError maps the member-management guard errors to their HTTP status
// and a stable machine code, falling back to the generic ent-error mapping
// (which turns a non-member target into a 404).
func writeRoleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidRole):
		writeError(w, http.StatusBadRequest, "invalid_role", "The role is not a valid tenant role.")
	case errors.Is(err, service.ErrForbiddenSelf):
		writeError(w, http.StatusForbidden, "forbidden_self", "You cannot change or remove your own membership.")
	case errors.Is(err, service.ErrForbiddenRole):
		writeError(w, http.StatusForbidden, "forbidden_role_change", "You cannot manage a member at or above your own role.")
	case errors.Is(err, service.ErrForbiddenHigher):
		writeError(w, http.StatusForbidden, "forbidden_higher_role", "You cannot grant a role at or above your own.")
	case errors.Is(err, service.ErrNoActorRole):
		writeError(w, http.StatusForbidden, "forbidden_role_change", "You are not permitted to manage members.")
	default:
		writeEntError(w, err)
	}
}
