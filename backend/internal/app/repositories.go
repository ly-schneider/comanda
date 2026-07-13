package app

import (
	"context"

	"backend/internal/database"
	"backend/internal/generated/ent"
	"backend/internal/repository"

	"go.uber.org/fx"
)

func newCachedSessionRepository(client *ent.Client) repository.SessionRepository {
	return repository.NewCachedSessionRepository(repository.NewSessionRepository(client))
}

func newCachedDeviceBindingRepository(client *ent.Client, app *database.AppClient) repository.DeviceBindingRepository {
	runInTx := func(ctx context.Context, tenantID string, fn func(context.Context) error) error {
		return repository.RunInTenantTx(ctx, app.DB, tenantID, fn)
	}
	return repository.NewCachedDeviceBindingRepository(repository.NewDeviceBindingRepository(client), runInTx)
}

func NewRepositories() fx.Option {
	return fx.Options(
		fx.Provide(
			repository.NewCategoryRepository,
			repository.NewProductRepository,
			repository.NewProductVariantRepository,
			repository.NewJetonRepository,
			repository.NewBundleSlotRepository,
			repository.NewBundleSlotOptionRepository,
			repository.NewDeviceRepository,
			repository.NewDeviceProductRepository,
			repository.NewSettingsRepository,
			repository.NewOrderRepository,
			repository.NewOrderPaymentRepository,
			repository.NewOrderLineRepository,
			repository.NewOrderLineRedemptionRepository,
			repository.NewRefundRepository,
			repository.NewInventoryLedgerRepository,
			repository.NewAdminInviteRepository,
			repository.NewOwnerInviteRepository,
			repository.NewMemberRepository,
			repository.NewUserRepository,
			repository.NewVerificationRepository,
			newCachedSessionRepository,
			newCachedDeviceBindingRepository,
			repository.NewTenantDomainRepository,
			repository.NewTenantRepository,
			repository.NewTenantPluginRepository,
			repository.NewQaRepository,
		),
	)
}
