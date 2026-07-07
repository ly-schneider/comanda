package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/api"
	"backend/internal/database"
	"backend/internal/enttenant"
	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/plugin"
	"backend/internal/repository"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// fixturePlugin is a minimal in-test plugin proving registration → route
// mounting → gating end to end, without shipping anything in the production
// registry.
type fixturePlugin struct{}

func (fixturePlugin) Key() string       { return "fixture" }
func (fixturePlugin) Name() string      { return "Fixture" }
func (fixturePlugin) Module() fx.Option { return fx.Options() }
func (fixturePlugin) Routes(_ *generated.ServerInterfaceWrapper, _ plugin.Middlewares) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/ping", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("pong"))
		})
	}
}

type pluginHTTPHarness struct {
	env    *rlsEnv
	router http.Handler
	svc    plugin.Service
}

func newPluginHTTPHarness(t *testing.T, env *rlsEnv) *pluginHTTPHarness {
	t.Helper()
	staticClient := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, env.app)))
	repo := repository.NewTenantPluginRepository(staticClient)
	plugins := []plugin.Plugin{fixturePlugin{}}
	svc := plugin.NewService(plugins, repo, testQREncryptor(), zap.NewNop())

	handlers := api.NewPluginHandlers(svc, zap.NewNop())
	wrapper := generated.ServerInterfaceWrapper{
		Handler: handlers,
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		},
	}

	h := &pluginHTTPHarness{env: env, svc: svc}
	r := chi.NewRouter()
	r.Route("/v1", func(v1 chi.Router) {
		v1.Use(h.tx)
		v1.Get("/plugins", wrapper.ListPlugins)
		v1.Get("/platform/tenants/{tenantId}/plugins", wrapper.ListTenantPlugins)
		v1.Put("/platform/tenants/{tenantId}/plugins/{key}", wrapper.UpdateTenantPlugin)
		plugin.Mount(v1, plugins, &wrapper, plugin.Middlewares{}, svc)
	})
	h.router = r
	return h
}

// tx binds either a tenant tx (X-Test-Tenant) or a platform-admin tx
// (X-Test-Platform-Admin: true), mirroring tenantMw.Tx, and commits so writes
// persist across the separate read requests.
func (h *pluginHTTPHarness) tx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		conn, err := h.env.app.Conn(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer func() { _ = conn.Close() }()
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		platform := r.Header.Get("X-Test-Platform-Admin") == "true"
		tenantID := r.Header.Get("X-Test-Tenant")
		adminGUC := "false"
		if platform {
			adminGUC = "true"
			tenantID = ""
		}
		if _, err := tx.ExecContext(ctx,
			"SELECT set_config('app.tenant_id', $1, true), set_config('app.is_platform_admin', $2, true)",
			tenantID, adminGUC,
		); err != nil {
			_ = tx.Rollback()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		txClient := ent.NewClient(ent.Driver(database.NewTxEntDriver(tx)))
		enttenant.Register(txClient)
		ctx = repository.ContextWithClient(ctx, txClient)
		ctx = repository.ContextWithRequestTx(ctx, tx)
		if platform {
			ctx = enttenant.WithPlatform(ctx)
		} else {
			ctx = repository.ContextWithTenantID(ctx, tenantID)
			ctx = enttenant.WithTenant(ctx, tenantID)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
		_ = tx.Commit()
	})
}

func (h *pluginHTTPHarness) do(t *testing.T, method, path string, headers map[string]string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if rdr != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	return rec
}

func (h *pluginHTTPHarness) get(t *testing.T, path, tenantID string) *httptest.ResponseRecorder {
	return h.do(t, http.MethodGet, path, map[string]string{"X-Test-Tenant": tenantID}, nil)
}

func (h *pluginHTTPHarness) getPlatform(t *testing.T, tenantID string) *httptest.ResponseRecorder {
	return h.do(t, http.MethodGet, "/v1/platform/tenants/"+tenantID+"/plugins",
		map[string]string{"X-Test-Platform-Admin": "true"}, nil)
}

func (h *pluginHTTPHarness) putPlatform(t *testing.T, tenantID, key string, body any) *httptest.ResponseRecorder {
	return h.do(t, http.MethodPut, "/v1/platform/tenants/"+tenantID+"/plugins/"+key,
		map[string]string{"X-Test-Platform-Admin": "true"}, body)
}

func TestPluginHTTP_EnablementGatingAndSecrets(t *testing.T) {
	env := newRLSEnv(t)
	h := newPluginHTTPHarness(t, env)
	tenantA := env.seedTenant(t, "tp-http-a-"+randSuffix())
	tenantB := env.seedTenant(t, "tp-http-b-"+randSuffix())

	const secret = "s3cret-value"

	t.Run("platform PUT enables and stores write-only secrets", func(t *testing.T) {
		rec := h.putPlatform(t, tenantA, "fixture", map[string]any{
			"enabled": true,
			"config":  map[string]any{"maxRedemptions": 2},
			"secrets": map[string]string{"apiKey": secret},
		})
		require.Equal(t, http.StatusOK, rec.Code)
		require.NotContains(t, rec.Body.String(), secret, "secret must never be returned")

		var state map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &state))
		require.Equal(t, true, state["enabled"])
		require.NotNil(t, state["secretsConfiguredAt"], "configuredAt must be reported")
		require.Contains(t, rec.Body.String(), "maxRedemptions")
	})

	t.Run("public list shows enabled plugin for its tenant only, without secrets", func(t *testing.T) {
		recA := h.get(t, "/v1/plugins", tenantA)
		require.Equal(t, http.StatusOK, recA.Code)
		require.Contains(t, recA.Body.String(), "fixture")
		require.Contains(t, recA.Body.String(), "maxRedemptions")
		require.NotContains(t, recA.Body.String(), secret)
		require.NotContains(t, recA.Body.String(), "secretsConfiguredAt")

		recB := h.get(t, "/v1/plugins", tenantB)
		require.Equal(t, http.StatusOK, recB.Code)
		require.NotContains(t, recB.Body.String(), "fixture")
	})

	t.Run("plugin route 404s for a tenant it is not enabled for", func(t *testing.T) {
		require.Equal(t, http.StatusOK, h.get(t, "/v1/plugins/fixture/ping", tenantA).Code)
		require.Equal(t, http.StatusNotFound, h.get(t, "/v1/plugins/fixture/ping", tenantB).Code)
	})

	t.Run("platform list reports per-tenant state without secrets", func(t *testing.T) {
		recA := h.getPlatform(t, tenantA)
		require.Equal(t, http.StatusOK, recA.Code)
		require.Contains(t, recA.Body.String(), `"enabled":true`)
		require.Contains(t, recA.Body.String(), "secretsConfiguredAt")
		require.NotContains(t, recA.Body.String(), secret)

		recB := h.getPlatform(t, tenantB)
		require.Equal(t, http.StatusOK, recB.Code)
		require.Contains(t, recB.Body.String(), `"enabled":false`)
	})

	t.Run("disabling hides the plugin again", func(t *testing.T) {
		rec := h.putPlatform(t, tenantA, "fixture", map[string]any{"enabled": false})
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, http.StatusNotFound, h.get(t, "/v1/plugins/fixture/ping", tenantA).Code)
		require.NotContains(t, h.get(t, "/v1/plugins", tenantA).Body.String(), "fixture")
	})

	t.Run("unknown plugin key is rejected", func(t *testing.T) {
		rec := h.putPlatform(t, tenantA, "ghost", map[string]any{"enabled": true})
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})
}
