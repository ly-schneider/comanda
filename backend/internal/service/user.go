package service

import (
	"context"
	"errors"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/user"
	"backend/internal/repository"

	"entgo.io/ent/dialect/sql"
)

type UserService interface {
	List(ctx context.Context) ([]*ent.User, int64, error)
	GetByID(ctx context.Context, id string) (*ent.User, error)
	UpdateName(ctx context.Context, id string, name string) error
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

func (s *userService) UpdateName(ctx context.Context, id string, name string) error {
	if err := s.requireOrgMember(ctx, id); err != nil {
		return err
	}
	return s.userRepo.UpdateName(ctx, id, name)
}

func (s *userService) Delete(ctx context.Context, id string) error {
	if err := s.requireOrgMember(ctx, id); err != nil {
		return err
	}
	return s.userRepo.Delete(ctx, id)
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
	defer rows.Close()

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

func entDescOrder() sql.OrderTermOption {
	return sql.OrderDesc()
}
