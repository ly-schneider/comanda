package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"backend/internal/auth"
	"backend/internal/generated/api/generated"
	nanoid "backend/internal/id"
	"backend/internal/repository"
	"backend/internal/response"
	"backend/internal/service"
)

// CreateOwnerInvite invites a vendor owner for a pre-created tenant.
func (h *Handlers) CreateOwnerInvite(w http.ResponseWriter, r *http.Request, tenantID string) {
	if !h.requirePlatformContext(w, r) {
		return
	}

	ctx := r.Context()
	inviterID, ok := auth.GetUserID(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	if !nanoid.Valid(tenantID) {
		writeError(w, http.StatusBadRequest, "invalid_tenant_id", "Invalid tenant ID")
		return
	}

	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	invite, err := h.ownerInvites.Create(ctx, tenantID, inviterID, body.Email)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidToken):
			writeError(w, http.StatusBadRequest, "invalid_email", "A valid email is required")
		case errors.Is(err, service.ErrOwnerExists):
			writeError(w, http.StatusConflict, "owner_exists", "This organization already has an owner")
		case errors.Is(err, service.ErrTenantNotLinked):
			writeError(w, http.StatusConflict, "tenant_not_linked", "This tenant is not linked to an organization")
		case errors.Is(err, repository.ErrNotFound):
			writeError(w, http.StatusNotFound, "tenant_not_found", "Tenant not found")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
		}
		return
	}

	response.WriteJSON(w, http.StatusCreated, generated.OwnerInvite{
		Email:     invite.Email,
		Status:    invite.Status,
		ExpiresAt: invite.ExpiresAt,
	})
}

// VerifyOwnerInvite returns the public view of an owner invite by token.
func (h *Handlers) VerifyOwnerInvite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	invite, err := h.ownerInvites.Verify(r.Context(), body.Token)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_token", "Invalid or expired invite")
		return
	}

	response.WriteJSON(w, http.StatusOK, generated.OwnerInvite{
		Email:     invite.Email,
		Status:    invite.Status,
		ExpiresAt: invite.ExpiresAt,
	})
}

// AcceptOwnerInvite makes the logged-in user the owner of the invite's
// organization and binds it as their active organization.
func (h *Handlers) AcceptOwnerInvite(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlatformContext(w, r) {
		return
	}

	ctx := r.Context()

	userID, ok := auth.GetUserID(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}
	email, _ := auth.GetUserEmail(ctx)
	sessionID, ok := auth.GetSessionID(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	orgID, err := h.ownerInvites.Accept(ctx, body.Token, userID, email, sessionID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrEmailMismatch):
			writeError(w, http.StatusForbidden, "email_mismatch", "This invite is for a different email")
		case errors.Is(err, service.ErrInviteExpired):
			writeError(w, http.StatusGone, "invite_expired", "This invite has expired")
		case errors.Is(err, service.ErrInviteAlreadyUsed):
			writeError(w, http.StatusConflict, "invite_used", "This invite has already been used")
		case errors.Is(err, service.ErrInviteNotPending):
			writeError(w, http.StatusConflict, "invite_not_pending", "This invite is no longer valid")
		case errors.Is(err, service.ErrOwnerExists):
			writeError(w, http.StatusConflict, "owner_exists", "This organization already has an owner")
		case errors.Is(err, service.ErrInvalidToken):
			writeError(w, http.StatusBadRequest, "invalid_token", "Invalid invite token")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
		}
		return
	}

	response.WriteJSON(w, http.StatusOK, generated.OwnerInviteAcceptResult{OrganizationId: orgID})
}
