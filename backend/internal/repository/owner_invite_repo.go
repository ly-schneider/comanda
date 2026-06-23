package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"backend/internal/database"
	nanoid "backend/internal/id"
)

// OwnerInvitation is the backend's view of an owner invite stored in the Better
// Auth invitation table.
type OwnerInvitation struct {
	ID             string
	OrganizationID string
	Email          string
	Status         string
	ExpiresAt      time.Time
}

// OwnerInviteRepository manages owner invites on the privileged pool. invitation
// is owned by Better Auth (not Ent); owner invites carry a hashed token and an
// accepted_at the plugin never writes. The privileged pool is required because
// Create runs in a platform-admin context and Accept runs as a not-yet-member,
// not-admin user — both of which the org-keyed RLS would otherwise deny.
type OwnerInviteRepository interface {
	Create(ctx context.Context, orgID, email, inviterID, tokenHash string, expiresAt time.Time) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*OwnerInvitation, error)
	GetPendingOwnerForOrg(ctx context.Context, orgID string) (*OwnerInvitation, error)
	SupersedePendingOwner(ctx context.Context, orgID string) error

	// GetByTokenHashTx and MarkAcceptedTx run on a caller-provided privileged tx
	// so the accept flow commits the membership, session update, and invite
	// status atomically.
	GetByTokenHashTx(ctx context.Context, tx *sql.Tx, tokenHash string) (*OwnerInvitation, error)
	MarkAcceptedTx(ctx context.Context, tx *sql.Tx, id string) error
}

type ownerInviteRepo struct {
	db *sql.DB
}

func NewOwnerInviteRepository(privileged *database.PrivilegedClient) OwnerInviteRepository {
	return &ownerInviteRepo{db: privileged.DB}
}

func (r *ownerInviteRepo) Create(ctx context.Context, orgID, email, inviterID, tokenHash string, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO "invitation" (id, organization_id, email, role, status, inviter_id, token_hash, expires_at, created_at)
		 VALUES ($1, $2, $3, 'owner', 'pending', $4, $5, $6, now())`,
		nanoid.New(), orgID, email, inviterID, tokenHash, expiresAt,
	)
	return err
}

const ownerInviteSelect = `SELECT id, organization_id, email, status, expires_at FROM "invitation"`

func scanOwnerInvitation(row interface{ Scan(...any) error }) (*OwnerInvitation, error) {
	var inv OwnerInvitation
	if err := row.Scan(&inv.ID, &inv.OrganizationID, &inv.Email, &inv.Status, &inv.ExpiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &inv, nil
}

func (r *ownerInviteRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*OwnerInvitation, error) {
	return scanOwnerInvitation(r.db.QueryRowContext(ctx,
		ownerInviteSelect+` WHERE token_hash = $1`, tokenHash))
}

func (r *ownerInviteRepo) GetByTokenHashTx(ctx context.Context, tx *sql.Tx, tokenHash string) (*OwnerInvitation, error) {
	return scanOwnerInvitation(tx.QueryRowContext(ctx,
		ownerInviteSelect+` WHERE token_hash = $1`, tokenHash))
}

func (r *ownerInviteRepo) GetPendingOwnerForOrg(ctx context.Context, orgID string) (*OwnerInvitation, error) {
	return scanOwnerInvitation(r.db.QueryRowContext(ctx,
		ownerInviteSelect+` WHERE organization_id = $1 AND role = 'owner' AND status = 'pending'`, orgID))
}

func (r *ownerInviteRepo) SupersedePendingOwner(ctx context.Context, orgID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE "invitation" SET status = 'revoked'
		 WHERE organization_id = $1 AND role = 'owner' AND status = 'pending'`,
		orgID,
	)
	return err
}

func (r *ownerInviteRepo) MarkAcceptedTx(ctx context.Context, tx *sql.Tx, id string) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE "invitation" SET status = 'accepted', accepted_at = now() WHERE id = $1`,
		id,
	)
	return err
}
