package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/api"
	"backend/internal/database"
	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	entOrder "backend/internal/generated/ent/order"
	entOrderPayment "backend/internal/generated/ent/orderpayment"
	"backend/internal/plugin"
	"backend/internal/repository"
	"backend/internal/service"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type hookState struct {
	validateErr error
	recordedErr error
	validateN   int
	recordedN   int
	lastMethod  string
}

func (s *hookState) reset() { *s = hookState{} }

type stubPaymentPlugin struct {
	key string
	st  *hookState
}

func (p stubPaymentPlugin) Key() string       { return p.key }
func (p stubPaymentPlugin) Name() string      { return p.key }
func (p stubPaymentPlugin) Module() fx.Option { return fx.Options() }
func (p stubPaymentPlugin) Routes(_ *generated.ServerInterfaceWrapper, _ plugin.Middlewares) func(chi.Router) {
	return func(chi.Router) {}
}

func (p stubPaymentPlugin) ValidatePayment(_ context.Context, _ *ent.Order, _ json.RawMessage) error {
	p.st.validateN++
	return p.st.validateErr
}

func (p stubPaymentPlugin) OnPaymentRecorded(_ context.Context, _ *ent.Order, method string, _ json.RawMessage) error {
	p.st.recordedN++
	p.st.lastMethod = method
	return p.st.recordedErr
}

type paymentPluginHarness struct {
	tdb    *TestDB
	repos  *Repositories
	router http.Handler
}

func newPaymentPluginHarness(t *testing.T, plugins []plugin.Plugin) *paymentPluginHarness {
	t.Helper()
	tdb := NewTestDB(t)
	repos := NewRepositories(tdb.Client)
	cfg := TestConfig()

	paymentSvc := NewPaymentSvc(cfg, repos, NewProductSvc(repos))
	posSvc := service.NewPOSService(cfg, repos.Device, repos.Order, paymentSvc)
	orderSvc := service.NewOrderService(repos.Order, repos.OrderLine, repos.Inventory, nil)
	pluginRepo := repository.NewTenantPluginRepository(tdb.Client)
	pluginSvc := plugin.NewService(plugins, pluginRepo, testQREncryptor(), zap.NewNop())

	handlers := api.NewHandlers(api.HandlersDeps{
		Config:      cfg,
		Orders:      orderSvc,
		Payments:    paymentSvc,
		POS:         posSvc,
		Plugins:     pluginSvc,
		Idempotency: repos.Idempotency,
		Privileged:  &database.PrivilegedClient{Client: tdb.Client, DB: tdb.DB},
		Logger:      zap.NewNop(),
	})
	wrapper := generated.ServerInterfaceWrapper{
		Handler: handlers,
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		},
	}

	h := &paymentPluginHarness{tdb: tdb, repos: repos}
	r := chi.NewRouter()
	r.Route("/v1", func(v1 chi.Router) {
		v1.Use(h.tx)
		v1.Post("/orders/{orderId}/payment", wrapper.CreateOrderPayment)
	})
	h.router = r
	return h
}

func (h *paymentPluginHarness) tx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		conn, err := h.tdb.DB.Conn(ctx)
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

		txClient := ent.NewClient(ent.Driver(database.NewTxEntDriver(tx)))
		txClient.Use(defaultTenantHook)
		ctx = repository.ContextWithClient(ctx, txClient)
		ctx = repository.ContextWithRequestTx(ctx, tx)
		ctx = repository.ContextWithTenantID(ctx, DefaultTestTenantID)

		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r.WithContext(ctx))

		if status := ww.Status(); status == 0 || status < http.StatusBadRequest {
			_ = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
	})
}

func (h *paymentPluginHarness) pay(t *testing.T, orderID string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/v1/orders/"+orderID+"/payment", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	return rec
}

func (h *paymentPluginHarness) payments(t *testing.T, orderID string) []*ent.OrderPayment {
	t.Helper()
	rows, err := h.tdb.Client.OrderPayment.Query().
		Where(entOrderPayment.OrderIDEQ(orderID)).
		All(context.Background())
	require.NoError(t, err)
	return rows
}

func (h *paymentPluginHarness) orderStatus(t *testing.T, orderID string) entOrder.Status {
	t.Helper()
	o, err := h.repos.Order.GetByID(context.Background(), orderID)
	require.NoError(t, err)
	return o.Status
}

func (h *paymentPluginHarness) newPendingOrder(t *testing.T) string {
	t.Helper()
	return NewFixtures(h.repos).CreateOrder(1000, entOrder.StatusPending, entOrder.OriginPos).ID
}

func TestPaymentPlugin_Dispatch(t *testing.T) {
	st := &hookState{}
	h := newPaymentPluginHarness(t, []plugin.Plugin{stubPaymentPlugin{key: "testpay", st: st}})
	defer h.tdb.Close()
	h.tdb.Cleanup(t)

	enabled := true
	_, err := plugin.NewService(
		[]plugin.Plugin{stubPaymentPlugin{key: "testpay", st: st}},
		repository.NewTenantPluginRepository(h.tdb.Client),
		testQREncryptor(),
		zap.NewNop(),
	).SetForTenant(context.Background(), DefaultTestTenantID, "testpay", plugin.UpdateInput{Enabled: &enabled})
	require.NoError(t, err)

	pluginBody := map[string]any{"key": "testpay", "data": map[string]any{"personId": "p1"}}

	t.Run("gratis_plugin full redemption", func(t *testing.T) {
		st.reset()
		orderID := h.newPendingOrder(t)

		rec := h.pay(t, orderID, map[string]any{"method": "gratis_plugin", "plugin": pluginBody})
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, "gratis_plugin", resp["method"])
		require.Equal(t, "testpay", resp["pluginKey"])

		require.Equal(t, entOrder.StatusPaid, h.orderStatus(t, orderID))
		rows := h.payments(t, orderID)
		require.Len(t, rows, 1)
		require.Equal(t, entOrderPayment.MethodGRATIS_PLUGIN, rows[0].Method)
		require.NotNil(t, rows[0].PluginKey)
		require.Equal(t, "testpay", *rows[0].PluginKey)
		require.Equal(t, 1, st.validateN)
		require.Equal(t, 1, st.recordedN)
		require.Equal(t, "gratis_plugin", st.lastMethod)
	})

	t.Run("mixed cart: cash carries plugin data", func(t *testing.T) {
		st.reset()
		orderID := h.newPendingOrder(t)

		rec := h.pay(t, orderID, map[string]any{"method": "cash", "plugin": pluginBody})
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

		require.Equal(t, entOrder.StatusPaid, h.orderStatus(t, orderID))
		rows := h.payments(t, orderID)
		require.Len(t, rows, 1)
		require.Equal(t, entOrderPayment.MethodCASH, rows[0].Method)
		require.NotNil(t, rows[0].PluginKey)
		require.Equal(t, "testpay", *rows[0].PluginKey)
		require.Equal(t, 1, st.validateN)
		require.Equal(t, 1, st.recordedN)
		require.Equal(t, "cash", st.lastMethod)
	})

	t.Run("ValidatePayment failure aborts before recording", func(t *testing.T) {
		st.reset()
		st.validateErr = errors.New("no redemptions left")
		orderID := h.newPendingOrder(t)

		rec := h.pay(t, orderID, map[string]any{"method": "gratis_plugin", "plugin": pluginBody})
		require.Equal(t, http.StatusBadRequest, rec.Code)

		require.Equal(t, entOrder.StatusPending, h.orderStatus(t, orderID), "order must stay pending")
		require.Empty(t, h.payments(t, orderID), "no payment row on validation failure")
		require.Equal(t, 1, st.validateN)
		require.Equal(t, 0, st.recordedN, "OnPaymentRecorded must not fire")
	})

	t.Run("OnPaymentRecorded failure rolls the tx back atomically", func(t *testing.T) {
		st.reset()
		st.recordedErr = errors.New("elvanto write failed")
		orderID := h.newPendingOrder(t)

		rec := h.pay(t, orderID, map[string]any{"method": "gratis_plugin", "plugin": pluginBody})
		require.Equal(t, http.StatusBadRequest, rec.Code)

		require.Equal(t, entOrder.StatusPending, h.orderStatus(t, orderID), "status=paid must be rolled back")
		require.Empty(t, h.payments(t, orderID), "the recorded payment row must be rolled back")
		require.Equal(t, 1, st.validateN)
		require.Equal(t, 1, st.recordedN)
	})

	t.Run("unregistered plugin key is rejected", func(t *testing.T) {
		st.reset()
		orderID := h.newPendingOrder(t)

		rec := h.pay(t, orderID, map[string]any{
			"method": "gratis_plugin",
			"plugin": map[string]any{"key": "ghost", "data": map[string]any{}},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code)

		require.Equal(t, entOrder.StatusPending, h.orderStatus(t, orderID))
		require.Empty(t, h.payments(t, orderID))
		require.Equal(t, 0, st.validateN)
	})

	t.Run("gratis_plugin without a plugin field is rejected", func(t *testing.T) {
		st.reset()
		orderID := h.newPendingOrder(t)

		rec := h.pay(t, orderID, map[string]any{"method": "gratis_plugin"})
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Equal(t, entOrder.StatusPending, h.orderStatus(t, orderID))
		require.Empty(t, h.payments(t, orderID))
	})
}
