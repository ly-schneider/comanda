package api

import (
	"encoding/json"
	"net/http"

	"backend/internal/auth"
	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent/admininvite"
	"backend/internal/response"
)

// ListInvites returns admin invites with optional filtering.
// (GET /invites)
func (h *Handlers) ListInvites(w http.ResponseWriter, r *http.Request, params generated.ListInvitesParams) {
	var statusFilter *string
	if params.Status != nil {
		s := string(*params.Status)
		statusFilter = &s
	}

	invites, _, err := h.invites.List(r.Context(), statusFilter, params.Email)
	if err != nil {
		writeEntError(w, err)
		return
	}

	// After Task 6, invites will be []*ent.AdminInvite and toAPIInvites maps them.
	response.WriteJSON(w, http.StatusOK, generated.InviteList{
		Items: toAPIInvites(invites),
	})
}

// CreateInvite creates a new admin invite and sends the invitation email.
// (POST /invites)
func (h *Handlers) CreateInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := auth.GetUserID(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var body generated.InviteCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Tenant /admin grants app-level roles only; admin is minted from the hub.
	memberRole := admininvite.MemberRoleStaff
	if body.MemberRole != nil {
		switch *body.MemberRole {
		case generated.MemberRoleAppAdmin:
			memberRole = admininvite.MemberRoleAppAdmin
		case generated.MemberRoleStaff:
			memberRole = admininvite.MemberRoleStaff
		case generated.MemberRoleCatalog:
			memberRole = admininvite.MemberRoleCatalog
		default:
			writeError(w, http.StatusBadRequest, "invalid_member_role", "Invalid member role")
			return
		}
	}

	invite, err := h.invites.Create(ctx, userID, string(body.Email), memberRole, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invite_failed", err.Error())
		return
	}
	response.WriteJSON(w, http.StatusCreated, toAPIInvite(invite))
}

// CreateAccountInvite creates a settings-admin invite from the account hub. The
// tenant is bound by ResolveVendorTenant, so the path param is unused here.
// (POST /account/{accountTenant}/invites)
func (h *Handlers) CreateAccountInvite(w http.ResponseWriter, r *http.Request, _ generated.AccountTenantParam) {
	ctx := r.Context()

	userID, ok := auth.GetUserID(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var body generated.InviteCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if body.MemberRole != nil && *body.MemberRole != generated.MemberRoleAdmin {
		writeError(w, http.StatusBadRequest, "invalid_member_role", "The account hub can only invite admins")
		return
	}

	invite, err := h.invites.Create(ctx, userID, string(body.Email), admininvite.MemberRoleAdmin, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invite_failed", err.Error())
		return
	}
	response.WriteJSON(w, http.StatusCreated, toAPIInvite(invite))
}

// GetInvite returns a public view of an invite by ID.
// (GET /invites/{inviteId})
func (h *Handlers) GetInvite(w http.ResponseWriter, r *http.Request, inviteId string) {
	invite, err := h.invites.GetByID(r.Context(), inviteId)
	if err != nil {
		writeEntError(w, err)
		return
	}

	// Return the public view (limited fields).
	apiInvite := toAPIInvite(invite)
	public := generated.InvitePublic{
		Id:           apiInvite.Id,
		InviteeEmail: apiInvite.InviteeEmail,
		Status:       apiInvite.Status,
		MemberRole:   apiInvite.MemberRole,
		ExpiresAt:    apiInvite.ExpiresAt,
	}
	response.WriteJSON(w, http.StatusOK, public)
}

// DeleteInvite revokes/deletes an invite.
// (DELETE /invites/{inviteId})
func (h *Handlers) DeleteInvite(w http.ResponseWriter, r *http.Request, inviteId string) {
	if err := h.invites.Delete(r.Context(), inviteId); err != nil {
		writeEntError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// VerifyInvite verifies an invite token without accepting it.
// (POST /invites/verify)
func (h *Handlers) VerifyInvite(w http.ResponseWriter, r *http.Request) {
	var body generated.InviteTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	invite, err := h.invites.Verify(r.Context(), body.Token)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_token", err.Error())
		return
	}

	// After Task 6, Verify returns *ent.AdminInvite.
	apiInvite := toAPIInvite(invite)
	public := generated.InvitePublic{
		Id:           apiInvite.Id,
		InviteeEmail: apiInvite.InviteeEmail,
		Status:       apiInvite.Status,
		MemberRole:   apiInvite.MemberRole,
		ExpiresAt:    apiInvite.ExpiresAt,
	}
	response.WriteJSON(w, http.StatusOK, public)
}

// AcceptInvite accepts an invite using the token.
// (POST /invites/accept)
func (h *Handlers) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	var body generated.InviteAccept
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	lastName := ""
	if body.LastName != nil {
		lastName = *body.LastName
	}

	if err := h.invites.Accept(r.Context(), body.Token, body.FirstName, lastName); err != nil {
		writeError(w, http.StatusBadRequest, "accept_failed", err.Error())
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]string{
		"status": "accepted",
	})
}
