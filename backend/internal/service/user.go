package service

import (
	"context"
	"database/sql"
	"errors"

	"backend/internal/auth"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/user"
	"backend/internal/repository"

	entsql "entgo.io/ent/dialect/sql"
)

var (
	ErrInvalidRole     = errors.New("invalid_role")
	ErrForbiddenSelf   = errors.New("forbidden_self")
	ErrForbiddenRole   = errors.New("forbidden_role_change")
	ErrForbiddenHigher = errors.New("forbidden_higher_role")
	ErrNoActorRole     = errors.New("actor has no role in the bound organization")
)

// roleRank orders the five tenant roles. A member may only manage a target
// whose rank is strictly below their own, and may only grant a role strictly
// below their own. This is intentionally separate from the 4-tier TenantTier,
// which collapses owner and admin into one tier.
var roleRank = map[string]int{
	"owner":     5,
	"admin":     4,
	"app_admin": 3,
	"staff":     2,
	"catalog":   1,
}

type UserService interface {
	List(ctx context.Context) ([]*ent.User, int64, error)
	GetByID(ctx context.Context, id string) (*ent.User, error)
	RolesByUserIDs(ctx context.Context, ids []string) (map[string]string, error)
	UpdateName(ctx context.Context, id string, name string) error
	UpdateRole(ctx context.Context, id string, newRole string) error
	Delete(ctx context.Context, id string) error
}

type userService struct {
	client   *ent.Client
	userRepo repository.UserRepository
}

func NewUserService(
	client *ent.Client,
	userRepo repository.UserRepository,
) UserService {
	return &userService{
		client:   client,
		userRepo: userRepo,
	}
}

func (s *userService) List(ctx context.Context) ([]*ent.User, int64, error) {
	ids, err := s.orgMemberUserIDs(ctx)
	if err != nil {
		return nil, 0, err
	}
	if len(ids) == 0 {
		return []*ent.User{}, 0, nil
	}

	q := repository.ClientFromContext(ctx, s.client).User.Query().Where(user.IDIn(ids...))

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	users, err := q.Order(user.ByCreatedAt(entDescOrder())).All(ctx)
	if err != nil {
		return nil, 0, err
	}

	return users, int64(total), nil
}

func (s *userService) GetByID(ctx context.Context, id string) (*ent.User, error) {
	if err := s.requireOrgMember(ctx, id); err != nil {
		return nil, err
	}
	return s.userRepo.GetByID(ctx, id)
}

// RolesByUserIDs returns the bound org's member role for each of the given user
// IDs. IDs without a membership in the bound org are simply absent from the map.
func (s *userService) RolesByUserIDs(ctx context.Context, ids []string) (map[string]string, error) {
	roles := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return roles, nil
	}
	tx, ok := repository.RequestTxFromContext(ctx)
	if !ok {
		return nil, errors.New("user service: no request tx in context")
	}
	rows, err := tx.QueryContext(ctx,
		"SELECT user_id, role FROM member WHERE organization_id = app_current_org_id() AND user_id = ANY($1)",
		ids)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id, role string
		if err := rows.Scan(&id, &role); err != nil {
			return nil, err
		}
		roles[id] = role
	}
	return roles, rows.Err()
}

func (s *userService) UpdateName(ctx context.Context, id string, name string) error {
	if err := s.requireOrgMember(ctx, id); err != nil {
		return err
	}
	return s.userRepo.UpdateName(ctx, id, name)
}

// UpdateRole changes a member's role in the bound org after enforcing the
// privilege-escalation guards. A caller can neither change their own role nor
// manage a member whose current role is at or above their own, and can only
// grant a role strictly below their own.
func (s *userService) UpdateRole(ctx context.Context, id string, newRole string) error {
	if _, ok := roleRank[newRole]; !ok {
		return ErrInvalidRole
	}
	actorRank, err := s.guardManage(ctx, id)
	if err != nil {
		return err
	}
	if roleRank[newRole] >= actorRank {
		return ErrForbiddenHigher
	}

	tx, ok := repository.RequestTxFromContext(ctx)
	if !ok {
		return errors.New("user service: no request tx in context")
	}
	_, err = tx.ExecContext(ctx,
		"UPDATE member SET role = $2 WHERE organization_id = app_current_org_id() AND user_id = $1",
		id, newRole)
	return err
}

// Delete removes the user from the bound tenant only — it deletes the member row,
// not the global user account, so a user who belongs to other tenants keeps those
// memberships. The management guard 404s a non-member and 403s an attempt to
// remove yourself or a peer/higher role.
func (s *userService) Delete(ctx context.Context, id string) error {
	if _, err := s.guardManage(ctx, id); err != nil {
		return err
	}
	tx, ok := repository.RequestTxFromContext(ctx)
	if !ok {
		return errors.New("user service: no request tx in context")
	}
	_, err := tx.ExecContext(ctx,
		"DELETE FROM member WHERE organization_id = app_current_org_id() AND user_id = $1", id)
	return err
}

// guardManage enforces the shared privilege-escalation guards for role changes
// and removals, and returns the actor's role rank so callers can compare a
// requested role against it. It fails closed: a missing actor role, self-target,
// or peer/higher target is refused.
func (s *userService) guardManage(ctx context.Context, targetID string) (int, error) {
	actorID, ok := auth.GetUserID(ctx)
	if !ok || actorID == "" {
		return 0, ErrNoActorRole
	}

	targetRole, ok, err := s.memberRole(ctx, targetID)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, repository.ErrNotFound
	}

	if targetID == actorID {
		return 0, ErrForbiddenSelf
	}

	actorRole, ok, err := s.memberRole(ctx, actorID)
	if err != nil {
		return 0, err
	}
	actorRank, known := roleRank[actorRole]
	if !ok || !known {
		return 0, ErrNoActorRole
	}

	if roleRank[targetRole] >= actorRank {
		return 0, ErrForbiddenRole
	}
	return actorRank, nil
}

func (s *userService) memberRole(ctx context.Context, userID string) (string, bool, error) {
	tx, ok := repository.RequestTxFromContext(ctx)
	if !ok {
		return "", false, errors.New("user service: no request tx in context")
	}
	var role string
	err := tx.QueryRowContext(ctx,
		"SELECT role FROM member WHERE organization_id = app_current_org_id() AND user_id = $1",
		userID,
	).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return role, true, nil
}

// orgMemberUserIDs returns the user IDs that belong to the bound tenant's
// organization, read on the per-request tx so the org-keyed RLS pins the lookup
// to the host-resolved tenant.
func (s *userService) orgMemberUserIDs(ctx context.Context) ([]string, error) {
	tx, ok := repository.RequestTxFromContext(ctx)
	if !ok {
		return nil, errors.New("user service: no request tx in context")
	}
	rows, err := tx.QueryContext(ctx,
		"SELECT user_id FROM member WHERE organization_id = app_current_org_id()")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// requireOrgMember fails closed with ErrNotFound (a 404, not a 403, so callers
// can't probe which user IDs exist outside their org) unless the target user is
// a member of the bound tenant's organization.
func (s *userService) requireOrgMember(ctx context.Context, userID string) error {
	tx, ok := repository.RequestTxFromContext(ctx)
	if !ok {
		return errors.New("user service: no request tx in context")
	}
	var exists bool
	if err := tx.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM member WHERE organization_id = app_current_org_id() AND user_id = $1)",
		userID,
	).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return repository.ErrNotFound
	}
	return nil
}

func entDescOrder() entsql.OrderTermOption {
	return entsql.OrderDesc()
}
