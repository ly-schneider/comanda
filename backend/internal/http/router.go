package http

import (
	"net/http"

	"backend/internal/api"
	"backend/internal/auth"
	"backend/internal/config"
	"backend/internal/generated/api/generated"
	"backend/internal/middleware"
	"backend/internal/plugin"

	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/go-chi/chi/v5"
	chiMw "github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

func NewRouter(
	cfg config.Config,
	apiHandlers *api.Handlers,
	betterAuthMw *auth.SessionMiddleware,
	deviceAuthMw *auth.DeviceAuthMiddleware,
	securityMw *middleware.SecurityMiddleware,
	systemMw *middleware.SystemDisableMiddleware,
	tenantMw *middleware.TenantMiddleware,
	plugins []plugin.Plugin,
	pluginSvc plugin.Service,
	logger *zap.Logger,
	isDev bool,
) http.Handler {
	// ── Create the generated wrapper ──────────────────────────────────
	wrapper := generated.ServerInterfaceWrapper{
		Handler: apiHandlers,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		},
	}

	// ── SSE router (minimal middleware to preserve flushing) ──────────
	// The stream's only DB access is the initial snapshot; it then loops on an
	// in-memory hub. The tenant tx is held for the stream lifetime — acceptable
	// for the low number of admin dashboards in P1.
	sseRouter := chi.NewRouter()
	sseRouter.Use(securityMw.CORS)
	// Resolve + bind the tenant tx before the maintenance gate, mirroring the
	// main /v1 stack: RequireEnabled reads the tenant's system setting and its
	// admin bypass (IsAdmin → HasTenantTier) both need the bound tenant tx.
	sseRouter.Use(tenantMw.Resolve)
	sseRouter.Use(tenantMw.Tx)
	sseRouter.Use(systemMw.RequireEnabled)
	sseRouter.Get("/v1/inventory/stream", wrapper.StreamInventory)

	// ── Main router with full middleware stack ────────────────────────
	r := chi.NewRouter()
	sentryHandler := sentryhttp.New(sentryhttp.Options{Repanic: true})
	r.Use(sentryHandler.Handle)
	r.Use(middleware.TraceRoute)
	r.Use(middleware.HTTPMetrics)
	r.Use(securityMw.SecurityHeaders)
	r.Use(securityMw.CacheControlForSensitive)
	r.Use(securityMw.CORS)
	r.Use(chiMw.Logger)
	r.Use(chiMw.Recoverer)
	r.Use(chiMw.RequestID)
	r.Use(middleware.RealIP)
	r.Use(chiMw.Heartbeat("/ping"))
	r.Use(middleware.LogHTTPErrors)

	// ── Health check ──────────────────────────────────────────────────
	r.Get("/health", apiHandlers.HealthCheck)

	// ── Android update check + APK download (public, no auth) ───────
	r.Get("/v1/android/latest", wrapper.GetAndroidLatestVersion)
	r.Get("/v1/android/download", wrapper.DownloadAndroidApk)

	// ── Payrexx webhook (per-tenant, no auth, no Host) ──────────────
	// Wired outside the /v1 tenant-tx group: with no tenant-bearing Host it can't
	// use the Host resolver, so the handler resolves {tenantToken} and opens its
	// own RLS-bound tenant tx.
	r.Post("/v1/payments/webhooks/payrexx/{tenantToken}", wrapper.HandlePayrexxWebhook)

	// ── Trusted-origins feed (cross-tenant, internal, no Host, no auth UI) ──
	// The web tier consumes this to seed Better Auth's trustedOrigins with every
	// verified custom domain. It reads cross-tenant on the privileged client, so
	// it sits outside the /v1 tenant-tx group and is gated by a shared internal
	// token. Only registered when that token is configured (fail closed).
	if cfg.CustomDomain.InternalAPIToken != "" {
		r.Get("/internal/trusted-origins", wrapper.HandleTrustedOrigins)
	}

	// ── API documentation (Scalar) ───────────────────────────────────
	r.Get("/docs", DocsScalarHandler())
	r.Get("/docs/openapi.json", DocsOpenAPIHandler())

	r.Route("/v1", func(v1 chi.Router) {

		// Tenant resolution + per-request tenant transaction. Every /v1
		// request runs inside a single Postgres tx bound to its tenant via
		// set_config('app.tenant_id', …), the anchor for RLS. In P1 the tenant
		// is resolved from the X-Tenant-Id header or BOOTSTRAP_TENANT_SLUG;
		// host-based resolution replaces the resolver in P2.
		v1.Use(tenantMw.Resolve)
		v1.Use(tenantMw.Tx)

		// ── System status (always accessible, no auth) ───────────
		v1.Get("/system/status", wrapper.GetSystemStatus)

		// ── Exempt routes (no auth, always accessible) ───────────
		v1.Post("/devices/pairings", wrapper.CreateDevicePairing)
		v1.Get("/devices/pairings/{code}", wrapper.GetDevicePairing)
		v1.Post("/invites/verify", wrapper.VerifyInvite)
		v1.Post("/invites/accept", wrapper.AcceptInvite)
		v1.Post("/owner-invites/verify", wrapper.VerifyOwnerInvite)
		v1.Get("/payments/{paymentId}", wrapper.GetPayment)
		v1.Post("/auth/otp-email", wrapper.SendOtpEmail)

		// Public branding (no auth). Exempt from RequireEnabled so the
		// storefront can render branding even when the tenant is disabled.
		v1.Get("/branding", wrapper.GetBranding)

		v1.Get("/plugins", wrapper.ListPlugins)

		// Public QR signing config (no auth). Exempt from RequireEnabled so a
		// station/PWA can fetch the verification key at setup time.
		v1.Get("/qr-config", wrapper.GetQRConfig)

		// ── Public routes (no auth, blocked when system disabled) ─
		v1.Group(func(pub chi.Router) {
			pub.Use(systemMw.RequireEnabled)
			pub.Get("/products", wrapper.ListProducts)
			pub.Get("/products/{productId}", wrapper.GetProduct)
			pub.Get("/orders/{orderId}", wrapper.GetOrder)
			pub.Get("/bundles", wrapper.ListBundles)
			pub.Get("/bundles/{bundleId}", wrapper.GetBundle)
			pub.Get("/categories", wrapper.ListCategories)
			pub.Get("/categories/{categoryId}", wrapper.GetCategory)

			pub.Post("/claim/{token}/auth", wrapper.VerifyClaimAccess)
			pub.Get("/claim/{token}", wrapper.GetClaimCampaign)
		})

		// ── Device-authenticated routes (blocked when disabled) ──
		v1.Group(func(station chi.Router) {
			station.Use(systemMw.RequireEnabled)
			station.Use(deviceAuthMw.RequireDevice(auth.DeviceTypeStation))
			station.Get("/stations/me", wrapper.GetCurrentStation)
			station.Post("/stations/redeem", wrapper.RedeemAtStation)
			station.Post("/stations/redeem-campaign", wrapper.RedeemCampaignAtStation)
		})

		v1.Group(func(pos chi.Router) {
			pos.Use(systemMw.RequireEnabled)
			pos.Use(deviceAuthMw.RequireDevice(auth.DeviceTypePOS))
			pos.Get("/pos/me", wrapper.GetCurrentPos)
			pos.Patch("/pos/products/{productId}/inventory", wrapper.AdjustProductInventory)
			pos.Patch("/pos/products/{productId}/active", wrapper.SetProductActive)

			// Refunds (operator surface): lookup, view, line refund, deposit
			// return, cancel. Arbitrary refunds are admin-only and have no POS route.
			pos.Get("/pos/orders/lookup", wrapper.LookupPosOrders)
			pos.Get("/pos/orders/{orderId}/refunds", wrapper.GetPosRefundView)
			pos.Post("/pos/orders/{orderId}/refunds", wrapper.CreatePosLineRefund)
			pos.Post("/pos/orders/{orderId}/deposit-returns", wrapper.CreatePosDepositReturn)
			pos.Post("/pos/orders/{orderId}/cancel", wrapper.CancelPosOrder)
		})

		// ── Orders (anonymous allowed, blocked when disabled) ────
		v1.Group(func(orders chi.Router) {
			orders.Use(systemMw.RequireEnabled)
			orders.Use(betterAuthMw.OptionalAuth())
			orders.Post("/orders", wrapper.CreateOrder)
			orders.Get("/orders/{orderId}/payment", wrapper.GetOrderPayment)
			orders.Post("/orders/{orderId}/payment", wrapper.CreateOrderPayment)
		})

		// ── User-authenticated routes (blocked when disabled) ────
		v1.Group(func(authed chi.Router) {
			authed.Use(systemMw.RequireEnabled)
			authed.Use(betterAuthMw.RequireAuth())
			authed.Get("/orders", wrapper.ListOrders)
		})

		// ── Catalog routes (auth + catalog membership + CSRF) ──
		// The lowest tenant tier: products (incl. images and inventory),
		// categories, and bundles. Staff and tenant-admins satisfy it too.
		v1.Group(func(catalog chi.Router) {
			catalog.Use(betterAuthMw.RequireAuth())
			catalog.Use(betterAuthMw.RequireTenantRole(auth.TierCatalog))
			csrf := middleware.NewCSRFMiddleware()
			catalog.Use(csrf.Require)

			catalog.Post("/products", wrapper.CreateProduct)
			catalog.Patch("/products/{productId}", wrapper.UpdateProduct)
			catalog.Delete("/products/{productId}", wrapper.DeleteProduct)
			catalog.Post("/products/{productId}/image", wrapper.UploadProductImage)
			catalog.Delete("/products/{productId}/image", wrapper.DeleteProductImage)
			catalog.Get("/products/{productId}/inventory", wrapper.GetProductInventory)
			catalog.Get("/products/{productId}/inventory/history", wrapper.GetProductInventoryHistory)
			catalog.Patch("/products/{productId}/inventory", wrapper.AdjustProductInventory)

			catalog.Post("/categories", wrapper.CreateCategory)
			catalog.Patch("/categories/{categoryId}", wrapper.UpdateCategory)
			catalog.Delete("/categories/{categoryId}", wrapper.DeleteCategory)

			catalog.Post("/bundles", wrapper.CreateBundle)
			catalog.Patch("/bundles/{bundleId}", wrapper.UpdateBundle)
			catalog.Delete("/bundles/{bundleId}", wrapper.DeleteBundle)
			catalog.Post("/bundles/{bundleId}/slots", wrapper.CreateBundleSlot)
			catalog.Patch("/bundles/{bundleId}/slots/reorder", wrapper.ReorderBundleSlots)
			catalog.Patch("/bundles/{bundleId}/slots/{slotId}", wrapper.UpdateBundleSlot)
			catalog.Delete("/bundles/{bundleId}/slots/{slotId}", wrapper.DeleteBundleSlot)
			catalog.Post("/bundles/{bundleId}/slots/{slotId}/options", wrapper.AddSlotOption)
			catalog.Delete("/bundles/{bundleId}/slots/{slotId}/options/{optionProductId}", wrapper.RemoveSlotOption)
		})

		// ── Staff routes (auth + staff membership + CSRF, NOT blocked) ─
		// The subdomain /admin surface is gated by the tenant-scoped org member
		// role, not the global user.role. Staff is the day-to-day operational tier
		// (orders, stations, POS, jetons, devices, staff-perks), strictly above
		// catalog.
		v1.Group(func(ops chi.Router) {
			ops.Use(betterAuthMw.RequireAuth())
			ops.Use(betterAuthMw.RequireTenantRole(auth.TierOps))
			csrf := middleware.NewCSRFMiddleware()
			ops.Use(csrf.Require)

			ops.Patch("/orders/{orderId}", wrapper.UpdateOrderStatus)

			// Refunds (admin surface): audit list, lookup, view, line refund,
			// deposit return, cancel — the same operations POS operators get, on the
			// staff/ops tier. Arbitrary refunds are gated higher (see the app-admin group).
			ops.Get("/refunds", wrapper.ListRefunds)
			ops.Get("/orders/lookup", wrapper.LookupOrders)
			ops.Get("/orders/{orderId}/refunds", wrapper.GetRefundView)
			ops.Post("/orders/{orderId}/refunds", wrapper.CreateLineRefund)
			ops.Post("/orders/{orderId}/deposit-returns", wrapper.CreateDepositReturn)
			ops.Post("/orders/{orderId}/cancel", wrapper.CancelOrder)

			ops.Get("/stations", wrapper.ListStations)
			ops.Get("/stations/{stationId}", wrapper.GetStation)
			ops.Patch("/stations/{stationId}", wrapper.RenameStation)
			ops.Get("/stations/{stationId}/products", wrapper.ListStationProducts)
			ops.Put("/stations/{stationId}/products", wrapper.SetStationProducts)
			ops.Post("/stations/{stationId}/products/{productId}", wrapper.AddStationProduct)
			ops.Delete("/stations/{stationId}/products/{productId}", wrapper.RemoveStationProduct)
			ops.Delete("/stations/{stationId}", wrapper.RevokeStation)

			ops.Get("/pos/devices", wrapper.ListPosDevices)

			ops.Get("/jetons", wrapper.ListJetons)
			ops.Post("/jetons", wrapper.CreateJeton)
			ops.Patch("/jetons/{jetonId}", wrapper.UpdateJeton)
			ops.Delete("/jetons/{jetonId}", wrapper.DeleteJeton)

			ops.Get("/devices", wrapper.ListDevices)
			ops.Get("/devices/{deviceId}", wrapper.GetDevice)
			ops.Delete("/devices/{deviceId}", wrapper.RevokeDevice)
			ops.Post("/devices/pairings/{code}", wrapper.CompleteDevicePairing)

			ops.Get("/events", wrapper.ListEvents)

			ops.Post("/staff-perks", wrapper.CreateVolunteerCampaign)
			ops.Get("/staff-perks", wrapper.ListVolunteerCampaigns)
			ops.Get("/staff-perks/{campaignId}", wrapper.GetVolunteerCampaign)
			ops.Patch("/staff-perks/{campaignId}", wrapper.UpdateVolunteerCampaign)
			ops.Post("/staff-perks/{campaignId}/end", wrapper.EndVolunteerCampaign)
			ops.Post("/staff-perks/{campaignId}/rotate-token", wrapper.RotateVolunteerCampaignToken)
			ops.Get("/staff-perks/{campaignId}/print.pdf", wrapper.PrintStaffPerkSlips)
		})

		// ── App-admin routes (auth + app-admin membership + CSRF) ──────
		// Shop settings, users, and app-role invites. Branding, domains and
		// payments live on the account hub; UpdateSettings rejects branding below
		// tenant-admin.
		v1.Group(func(appadmin chi.Router) {
			appadmin.Use(betterAuthMw.RequireAuth())
			appadmin.Use(betterAuthMw.RequireTenantRole(auth.TierAppAdmin))
			csrf := middleware.NewCSRFMiddleware()
			appadmin.Use(csrf.Require)

			// Arbitrary (discretionary) refund: a strictly higher gate than line
			// refunds/cancel (which are ops-tier). App-admin only; no POS route.
			appadmin.Post("/orders/{orderId}/refunds/arbitrary", wrapper.CreateArbitraryRefund)

			appadmin.Get("/settings", wrapper.GetSettings)
			appadmin.Patch("/settings", wrapper.UpdateSettings)

			appadmin.Get("/users", wrapper.ListUsers)
			appadmin.Get("/users/{userId}", wrapper.GetUser)
			appadmin.Patch("/users/{userId}", wrapper.UpdateUser)
			appadmin.Delete("/users/{userId}", wrapper.DeleteUser)

			appadmin.Get("/invites", wrapper.ListInvites)
			appadmin.Post("/invites", wrapper.CreateInvite)
			appadmin.Get("/invites/{inviteId}", wrapper.GetInvite)
			appadmin.Delete("/invites/{inviteId}", wrapper.DeleteInvite)
		})

		// ── Platform-admin routes (cross-tenant; platform host only) ──
		// Served on the no-tenant platform context. RequireAuth flips the
		// app.is_platform_admin GUC on the request tx (RLS uses it to bypass
		// tenant scoping); RequirePlatformAdmin 403s non-admins; CSRF guards the
		// mutations. The handlers also assert the platform context so a platform
		// admin on a tenant host can't drive these against a single tenant.
		v1.Group(func(pa chi.Router) {
			pa.Use(betterAuthMw.RequireAuth())
			pa.Use(betterAuthMw.RequirePlatformAdmin())
			csrf := middleware.NewCSRFMiddleware()
			pa.Use(csrf.Require)

			pa.Get("/platform/tenants", wrapper.ListTenants)
			pa.Post("/platform/tenants", wrapper.CreateTenant)
			pa.Get("/platform/tenants/{tenantId}", wrapper.GetTenant)
			pa.Patch("/platform/tenants/{tenantId}", wrapper.UpdateTenant)
			pa.Post("/platform/tenants/{tenantId}/owner-invite", wrapper.CreateOwnerInvite)

			pa.Get("/platform/tenants/{tenantId}/plugins", wrapper.ListTenantPlugins)
			pa.Put("/platform/tenants/{tenantId}/plugins/{key}", wrapper.UpdateTenantPlugin)

			pa.Get("/platform/qa/runs", wrapper.ListQaRuns)
			pa.Post("/platform/qa/runs", wrapper.CreateQaRun)
			pa.Get("/platform/qa/runs/{runId}", wrapper.GetQaRun)
			pa.Patch("/platform/qa/runs/{runId}", wrapper.UpdateQaRun)
			pa.Delete("/platform/qa/runs/{runId}", wrapper.DeleteQaRun)
			pa.Put("/platform/qa/runs/{runId}/results/{testKey}", wrapper.UpsertQaResult)
		})

		// ── Account hub: tenant listing (platform host; pre-selection) ─
		// Runs before a tenant is bound, so RequireAuth only — no ResolveVendorTenant.
		v1.Group(func(acctList chi.Router) {
			acctList.Use(betterAuthMw.RequireAuth())
			acctList.Get("/account/tenants", wrapper.ListAccountTenants)
		})

		// ── Account routes (vendor self-service; platform host only) ──
		// ResolveVendorTenant binds + authorizes the {accountTenant} slug; the whole
		// group is the settings tier.
		v1.Route("/account/{accountTenant}", func(account chi.Router) {
			account.Use(betterAuthMw.RequireAuth())
			account.Use(tenantMw.ResolveVendorTenant)
			csrf := middleware.NewCSRFMiddleware()
			account.Use(csrf.Require)
			account.Use(betterAuthMw.RequireTenantRole(auth.TierTenantAdmin))

			account.Get("/tenant", wrapper.GetAccountTenant)
			account.Get("/branding", wrapper.GetBranding)
			account.Get("/onboarding", wrapper.GetOnboardingStatus)

			account.Get("/settings", wrapper.GetSettings)
			account.Patch("/settings", wrapper.UpdateSettings)
			account.Post("/settings/logo", wrapper.UploadBrandingLogo)
			account.Delete("/settings/logo", wrapper.DeleteBrandingLogo)
			account.Get("/settings/payrexx", wrapper.GetPayrexxStatus)
			account.Put("/settings/payrexx/api", wrapper.ConnectPayrexxApi)
			account.Put("/settings/payrexx/webhook", wrapper.SetPayrexxWebhookSecret)
			account.Post("/settings/payrexx/test", wrapper.TestPayrexxConnection)

			account.Get("/domains", wrapper.ListDomains)
			account.Post("/domains", wrapper.CreateDomain)
			account.Get("/domains/{id}", wrapper.GetDomain)
			account.Delete("/domains/{id}", wrapper.DeleteDomain)
			account.Post("/domains/{id}/verify", wrapper.VerifyDomain)
			account.Post("/domains/{id}/primary", wrapper.SetPrimaryDomain)

			account.Get("/invites", wrapper.ListInvites)
			account.Post("/invites", wrapper.CreateAccountInvite)
			account.Get("/invites/{inviteId}", wrapper.GetInvite)
			account.Delete("/invites/{inviteId}", wrapper.DeleteInvite)

			account.Post("/onboarding/complete", wrapper.CompleteOnboarding)
		})

		// ── Owner-invite acceptance (authed; not yet a member) ──
		// The accepting user has just registered via OTP and is not yet a member
		// of any organization, so this runs RequireAuth + CSRF only — no
		// platform-admin gate, no vendor-tenant resolution. The handler binds the
		// membership and the session's active organization on the privileged pool.
		v1.Group(func(onb chi.Router) {
			onb.Use(betterAuthMw.RequireAuth())
			csrf := middleware.NewCSRFMiddleware()
			onb.Use(csrf.Require)

			onb.Post("/owner-invites/accept", wrapper.AcceptOwnerInvite)
		})

		// ── Plugins (per-tenant, 404 when dormant) ───────────────────────
		pluginMw := plugin.Middlewares{
			RequireEnabled:    systemMw.RequireEnabled,
			RequireDevice:     deviceAuthMw.RequireDevice,
			RequireAuth:       betterAuthMw.RequireAuth,
			OptionalAuth:      betterAuthMw.OptionalAuth,
			RequireTenantRole: betterAuthMw.RequireTenantRole,
			CSRF:              middleware.NewCSRFMiddleware().Require,
		}
		plugin.Mount(v1, plugins, &wrapper, pluginMw, pluginSvc)
	})

	// Compose: SSE bypasses main middleware, everything else uses full stack
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/v1/inventory/stream" {
			sseRouter.ServeHTTP(w, req)
			return
		}
		r.ServeHTTP(w, req)
	})
}
