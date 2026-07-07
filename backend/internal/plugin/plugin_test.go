package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"backend/internal/crypto"
	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/repository"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type fakeRepo struct {
	mu       sync.Mutex
	rows     map[string]map[string]*ent.TenantPlugin
	getCalls int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{rows: map[string]map[string]*ent.TenantPlugin{}}
}

func (f *fakeRepo) set(tenantID string, row *ent.TenantPlugin) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rows[tenantID] == nil {
		f.rows[tenantID] = map[string]*ent.TenantPlugin{}
	}
	f.rows[tenantID][row.PluginKey] = row
}

func (f *fakeRepo) GetByKey(ctx context.Context, key string) (*ent.TenantPlugin, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls++
	tid, _ := repository.TenantIDFromContext(ctx)
	if m, ok := f.rows[tid]; ok {
		if row, ok := m[key]; ok {
			return row, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (f *fakeRepo) ListEnabled(ctx context.Context) ([]*ent.TenantPlugin, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	tid, _ := repository.TenantIDFromContext(ctx)
	out := []*ent.TenantPlugin{}
	for _, row := range f.rows[tid] {
		if row.Enabled {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f *fakeRepo) ListByTenant(_ context.Context, tenantID string) ([]*ent.TenantPlugin, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*ent.TenantPlugin{}
	for _, row := range f.rows[tenantID] {
		out = append(out, row)
	}
	return out, nil
}

func (f *fakeRepo) Upsert(_ context.Context, tenantID string, in repository.TenantPluginUpsert) (*ent.TenantPlugin, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rows[tenantID] == nil {
		f.rows[tenantID] = map[string]*ent.TenantPlugin{}
	}
	row := f.rows[tenantID][in.PluginKey]
	if row == nil {
		row = &ent.TenantPlugin{PluginKey: in.PluginKey}
	}
	row.Enabled = in.Enabled
	row.Config = in.Config
	if in.SecretsEnc != nil {
		enc := in.SecretsEnc
		row.SecretsEnc = &enc
		row.SecretsConfiguredAt = in.SecretsConfiguredAt
	}
	f.rows[tenantID][in.PluginKey] = row
	return row, nil
}

type fixturePlugin struct {
	key  string
	name string
}

func (p fixturePlugin) Key() string       { return p.key }
func (p fixturePlugin) Name() string      { return p.name }
func (p fixturePlugin) Module() fx.Option { return fx.Options() }
func (p fixturePlugin) Routes(_ *generated.ServerInterfaceWrapper, _ Middlewares) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/ping", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("pong"))
		})
	}
}

func ctxTenant(id string) context.Context {
	return repository.ContextWithTenantID(context.Background(), id)
}

func TestEnabledIsPerTenantWithNoBleed(t *testing.T) {
	repo := newFakeRepo()
	repo.set("A", &ent.TenantPlugin{PluginKey: "fx", Enabled: true})
	svc := NewService([]Plugin{fixturePlugin{"fx", "Fixture"}}, repo, nil, zap.NewNop())

	enabledA, err := svc.Enabled(ctxTenant("A"), "fx")
	require.NoError(t, err)
	require.True(t, enabledA)

	enabledB, err := svc.Enabled(ctxTenant("B"), "fx")
	require.NoError(t, err)
	require.False(t, enabledB, "tenant B must not see tenant A's enablement")
}

func TestEnabledCachesPerTenantKey(t *testing.T) {
	repo := newFakeRepo()
	repo.set("A", &ent.TenantPlugin{PluginKey: "fx", Enabled: true})
	svc := NewService([]Plugin{fixturePlugin{"fx", "Fixture"}}, repo, nil, zap.NewNop())

	for i := 0; i < 3; i++ {
		_, err := svc.Enabled(ctxTenant("A"), "fx")
		require.NoError(t, err)
	}
	require.Equal(t, 1, repo.getCalls, "repeated checks for one tenant should hit the repo once")

	_, err := svc.Enabled(ctxTenant("B"), "fx")
	require.NoError(t, err)
	require.Equal(t, 2, repo.getCalls, "a different tenant must not reuse the cached entry")
}

func TestEnabledUnregisteredSkipsRepo(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService([]Plugin{fixturePlugin{"fx", "Fixture"}}, repo, nil, zap.NewNop())

	enabled, err := svc.Enabled(ctxTenant("A"), "nope")
	require.NoError(t, err)
	require.False(t, enabled)
	require.Zero(t, repo.getCalls)
}

func TestRequirePluginMountGating(t *testing.T) {
	repo := newFakeRepo()
	repo.set("A", &ent.TenantPlugin{PluginKey: "fx", Enabled: true})
	repo.set("B", &ent.TenantPlugin{PluginKey: "fx", Enabled: false})
	plugins := []Plugin{fixturePlugin{"fx", "Fixture"}}
	svc := NewService(plugins, repo, nil, zap.NewNop())

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := repository.ContextWithTenantID(req.Context(), req.Header.Get("X-Tenant"))
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	Mount(r, plugins, &generated.ServerInterfaceWrapper{}, Middlewares{}, svc)

	cases := map[string]int{"A": http.StatusOK, "B": http.StatusNotFound, "C": http.StatusNotFound}
	for tenant, want := range cases {
		req := httptest.NewRequest(http.MethodGet, "/plugins/fx/ping", nil)
		req.Header.Set("X-Tenant", tenant)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equalf(t, want, rec.Code, "tenant %s", tenant)
	}
}

func TestRequirePluginUnregistered404(t *testing.T) {
	svc := NewService(nil, newFakeRepo(), nil, zap.NewNop())
	h := svc.RequirePlugin("ghost")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctxTenant("A")))
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestListPublicOmitsDisabled(t *testing.T) {
	repo := newFakeRepo()
	repo.set("A", &ent.TenantPlugin{PluginKey: "on", Enabled: true, Config: json.RawMessage(`{"x":1}`)})
	repo.set("A", &ent.TenantPlugin{PluginKey: "off", Enabled: false})
	plugins := []Plugin{fixturePlugin{"on", "On"}, fixturePlugin{"off", "Off"}}
	svc := NewService(plugins, repo, nil, zap.NewNop())

	out, err := svc.ListPublic(ctxTenant("A"))
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, "on", out[0].Key)
	require.JSONEq(t, `{"x":1}`, string(out[0].Config))
}

func TestSetForTenantUnknownPlugin(t *testing.T) {
	svc := NewService([]Plugin{fixturePlugin{"fx", "Fixture"}}, newFakeRepo(), nil, zap.NewNop())
	enabled := true
	_, err := svc.SetForTenant(context.Background(), "A", "ghost", UpdateInput{Enabled: &enabled})
	require.ErrorIs(t, err, ErrUnknownPlugin)
}

func TestSetForTenantSecretsWriteOnlyRoundTrip(t *testing.T) {
	key := make([]byte, crypto.KeySize)
	for i := range key {
		key[i] = byte(i + 1)
	}
	enc, err := crypto.NewEncryptor(key)
	require.NoError(t, err)

	repo := newFakeRepo()
	svc := NewService([]Plugin{fixturePlugin{"fx", "Fixture"}}, repo, enc, zap.NewNop())

	enabled := true
	state, err := svc.SetForTenant(context.Background(), "A", "fx", UpdateInput{
		Enabled: &enabled,
		Config:  json.RawMessage(`{"maxRedemptions":2}`),
		Secrets: map[string]string{"apiKey": "s3cret"},
	})
	require.NoError(t, err)
	require.True(t, state.Enabled)
	require.NotNil(t, state.SecretsConfiguredAt, "configuredAt must be reported")
	require.JSONEq(t, `{"maxRedemptions":2}`, string(state.Config))

	stored := repo.rows["A"]["fx"]
	require.NotNil(t, stored.SecretsEnc)
	require.NotContains(t, string(*stored.SecretsEnc), "s3cret", "secret must be encrypted at rest")

	secrets, err := svc.Secrets(ctxTenant("A"), "fx")
	require.NoError(t, err)
	require.Equal(t, "s3cret", secrets["apiKey"])
}

func TestSetForTenantPreservesSecretsOnConfigOnlyUpdate(t *testing.T) {
	key := make([]byte, crypto.KeySize)
	enc, err := crypto.NewEncryptor(key)
	require.NoError(t, err)
	repo := newFakeRepo()
	svc := NewService([]Plugin{fixturePlugin{"fx", "Fixture"}}, repo, enc, zap.NewNop())

	_, err = svc.SetForTenant(context.Background(), "A", "fx", UpdateInput{
		Secrets: map[string]string{"apiKey": "keep-me"},
	})
	require.NoError(t, err)

	disabled := false
	_, err = svc.SetForTenant(context.Background(), "A", "fx", UpdateInput{Enabled: &disabled})
	require.NoError(t, err)

	secrets, err := svc.Secrets(ctxTenant("A"), "fx")
	require.NoError(t, err)
	require.Equal(t, "keep-me", secrets["apiKey"], "a config/enable-only update must not wipe secrets")
}
