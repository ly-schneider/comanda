package repository

import (
	"context"
	"database/sql"

	"backend/internal/database"
	nanoid "backend/internal/id"
)

// MemberRepository manages Better Auth org membership rows on the privileged
// pool. member is owned by Better Auth (not Ent), so it is hand-written SQL; the
// privileged pool bypasses the org-keyed RLS by table ownership, which the
// owner-invite flow needs because the accepting user is not yet a member.
type MemberRepository interface {
	CountOwnersForOrg(ctx context.Context, orgID string) (int, error)
	CreateMember(ctx context.Context, orgID, userID, role string) error
}

type memberRepo struct {
	db *sql.DB
}

func NewMemberRepository(privileged *database.PrivilegedClient) MemberRepository {
	return &memberRepo{db: privileged.DB}
}

func (r *memberRepo) CountOwnersForOrg(ctx context.Context, orgID string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT count(*) FROM "member" WHERE organization_id = $1 AND role = 'owner'`,
		orgID,
	).Scan(&n)
	return n, err
}

func (r *memberRepo) CreateMember(ctx context.Context, orgID, userID, role string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO "member" (id, organization_id, user_id, role, created_at)
		 VALUES ($1, $2, $3, $4, now())
		 ON CONFLICT (organization_id, user_id) DO NOTHING`,
		nanoid.New(), orgID, userID, role,
	)
	return err
}
