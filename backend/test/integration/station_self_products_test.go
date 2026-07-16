package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/api"
	"backend/internal/auth"
	"backend/internal/database"
	"backend/internal/enttenant"
	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/repository"
	"backend/internal/service"

	nanoid "backend/internal/id"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// stationHTTPHarness stands up the device-authenticated station routes with the
// REAL RequireDevice(STATION) middleware over a per-request tenant-bound tx, so
// the self-service product endpoints are exercised end to end: the gate, the
// device-scoped assignment, and the RLS product validation.
type stationHTTPHarness struct {
	env    *rlsEnv
	router http.Handler
}

func newStationHTTPHarness(t *testing.T, env *rlsEnv) *stationHTTPHarness {
	t.Helper()
	staticClient := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, env.app)))
	repos := NewRepositories(staticClient)

	settingsSvc := service.NewSettingsService(repos.Settings, repos.Jeton, repos.Product)
	stationSvc := service.NewStationService(
		TestConfig(), staticClient,
		repos.Device, repos.DeviceProduct, repos.OrderLine, repos.OrderRedemption, repos.Idempotency,
	)
	handlers := api.NewStationTestHandlers(settingsSvc, stationSvc, NewProductSvc(repos), zap.NewNop())

	bindingRepo := repository.NewDeviceBindingRepository(staticClient)
	sessionRepo := repository.NewSessionRepository(staticClient)
	deviceMw := auth.NewDeviceAuthMiddleware(bindingRepo, sessionRepo, zap.NewNop())

	wrapper := generated.ServerInterfaceWrapper{
		Handler: handlers,
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		},
	}

	h := &stationHTTPHarness{env: env}
	r := chi.NewRouter()
	r.Route("/v1", func(v1 chi.Router) {
		v1.Use(h.tenantTx)
		v1.Group(func(station chi.Router) {
			station.Use(deviceMw.RequireDevice(auth.DeviceTypeStation))
			station.Get("/stations/me", wrapper.GetCurrentStation)
			station.Get("/stations/me/products", wrapper.GetCurrentStationProducts)
			station.Put("/stations/me/products", wrapper.SetCurrentStationProducts)
		})
	})
	h.router = r
	return h
}

func (h *stationHTTPHarness) tenantTx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.Header.Get("X-Test-Tenant")
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
		if _, err := tx.ExecContext(ctx,
			"SELECT set_config('app.tenant_id', $1, true), set_config('app.is_platform_admin', 'false', true)",
			tenantID,
		); err != nil {
			_ = tx.Rollback()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		txClient := ent.NewClient(ent.Driver(database.NewTxEntDriver(tx)))
		enttenant.Register(txClient)

		ctx = repository.ContextWithClient(ctx, txClient)
		ctx = repository.ContextWithRequestTx(ctx, tx)
		ctx = repository.ContextWithTenantID(ctx, tenantID)
		ctx = enttenant.WithTenant(ctx, tenantID)

		next.ServeHTTP(w, r.WithContext(ctx))
		_ = tx.Commit()
	})
}

func (h *stationHTTPHarness) do(t *testing.T, method, path, tenantID, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("X-Test-Tenant", tenantID)
	req.Header.Set("Authorization", "Bearer "+token)
	if rdr != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	return rec
}

// setStationSelfManage upserts the tenant's settings row with the flag value.
func (e *rlsEnv) setStationSelfManage(t *testing.T, tenantID string, on bool) {
	t.Helper()
	e.privExec(t,
		`INSERT INTO settings (id, tenant_id, station_self_manage_products) VALUES ($1, $1, $2)
		 ON CONFLICT (id) DO UPDATE SET station_self_manage_products = EXCLUDED.station_self_manage_products`,
		tenantID, on,
	)
}

func (e *rlsEnv) seedStation(t *testing.T, tenantID string, productIDs ...string) string {
	t.Helper()
	stationID := nanoid.New()
	e.privExec(t,
		`INSERT INTO device (id, name, device_key, type, status, tenant_id)
		 VALUES ($1, 'St', $2, 'STATION', 'approved', $3)`,
		stationID, "stn-"+randSuffix(), tenantID,
	)
	for _, pid := range productIDs {
		e.privExec(t,
			`INSERT INTO device_product (device_id, product_id, tenant_id) VALUES ($1, $2, $3)`,
			stationID, pid, tenantID,
		)
	}
	return stationID
}

func deviceProductIDs(t *testing.T, env *rlsEnv, deviceID string) []string {
	t.Helper()
	rows, err := env.priv.Query(`SELECT product_id FROM device_product WHERE device_id = $1 ORDER BY product_id`, deviceID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		out = append(out, id)
	}
	require.NoError(t, rows.Err())
	return out
}

type stationProductListBody struct {
	Items []struct {
		ProductID string `json:"productId"`
	} `json:"items"`
}

func productIDsIn(body stationProductListBody) []string {
	out := make([]string, 0, len(body.Items))
	for _, it := range body.Items {
		out = append(out, it.ProductID)
	}
	return out
}

func TestStationSelfProducts_DisabledReturns403(t *testing.T) {
	env := newRLSEnv(t)
	tenant, products := env.seedTenantWithProducts(t, "ssp-off-"+randSuffix(), 2)
	owner := env.seedUser(t)
	station, token := env.seedDeviceToken(t, tenant, owner, "STATION")
	require.NotEmpty(t, station)
	env.setStationSelfManage(t, tenant, false)

	h := newStationHTTPHarness(t, env)

	getRec := h.do(t, http.MethodGet, "/v1/stations/me/products", tenant, token, nil)
	require.Equal(t, http.StatusForbidden, getRec.Code)

	putRec := h.do(t, http.MethodPut, "/v1/stations/me/products", tenant, token, map[string]any{"productIds": products})
	require.Equal(t, http.StatusForbidden, putRec.Code)

	require.Empty(t, deviceProductIDs(t, env, station), "a rejected PUT must not persist assignments")
}

func TestStationSelfProducts_EnabledSetsOwnProducts(t *testing.T) {
	env := newRLSEnv(t)
	tenant, products := env.seedTenantWithProducts(t, "ssp-on-"+randSuffix(), 3)
	owner := env.seedUser(t)
	station, token := env.seedDeviceToken(t, tenant, owner, "STATION")
	env.setStationSelfManage(t, tenant, true)

	h := newStationHTTPHarness(t, env)

	meRec := h.do(t, http.MethodGet, "/v1/stations/me", tenant, token, nil)
	require.Equal(t, http.StatusOK, meRec.Code)
	var me struct {
		SelfManageProducts *bool `json:"selfManageProducts"`
	}
	require.NoError(t, json.Unmarshal(meRec.Body.Bytes(), &me))
	require.NotNil(t, me.SelfManageProducts)
	require.True(t, *me.SelfManageProducts)

	want := []string{products[0], products[1]}
	putRec := h.do(t, http.MethodPut, "/v1/stations/me/products", tenant, token, map[string]any{"productIds": want})
	require.Equal(t, http.StatusOK, putRec.Code)

	var putBody stationProductListBody
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &putBody))
	require.ElementsMatch(t, want, productIDsIn(putBody))

	getRec := h.do(t, http.MethodGet, "/v1/stations/me/products", tenant, token, nil)
	require.Equal(t, http.StatusOK, getRec.Code)
	var getBody stationProductListBody
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getBody))
	require.ElementsMatch(t, want, productIDsIn(getBody))

	require.ElementsMatch(t, want, deviceProductIDs(t, env, station))

	putRec2 := h.do(t, http.MethodPut, "/v1/stations/me/products", tenant, token, map[string]any{"productIds": []string{products[2]}})
	require.Equal(t, http.StatusOK, putRec2.Code)
	require.Equal(t, []string{products[2]}, deviceProductIDs(t, env, station))
}

func TestStationSelfProducts_DeviceScoped(t *testing.T) {
	env := newRLSEnv(t)
	tenant, products := env.seedTenantWithProducts(t, "ssp-scope-"+randSuffix(), 3)
	owner := env.seedUser(t)
	caller, token := env.seedDeviceToken(t, tenant, owner, "STATION")
	other := env.seedStation(t, tenant, products[2]) // a different station with its own assignment
	env.setStationSelfManage(t, tenant, true)

	h := newStationHTTPHarness(t, env)

	putRec := h.do(t, http.MethodPut, "/v1/stations/me/products", tenant, token, map[string]any{"productIds": []string{products[0]}})
	require.Equal(t, http.StatusOK, putRec.Code)

	require.Equal(t, []string{products[0]}, deviceProductIDs(t, env, caller), "caller's own assignment updated")
	require.Equal(t, []string{products[2]}, deviceProductIDs(t, env, other), "another station must be untouched")
}

func TestStationSelfProducts_ForeignProductDropped(t *testing.T) {
	env := newRLSEnv(t)
	tenant, products := env.seedTenantWithProducts(t, "ssp-fk-a-"+randSuffix(), 1)
	_, foreign := env.seedTenantWithProducts(t, "ssp-fk-b-"+randSuffix(), 1)
	owner := env.seedUser(t)
	station, token := env.seedDeviceToken(t, tenant, owner, "STATION")
	env.setStationSelfManage(t, tenant, true)

	h := newStationHTTPHarness(t, env)

	putRec := h.do(t, http.MethodPut, "/v1/stations/me/products", tenant, token,
		map[string]any{"productIds": []string{products[0], foreign[0]}})
	require.Equal(t, http.StatusOK, putRec.Code)

	var body stationProductListBody
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &body))
	require.Equal(t, []string{products[0]}, productIDsIn(body), "a foreign-tenant product id must be dropped")
	require.Equal(t, []string{products[0]}, deviceProductIDs(t, env, station))
}
