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

// refundHTTPHarness stands up a chi router that mirrors the production refund
// route groups, driving the REAL middleware: RequireDevice(POS) on the POS
// surface and RequireTenantRole on the admin surface, over a per-request
// tenant-bound tx. The session-cookie validation is replaced by an injected
// user id (X-Test-User) — exactly what auth.SessionMiddleware would set — so the
// tenant-tier gate itself is exercised.
type refundHTTPHarness struct {
	env    *rlsEnv
	router http.Handler
}

func newRefundHTTPHarness(t *testing.T, env *rlsEnv) *refundHTTPHarness {
	t.Helper()
	staticClient := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, env.app)))
	repos := NewRepositories(staticClient)

	refundSvc := service.NewRefundService(
		repos.Order,
		repos.Refund,
		repos.Inventory,
		stubResolver{p: &stubRefundProvider{}},
		NewQRKeySvc(repos),
		nil,
		zap.NewNop(),
	)
	handlers := api.NewRefundHandlers(refundSvc, zap.NewNop())

	bindingRepo := repository.NewDeviceBindingRepository(staticClient)
	sessionRepo := repository.NewSessionRepository(staticClient)
	deviceMw := auth.NewDeviceAuthMiddleware(bindingRepo, sessionRepo, zap.NewNop())
	sessionMw := auth.NewSessionMiddleware(nil, nil, zap.NewNop())

	h := &refundHTTPHarness{env: env}

	wrapper := generated.ServerInterfaceWrapper{
		Handler: handlers,
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		},
	}

	r := chi.NewRouter()
	r.Route("/v1", func(v1 chi.Router) {
		v1.Use(h.tenantTx)

		v1.Group(func(pos chi.Router) {
			pos.Use(deviceMw.RequireDevice(auth.DeviceTypePOS))
			pos.Get("/pos/orders/lookup", wrapper.LookupPosOrders)
			pos.Get("/pos/orders/{orderId}/refunds", wrapper.GetPosRefundView)
			pos.Post("/pos/orders/{orderId}/refunds", wrapper.CreatePosLineRefund)
			pos.Post("/pos/orders/{orderId}/cancel", wrapper.CancelPosOrder)
		})

		v1.Group(func(ops chi.Router) {
			ops.Use(injectTestUser)
			ops.Use(sessionMw.RequireTenantRole(auth.TierOps))
			ops.Get("/orders/lookup", wrapper.LookupOrders)
			ops.Get("/orders/{orderId}/refunds", wrapper.GetRefundView)
			ops.Post("/orders/{orderId}/refunds", wrapper.CreateLineRefund)
			ops.Post("/orders/{orderId}/cancel", wrapper.CancelOrder)
		})

		v1.Group(func(aa chi.Router) {
			aa.Use(injectTestUser)
			aa.Use(sessionMw.RequireTenantRole(auth.TierAppAdmin))
			aa.Post("/orders/{orderId}/refunds/arbitrary", wrapper.CreateArbitraryRefund)
		})
	})

	h.router = r
	return h
}

// tenantTx binds the per-request RLS tenant tx from the X-Test-Tenant header,
// mirroring tenantMw.Tx so the refund service + RequireDevice/RequireTenantRole
// all run on the bound, GUC-scoped connection.
func (h *refundHTTPHarness) tenantTx(next http.Handler) http.Handler {
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

// injectTestUser stands in for the session middleware: it binds X-Test-User as
// the authenticated user so RequireTenantRole resolves its membership role.
func injectTestUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if uid := r.Header.Get("X-Test-User"); uid != "" {
			r = r.WithContext(auth.WithUserID(r.Context(), uid))
		}
		next.ServeHTTP(w, r)
	})
}

func (h *refundHTTPHarness) do(t *testing.T, method, path, tenantID string, setAuth func(*http.Request), body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("X-Test-Tenant", tenantID)
	if rdr != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if setAuth != nil {
		setAuth(req)
	}
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	return rec
}

func asBearer(token string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }
}

func asUser(userID string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("X-Test-User", userID) }
}

// ── seeding ────────────────────────────────────────────────────────────────

func (e *rlsEnv) seedDeviceToken(t *testing.T, tenantID, ownerUserID, deviceType string) (deviceID, token string) {
	t.Helper()
	deviceID = nanoid.New()
	e.privExec(t,
		`INSERT INTO device (id, name, device_key, type, status, tenant_id) VALUES ($1, $2, $3, $4, 'approved', $5)`,
		deviceID, deviceType+" device", "dk-"+randSuffix(), deviceType, tenantID,
	)
	token = "bt_" + randSuffix() + randSuffix()
	e.privExec(t,
		`INSERT INTO "session" (id, token, user_id, expires_at, created_at, updated_at)
		 VALUES ($1, $2, $3, now() + interval '90 days', now(), now())`,
		"sess_"+randSuffix(), token, ownerUserID,
	)
	e.privExec(t,
		`INSERT INTO device_binding (id, device_type, token_hash, created_by_user_id, device_id, tenant_id)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		nanoid.New(), deviceType, repository.HashToken(token), ownerUserID, deviceID, tenantID,
	)
	return deviceID, token
}

func (e *rlsEnv) seedPaidOrderWithLine(t *testing.T, tenantID string, qty int, unitPrice int64) (orderID, lineID, productID string) {
	t.Helper()
	catID := nanoid.New()
	e.privExec(t, `INSERT INTO category (id, name, tenant_id) VALUES ($1, 'C', $2)`, catID, tenantID)
	productID = nanoid.New()
	e.privExec(t,
		`INSERT INTO product (id, category_id, name, price_cents, tenant_id) VALUES ($1, $2, 'P', $3, $4)`,
		productID, catID, unitPrice, tenantID,
	)
	orderID = nanoid.New()
	e.privExec(t,
		`INSERT INTO "order" (id, total_cents, status, origin, tenant_id) VALUES ($1, $2, 'paid', 'pos', $3)`,
		orderID, unitPrice*int64(qty), tenantID,
	)
	lineID = nanoid.New()
	e.privExec(t,
		`INSERT INTO order_line (id, order_id, line_type, product_id, title, quantity, unit_price_cents, tenant_id)
		 VALUES ($1, $2, 'simple', $3, 'P', $4, $5, $6)`,
		lineID, orderID, productID, qty, unitPrice, tenantID,
	)
	return orderID, lineID, productID
}

func (e *rlsEnv) refundActor(t *testing.T, orderID string) (actorType, actorID string, deviceID *string) {
	t.Helper()
	require.NoError(t, e.priv.QueryRow(
		`SELECT actor_type, actor_id, device_id FROM refund WHERE order_id = $1 ORDER BY created_at DESC LIMIT 1`,
		orderID,
	).Scan(&actorType, &actorID, &deviceID))
	return actorType, actorID, deviceID
}

func lineRefundBody(lineID string, qty int) map[string]any {
	return map[string]any{"lines": []map[string]any{{"orderLineId": lineID, "quantity": qty}}}
}

// ── tests ──────────────────────────────────────────────────────────────────

func TestRefundHTTP(t *testing.T) {
	env := newRLSEnv(t)
	h := newRefundHTTPHarness(t, env)

	tenantA := env.seedTenant(t, "rfh-a-"+randSuffix())
	orgA := env.seedOrgForTenant(t, tenantA, "rfh-a-"+randSuffix())
	owner := env.seedUser(t)
	staff := env.seedUser(t)
	appAdmin := env.seedUser(t)
	env.seedMember(t, orgA, staff, "staff")
	env.seedMember(t, orgA, appAdmin, "app_admin")

	_, posToken := env.seedDeviceToken(t, tenantA, owner, "POS")
	_, stationToken := env.seedDeviceToken(t, tenantA, owner, "STATION")

	t.Run("admin staff can line-refund (ops tier)", func(t *testing.T) {
		orderID, lineID, _ := env.seedPaidOrderWithLine(t, tenantA, 2, 500)
		rec := h.do(t, http.MethodPost, "/v1/orders/"+orderID+"/refunds", tenantA, asUser(staff), lineRefundBody(lineID, 1))
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

		var ref generated.Refund
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ref))
		require.Equal(t, int64(500), ref.AmountCents)
		require.Equal(t, generated.RefundMethod("cash"), ref.Method)

		at, aid, did := env.refundActor(t, orderID)
		require.Equal(t, "admin", at)
		require.Equal(t, staff, aid)
		require.Nil(t, did, "admin refund records no device")
	})

	t.Run("admin staff CANNOT arbitrary-refund (app-admin tier)", func(t *testing.T) {
		orderID, _, _ := env.seedPaidOrderWithLine(t, tenantA, 1, 500)
		rec := h.do(t, http.MethodPost, "/v1/orders/"+orderID+"/refunds/arbitrary", tenantA, asUser(staff),
			map[string]any{"amountCents": 100, "reason": "goodwill"})
		require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())

		refs := env.countRefunds(t, orderID)
		require.Equal(t, 0, refs, "a forbidden arbitrary refund must persist nothing")
	})

	t.Run("admin app-admin CAN arbitrary-refund", func(t *testing.T) {
		orderID, _, _ := env.seedPaidOrderWithLine(t, tenantA, 1, 500)
		rec := h.do(t, http.MethodPost, "/v1/orders/"+orderID+"/refunds/arbitrary", tenantA, asUser(appAdmin),
			map[string]any{"amountCents": 100, "reason": "goodwill"})
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

		var ref generated.Refund
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ref))
		require.Equal(t, generated.RefundKind("arbitrary"), ref.Kind)
		require.Equal(t, int64(100), ref.AmountCents)

		at, aid, _ := env.refundActor(t, orderID)
		require.Equal(t, "admin", at)
		require.Equal(t, appAdmin, aid)
	})

	t.Run("admin arbitrary requires a reason", func(t *testing.T) {
		orderID, _, _ := env.seedPaidOrderWithLine(t, tenantA, 1, 500)
		rec := h.do(t, http.MethodPost, "/v1/orders/"+orderID+"/refunds/arbitrary", tenantA, asUser(appAdmin),
			map[string]any{"amountCents": 100, "reason": "  "})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	})

	t.Run("POS operator can line-refund (operator actor)", func(t *testing.T) {
		orderID, lineID, _ := env.seedPaidOrderWithLine(t, tenantA, 2, 500)
		posDeviceID, posTok := env.seedDeviceToken(t, tenantA, owner, "POS")
		rec := h.do(t, http.MethodPost, "/v1/pos/orders/"+orderID+"/refunds", tenantA, asBearer(posTok), lineRefundBody(lineID, 1))
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

		at, _, did := env.refundActor(t, orderID)
		require.Equal(t, "operator", at)
		require.NotNil(t, did)
		require.Equal(t, posDeviceID, *did)
	})

	t.Run("POS operator can cancel order", func(t *testing.T) {
		orderID, _, _ := env.seedPaidOrderWithLine(t, tenantA, 2, 500)
		rec := h.do(t, http.MethodPost, "/v1/pos/orders/"+orderID+"/cancel", tenantA, asBearer(posToken), map[string]any{"reason": "spill"})
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
		require.Equal(t, "cancelled", env.orderStatus(t, orderID))
	})

	t.Run("POS has no arbitrary route (404)", func(t *testing.T) {
		orderID, _, _ := env.seedPaidOrderWithLine(t, tenantA, 1, 500)
		rec := h.do(t, http.MethodPost, "/v1/pos/orders/"+orderID+"/refunds/arbitrary", tenantA, asBearer(posToken),
			map[string]any{"amountCents": 100, "reason": "x"})
		require.Equal(t, http.StatusNotFound, rec.Code, "POS operators must have no arbitrary-refund route")
	})

	t.Run("station device is excluded from POS refunds (403)", func(t *testing.T) {
		orderID, lineID, _ := env.seedPaidOrderWithLine(t, tenantA, 1, 500)
		rec := h.do(t, http.MethodPost, "/v1/pos/orders/"+orderID+"/refunds", tenantA, asBearer(stationToken), lineRefundBody(lineID, 1))
		require.Equal(t, http.StatusForbidden, rec.Code, "a station token must not reach POS refunds")
		require.Equal(t, 0, env.countRefunds(t, orderID))
	})

	t.Run("admin can view refund accounting and look up orders", func(t *testing.T) {
		orderID, lineID, productID := env.seedPaidOrderWithLine(t, tenantA, 3, 500)

		view := h.do(t, http.MethodGet, "/v1/orders/"+orderID+"/refunds", tenantA, asUser(staff), nil)
		require.Equal(t, http.StatusOK, view.Code, view.Body.String())
		var rv generated.RefundView
		require.NoError(t, json.Unmarshal(view.Body.Bytes(), &rv))
		require.Equal(t, int64(1500), rv.RefundableBalanceCents)
		require.Len(t, rv.Lines, 1)
		require.Equal(t, lineID, rv.Lines[0].OrderLineId)
		require.Equal(t, productID, rv.Lines[0].ProductId)
		require.Equal(t, 3, rv.Lines[0].RefundableQuantity)

		lookup := h.do(t, http.MethodGet, "/v1/orders/lookup?q="+orderID[:6], tenantA, asUser(staff), nil)
		require.Equal(t, http.StatusOK, lookup.Code, lookup.Body.String())
		var lr generated.OrderLookupResult
		require.NoError(t, json.Unmarshal(lookup.Body.Bytes(), &lr))
		found := false
		for _, o := range lr.Items {
			if o.Id == orderID {
				found = true
			}
		}
		require.True(t, found, "id-prefix lookup must return the order")
	})

	t.Run("cross-tenant order is invisible via the API (404)", func(t *testing.T) {
		tenantB := env.seedTenant(t, "rfh-b-"+randSuffix())
		bOrderID, _, _ := env.seedPaidOrderWithLine(t, tenantB, 1, 500)

		// staff (a member of tenant A) requests tenant B's order while bound to tenant A.
		rec := h.do(t, http.MethodGet, "/v1/orders/"+bOrderID+"/refunds", tenantA, asUser(staff), nil)
		require.Equal(t, http.StatusNotFound, rec.Code, "tenant A must not read tenant B's order")
	})
}

func (e *rlsEnv) countRefunds(t *testing.T, orderID string) int {
	t.Helper()
	var n int
	require.NoError(t, e.priv.QueryRow(`SELECT count(*) FROM refund WHERE order_id = $1`, orderID).Scan(&n))
	return n
}
