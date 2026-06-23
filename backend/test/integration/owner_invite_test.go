package integration

import (
	"context"
	"testing"
	"time"

	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/repository"
	"backend/internal/service"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// noopEmail satisfies service.EmailService without sending anything.
type noopEmail struct{}

func (noopEmail) SendInviteEmail(context.Context, string, string, time.Time) error { return nil }
func (noopEmail) SendOTPEmail(context.Context, string, string, service.OTPType) error {
	return nil
}
func (noopEmail) SendReceiptEmail(context.Context, string, service.ReceiptEmailData) error {
	return nil
}

func (e *rlsEnv) ownerInviteSvc(t *testing.T) service.OwnerInviteService {
	t.Helper()
	priv := &database.PrivilegedClient{DB: e.priv}
	sessions := repository.NewCachedSessionRepository(repository.NewSessionRepository(e.privClient()))
	return service.NewOwnerInviteService(
		config.Config{App: config.AppConfig{PublicBaseURL: "http://localhost:3000"}},
		priv,
		repository.NewOwnerInviteRepository(priv),
		repository.NewMemberRepository(priv),
		noopEmail{},
		sessions,
		zap.NewNop(),
	)
}

// ownerInviteTokenHash reads the token_hash the service stored for an org so a
// test can drive Accept/Verify without intercepting the (noop) email.
func (e *rlsEnv) pendingOwnerTokenInfo(t *testing.T, orgID string) (id, email string) {
	t.Helper()
	require.NoError(t, e.priv.QueryRow(
		`SELECT id, email FROM "invitation" WHERE organization_id = $1 AND role = 'owner' AND status = 'pending'`,
		orgID,
	).Scan(&id, &email))
	return id, email
}

// setInvitationToken overwrites the token_hash for an invitation so the test can
// present a known plaintext token to Verify/Accept (Create stores a random one
// and only the email carries the plaintext in production).
func (e *rlsEnv) setInvitationToken(t *testing.T, invitationID, tokenHash string) {
	t.Helper()
	e.privExec(t, `UPDATE "invitation" SET token_hash = $1 WHERE id = $2`, tokenHash, invitationID)
}

// tokenHashOf mirrors the service's token hashing (SHA-256 hex) so a test can
// present a known plaintext token; the service stores a random one and only the
// email carries the plaintext in production.
func tokenHashOf(token string) string {
	return repository.HashToken(token)
}

func TestOwnerInviteHappyPath(t *testing.T) {
	env := newRLSEnv(t)
	svc := env.ownerInviteSvc(t)
	ctx := context.Background()

	tenantID := env.seedTenant(t, "owni-"+randSuffix())
	orgID := env.seedOrgForTenant(t, tenantID, "owni-"+randSuffix())
	inviter := env.seedUser(t)

	email := "owner-" + randSuffix() + "@example.test"
	_, err := svc.Create(ctx, tenantID, inviter, email)
	require.NoError(t, err)

	token := "owni-tok-" + randSuffix()
	invID, storedEmail := env.pendingOwnerTokenInfo(t, orgID)
	require.Equal(t, email, storedEmail)
	env.setInvitationToken(t, invID, tokenHashOf(token))

	// The accepting user registered with the invited email.
	userID := env.seedUserWithEmail(t, email)
	sessionID := env.seedSession(t, userID)

	gotOrg, err := svc.Accept(ctx, token, userID, email, sessionID)
	require.NoError(t, err)
	require.Equal(t, orgID, gotOrg)

	// Member(owner) created, session's active org set, invite accepted.
	var role string
	require.NoError(t, env.priv.QueryRow(
		`SELECT role FROM "member" WHERE organization_id = $1 AND user_id = $2`, orgID, userID).Scan(&role))
	require.Equal(t, "owner", role)

	var activeOrg string
	require.NoError(t, env.priv.QueryRow(
		`SELECT active_organization_id FROM "session" WHERE id = $1`, sessionID).Scan(&activeOrg))
	require.Equal(t, orgID, activeOrg)

	var status string
	require.NoError(t, env.priv.QueryRow(
		`SELECT status FROM "invitation" WHERE id = $1`, invID).Scan(&status))
	require.Equal(t, "accepted", status)
}

func TestOwnerInviteEmailMismatch(t *testing.T) {
	env := newRLSEnv(t)
	svc := env.ownerInviteSvc(t)
	ctx := context.Background()

	tenantID := env.seedTenant(t, "owni-mm-"+randSuffix())
	orgID := env.seedOrgForTenant(t, tenantID, "owni-mm-"+randSuffix())
	inviter := env.seedUser(t)

	email := "invited-" + randSuffix() + "@example.test"
	_, err := svc.Create(ctx, tenantID, inviter, email)
	require.NoError(t, err)
	token := "owni-tok-" + randSuffix()
	invID, _ := env.pendingOwnerTokenInfo(t, orgID)
	env.setInvitationToken(t, invID, tokenHashOf(token))

	otherEmail := "someone-else-" + randSuffix() + "@example.test"
	userID := env.seedUserWithEmail(t, otherEmail)
	sessionID := env.seedSession(t, userID)

	_, err = svc.Accept(ctx, token, userID, otherEmail, sessionID)
	require.ErrorIs(t, err, service.ErrEmailMismatch)
}

func TestOwnerInviteExpired(t *testing.T) {
	env := newRLSEnv(t)
	svc := env.ownerInviteSvc(t)
	ctx := context.Background()

	tenantID := env.seedTenant(t, "owni-exp-"+randSuffix())
	orgID := env.seedOrgForTenant(t, tenantID, "owni-exp-"+randSuffix())
	inviter := env.seedUser(t)

	email := "exp-" + randSuffix() + "@example.test"
	_, err := svc.Create(ctx, tenantID, inviter, email)
	require.NoError(t, err)
	token := "owni-tok-" + randSuffix()
	invID, _ := env.pendingOwnerTokenInfo(t, orgID)
	env.setInvitationToken(t, invID, tokenHashOf(token))
	env.privExec(t, `UPDATE "invitation" SET expires_at = now() - interval '1 hour' WHERE id = $1`, invID)

	userID := env.seedUserWithEmail(t, email)
	sessionID := env.seedSession(t, userID)

	_, err = svc.Accept(ctx, token, userID, email, sessionID)
	require.ErrorIs(t, err, service.ErrInviteExpired)
}

func TestOwnerInviteAlreadyUsed(t *testing.T) {
	env := newRLSEnv(t)
	svc := env.ownerInviteSvc(t)
	ctx := context.Background()

	tenantID := env.seedTenant(t, "owni-used-"+randSuffix())
	orgID := env.seedOrgForTenant(t, tenantID, "owni-used-"+randSuffix())
	inviter := env.seedUser(t)

	email := "used-" + randSuffix() + "@example.test"
	_, err := svc.Create(ctx, tenantID, inviter, email)
	require.NoError(t, err)
	token := "owni-tok-" + randSuffix()
	invID, _ := env.pendingOwnerTokenInfo(t, orgID)
	env.setInvitationToken(t, invID, tokenHashOf(token))

	userID := env.seedUserWithEmail(t, email)
	sessionID := env.seedSession(t, userID)
	_, err = svc.Accept(ctx, token, userID, email, sessionID)
	require.NoError(t, err)

	_, err = svc.Accept(ctx, token, userID, email, sessionID)
	require.ErrorIs(t, err, service.ErrInviteAlreadyUsed)
}

func TestOwnerInviteOneOwnerGuard(t *testing.T) {
	env := newRLSEnv(t)
	svc := env.ownerInviteSvc(t)
	ctx := context.Background()

	tenantID := env.seedTenant(t, "owni-guard-"+randSuffix())
	orgID := env.seedOrgForTenant(t, tenantID, "owni-guard-"+randSuffix())
	inviter := env.seedUser(t)

	existingOwner := env.seedUser(t)
	env.seedMember(t, orgID, existingOwner, "owner")

	_, err := svc.Create(ctx, tenantID, inviter, "second-"+randSuffix()+"@example.test")
	require.ErrorIs(t, err, service.ErrOwnerExists)
}

func TestOwnerInviteSupersedesPending(t *testing.T) {
	env := newRLSEnv(t)
	svc := env.ownerInviteSvc(t)
	ctx := context.Background()

	tenantID := env.seedTenant(t, "owni-sup-"+randSuffix())
	orgID := env.seedOrgForTenant(t, tenantID, "owni-sup-"+randSuffix())
	inviter := env.seedUser(t)

	_, err := svc.Create(ctx, tenantID, inviter, "first-"+randSuffix()+"@example.test")
	require.NoError(t, err)
	_, err = svc.Create(ctx, tenantID, inviter, "second-"+randSuffix()+"@example.test")
	require.NoError(t, err)

	var pending int
	require.NoError(t, env.priv.QueryRow(
		`SELECT count(*) FROM "invitation" WHERE organization_id = $1 AND role = 'owner' AND status = 'pending'`,
		orgID).Scan(&pending))
	require.Equal(t, 1, pending)

	var revoked int
	require.NoError(t, env.priv.QueryRow(
		`SELECT count(*) FROM "invitation" WHERE organization_id = $1 AND role = 'owner' AND status = 'revoked'`,
		orgID).Scan(&revoked))
	require.Equal(t, 1, revoked)
}

func TestOwnerInviteTenantNotLinked(t *testing.T) {
	env := newRLSEnv(t)
	svc := env.ownerInviteSvc(t)
	ctx := context.Background()

	tenantID := env.seedTenant(t, "owni-nolink-"+randSuffix())
	inviter := env.seedUser(t)

	_, err := svc.Create(ctx, tenantID, inviter, "x-"+randSuffix()+"@example.test")
	require.ErrorIs(t, err, service.ErrTenantNotLinked)
}
