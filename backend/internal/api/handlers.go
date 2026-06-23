package api

import (
	"backend/internal/blobstore"
	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/inventory"
	"backend/internal/payment"
	"backend/internal/repository"
	"backend/internal/service"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Handlers implements generated.ServerInterface by injecting services directly.
type Handlers struct {
	generated.Unimplemented

	categories    service.CategoryService
	products      service.ProductService
	orders        service.OrderService
	payments      service.PaymentService
	qrKeys        service.QRKeyService
	pos           service.POSService
	settings      service.SettingsService
	stations      service.StationService
	invites       service.AdminInviteService
	ownerInvites  service.OwnerInviteService
	email         service.EmailService
	users         service.UserService
	devices       service.DeviceService
	club100       service.Club100Service
	volunteers    service.VolunteerService
	androidUpdate service.AndroidUpdateService
	domains       service.DomainService
	tenants       service.TenantService
	verification  repository.VerificationRepository
	idempotency   repository.IdempotencyRepository
	blobStore     *blobstore.Client
	inventoryHub  *inventory.Hub

	// privileged resolves {tenantToken} -> tenant before context exists; appDB
	// opens the RLS-bound tenant tx for the webhook path. The trusted-origins
	// feed also reads tenant_domain cross-tenant on the privileged client.
	privileged *ent.Client
	appDB      *database.AppClient
	providers  *payment.Factory

	config config.Config
	logger *zap.Logger
}

// Compile-time check that Handlers implements ServerInterface.
var _ generated.ServerInterface = (*Handlers)(nil)

type HandlersDeps struct {
	fx.In

	Config        config.Config
	Categories    service.CategoryService
	Products      service.ProductService
	Orders        service.OrderService
	Payments      service.PaymentService
	QRKeys        service.QRKeyService
	POS           service.POSService
	Settings      service.SettingsService
	Stations      service.StationService
	Invites       service.AdminInviteService
	OwnerInvites  service.OwnerInviteService
	Email         service.EmailService
	Users         service.UserService
	Devices       service.DeviceService
	Club100       service.Club100Service
	Volunteers    service.VolunteerService
	AndroidUpdate service.AndroidUpdateService
	Domains       service.DomainService
	Tenants       service.TenantService
	Verification  repository.VerificationRepository
	Idempotency   repository.IdempotencyRepository
	BlobStore     *blobstore.Client `optional:"true"`
	InventoryHub  *inventory.Hub
	Privileged    *database.PrivilegedClient
	AppDB         *database.AppClient
	Providers     *payment.Factory
	Logger        *zap.Logger
}

// NewHandlers creates a new Handlers with all required service dependencies.
func NewHandlers(deps HandlersDeps) *Handlers {
	return &Handlers{
		categories:    deps.Categories,
		products:      deps.Products,
		orders:        deps.Orders,
		payments:      deps.Payments,
		qrKeys:        deps.QRKeys,
		pos:           deps.POS,
		settings:      deps.Settings,
		stations:      deps.Stations,
		invites:       deps.Invites,
		ownerInvites:  deps.OwnerInvites,
		email:         deps.Email,
		users:         deps.Users,
		devices:       deps.Devices,
		club100:       deps.Club100,
		volunteers:    deps.Volunteers,
		androidUpdate: deps.AndroidUpdate,
		domains:       deps.Domains,
		tenants:       deps.Tenants,
		verification:  deps.Verification,
		idempotency:   deps.Idempotency,
		blobStore:     deps.BlobStore,
		inventoryHub:  deps.InventoryHub,
		privileged:    deps.Privileged.Client,
		appDB:         deps.AppDB,
		providers:     deps.Providers,
		config:        deps.Config,
		logger:        deps.Logger,
	}
}
