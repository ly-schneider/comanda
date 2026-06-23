package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"backend/internal/config"
	"backend/internal/database"
	nanoid "backend/internal/id"
	"backend/internal/repository"

	"go.uber.org/zap"
)

var (
	ErrTenantNotLinked = errors.New("tenant is not linked to an organization")
	ErrOwnerExists     = errors.New("organization already has an owner")
	ErrEmailMismatch   = errors.New("invite is for a different email")
)

// OwnerInvitePublic is the unauthenticated view of an owner invite.
type OwnerInvitePublic struct {
	Email     string
	Status    string
	ExpiresAt time.Time
}

type OwnerInviteService interface {
	Create(ctx context.Context, tenantID, inviterID, email string) (*OwnerInvitePublic, error)
	Verify(ctx context.Context, token string) (*OwnerInvitePublic, error)
	Accept(ctx context.Context, token, sessionUserID, sessionUserEmail, sessionID string) (orgID string, err error)
}

type ownerInviteService struct {
	cfg         config.Config
	db          *sql.DB
	inviteRepo  repository.OwnerInviteRepository
	memberRepo  repository.MemberRepository
	emailSvc    EmailService
	invalidator repository.SessionInvalidator
	logger      *zap.Logger
}

func NewOwnerInviteService(
	cfg config.Config,
	privileged *database.PrivilegedClient,
	inviteRepo repository.OwnerInviteRepository,
	memberRepo repository.MemberRepository,
	emailSvc EmailService,
	sessions repository.SessionRepository,
	logger *zap.Logger,
) OwnerInviteService {
	invalidator, _ := sessions.(repository.SessionInvalidator)
	return &ownerInviteService{
		cfg:         cfg,
		db:          privileged.DB,
		inviteRepo:  inviteRepo,
		memberRepo:  memberRepo,
		emailSvc:    emailSvc,
		invalidator: invalidator,
		logger:      logger,
	}
}

func (s *ownerInviteService) orgForTenant(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, tenantID string) (string, error) {
	var orgID sql.NullString
	err := q.QueryRowContext(ctx, `SELECT organization_id FROM tenant WHERE id = $1`, tenantID).Scan(&orgID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", repository.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if !orgID.Valid || orgID.String == "" {
		return "", ErrTenantNotLinked
	}
	return orgID.String, nil
}

func (s *ownerInviteService) Create(ctx context.Context, tenantID, inviterID, email string) (*OwnerInvitePublic, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, ErrInvalidToken
	}

	orgID, err := s.orgForTenant(ctx, s.db, tenantID)
	if err != nil {
		return nil, err
	}

	owners, err := s.memberRepo.CountOwnersForOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if owners > 0 {
		return nil, ErrOwnerExists
	}

	if err := s.inviteRepo.SupersedePendingOwner(ctx, orgID); err != nil {
		return nil, err
	}

	token, tokenHash, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	expiresAt := time.Now().Add(defaultInviteTTL)

	if err := s.inviteRepo.Create(ctx, orgID, email, inviterID, tokenHash, expiresAt); err != nil {
		return nil, err
	}

	inviteURL := s.buildAcceptURL(token)
	_ = s.emailSvc.SendInviteEmail(ctx, email, inviteURL, expiresAt)

	return &OwnerInvitePublic{Email: email, Status: "pending", ExpiresAt: expiresAt}, nil
}

func (s *ownerInviteService) Verify(ctx context.Context, token string) (*OwnerInvitePublic, error) {
	if token == "" {
		return nil, ErrInvalidToken
	}
	inv, err := s.inviteRepo.GetByTokenHash(ctx, hashToken(token))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, err
	}
	return &OwnerInvitePublic{Email: inv.Email, Status: inv.Status, ExpiresAt: inv.ExpiresAt}, nil
}

func (s *ownerInviteService) Accept(ctx context.Context, token, sessionUserID, sessionUserEmail, sessionID string) (string, error) {
	if token == "" {
		return "", ErrInvalidToken
	}
	if sessionID == "" {
		return "", ErrInvalidToken
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	inv, err := s.inviteRepo.GetByTokenHashTx(ctx, tx, hashToken(token))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", ErrInvalidToken
		}
		return "", err
	}

	switch inv.Status {
	case "pending":
	case "accepted":
		return "", ErrInviteAlreadyUsed
	default:
		return "", ErrInviteNotPending
	}
	if time.Now().After(inv.ExpiresAt) {
		return "", ErrInviteExpired
	}

	if !strings.EqualFold(strings.TrimSpace(sessionUserEmail), strings.TrimSpace(inv.Email)) {
		return "", ErrEmailMismatch
	}

	var owners int
	if err := tx.QueryRowContext(ctx,
		`SELECT count(*) FROM "member" WHERE organization_id = $1 AND role = 'owner' AND user_id <> $2`,
		inv.OrganizationID, sessionUserID,
	).Scan(&owners); err != nil {
		return "", err
	}
	if owners > 0 {
		return "", ErrOwnerExists
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO "member" (id, organization_id, user_id, role, created_at)
		 VALUES ($1, $2, $3, 'owner', now())
		 ON CONFLICT (organization_id, user_id) DO UPDATE SET role = 'owner'`,
		nanoid.New(), inv.OrganizationID, sessionUserID,
	); err != nil {
		return "", err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE "session" SET active_organization_id = $1 WHERE id = $2`,
		inv.OrganizationID, sessionID,
	); err != nil {
		return "", err
	}

	if err := s.inviteRepo.MarkAcceptedTx(ctx, tx, inv.ID); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}

	if s.invalidator != nil {
		s.invalidator.InvalidateUser(sessionUserID)
	}

	return inv.OrganizationID, nil
}

func (s *ownerInviteService) buildAcceptURL(token string) string {
	baseURL := s.cfg.App.PublicBaseURL
	if baseURL == "" {
		baseURL = "http://localhost:3000"
	}
	return fmt.Sprintf("%s/onboarding/accept?token=%s", baseURL, token)
}
