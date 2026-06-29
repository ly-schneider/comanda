package app

import (
	"backend/internal/inventory"
	"backend/internal/payment"
	"backend/internal/repository"
	"backend/internal/service"

	"go.uber.org/fx"
)

// refundProviderResolver adapts the concrete payment factory to the interface
// the refund service depends on, so tests can substitute a stub provider.
func refundProviderResolver(f *payment.Factory) service.RefundProviderResolver {
	return f
}

func NewServices() fx.Option {
	return fx.Options(
		fx.Provide(
			service.NewPaymentService,
			service.NewQRKeyService,
			service.NewSettingsService,
			service.NewProductService,
			service.NewCategoryService,
			service.NewOrderService,
			service.NewStationService,
			service.NewPOSService,
			refundProviderResolver,
			service.NewRefundService,
			repository.NewIdempotencyRepository,
			service.NewEmailService,
			service.NewAdminInviteService,
			service.NewOwnerInviteService,
			service.NewUserService,
			service.NewDeviceService,
			inventory.NewHub,
			service.NewElvantoService,
			service.NewClub100Service,
			service.NewVolunteerService,
			service.NewAndroidUpdateService,
			service.NewDomainService,
			service.NewTenantService,
			service.NewAccountService,
		),
	)
}
