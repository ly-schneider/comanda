package app

import (
	"context"
	"testing"
	"time"

	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/generated/ent"
	"backend/internal/repository"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// captureLifecycle records the hook so a test can drive OnStart/OnStop directly.
type captureLifecycle struct{ hook fx.Hook }

func (c *captureLifecycle) Append(h fx.Hook) { c.hook = h }

// stubDomainRepo satisfies repository.TenantDomainRepository for the verifier
// lifecycle test. ListUnverified signals each pass and returns no rows, so the
// verifier never dereferences the (nil) DomainService or privileged client.
type stubDomainRepo struct{ passes chan struct{} }

func (s *stubDomainRepo) ListUnverified(context.Context, *ent.Client) ([]*ent.TenantDomain, error) {
	select {
	case s.passes <- struct{}{}:
	default:
	}
	return nil, nil
}

func (s *stubDomainRepo) Create(context.Context, string, string) (*ent.TenantDomain, error) {
	return nil, nil
}
func (s *stubDomainRepo) List(context.Context) ([]*ent.TenantDomain, error) { return nil, nil }
func (s *stubDomainRepo) GetByID(context.Context, string) (*ent.TenantDomain, error) {
	return nil, nil
}
func (s *stubDomainRepo) Delete(context.Context, string) error     { return nil }
func (s *stubDomainRepo) SetPrimary(context.Context, string) error { return nil }
func (s *stubDomainRepo) MarkVerified(context.Context, string) (*ent.TenantDomain, error) {
	return nil, nil
}
func (s *stubDomainRepo) ListVerified(context.Context, *ent.Client) ([]*ent.TenantDomain, error) {
	return nil, nil
}
func (s *stubDomainRepo) ListAllVerifiedHostnames(context.Context, *ent.Client) ([]string, error) {
	return nil, nil
}

var _ repository.TenantDomainRepository = (*stubDomainRepo)(nil)

func TestStartDomainVerifierLifecycle(t *testing.T) {
	var cfg config.Config
	cfg.CustomDomain.CNAMETarget = "ingress.example.test"
	cfg.CustomDomain.VerifyInterval = time.Millisecond

	lc := &captureLifecycle{}
	repo := &stubDomainRepo{passes: make(chan struct{}, 1)}

	StartDomainVerifier(lc, cfg, &database.PrivilegedClient{}, repo, nil, zap.NewNop())

	if lc.hook.OnStart == nil || lc.hook.OnStop == nil {
		t.Fatal("StartDomainVerifier did not register start/stop hooks")
	}

	if err := lc.hook.OnStart(context.Background()); err != nil {
		t.Fatalf("OnStart returned error: %v", err)
	}

	select {
	case <-repo.passes:
	case <-time.After(time.Second):
		t.Fatal("verifier goroutine did not run a pass")
	}

	stopped := make(chan error, 1)
	go func() { stopped <- lc.hook.OnStop(context.Background()) }()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("OnStop returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("OnStop did not return promptly (goroutine leak)")
	}
}

func TestStartDomainVerifierDisabledWithoutTarget(t *testing.T) {
	lc := &captureLifecycle{}
	StartDomainVerifier(lc, config.Config{}, &database.PrivilegedClient{}, &stubDomainRepo{}, nil, zap.NewNop())
	if lc.hook.OnStart != nil || lc.hook.OnStop != nil {
		t.Fatal("verifier should register no hooks when no CNAME target is configured")
	}
}
