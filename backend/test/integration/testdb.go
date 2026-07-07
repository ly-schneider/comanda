// Package integration provides integration test infrastructure.
package integration

import (
	"context"
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"backend/internal/config"
	"backend/internal/crypto"
	"backend/internal/enttenant"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/device"
	"backend/internal/generated/ent/inventoryledger"
	"backend/internal/generated/ent/order"
	"backend/internal/generated/ent/orderline"
	"backend/internal/generated/ent/product"
	"backend/internal/generated/ent/tenant"
	"backend/internal/generated/ent/user"
	pgRepo "backend/internal/repository"
	"backend/internal/service"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
)

// DefaultTestTenantID is the tenant every functional integration test runs
// under. The functional suites use the privileged pool (RLS bypassed by
// ownership); a fixed-default tenant guard stamps tenant_id on writes and
// filters reads so they exercise the multi-tenant code paths without each test
// having to thread a tenant through.
var DefaultTestTenantID = "tenant_test1"

// randSuffix returns 8 lowercase hex chars for unique slugs/hostnames in
// tests — lowercase-only so DB hostname CHECKs and host normalization can't
// reject it.
func randSuffix() string {
	b := make([]byte, 4)
	if _, err := cryptorand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand: %v", err))
	}
	return hex.EncodeToString(b)
}

// TestDB wraps an Ent client and raw sql.DB for integration tests.
type TestDB struct {
	Client *ent.Client
	DB     *sql.DB
}

// NewTestDB creates a new test database connection using Ent.
// It expects POSTGRES_TEST_DSN environment variable to be set.
// If not set, tests will be skipped.
func NewTestDB(t *testing.T) *TestDB {
	t.Helper()

	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN not set; skipping integration tests")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Fatalf("failed to ping test database: %v", err)
	}

	drv := entsql.OpenDB(dialect.Postgres, db)
	client := ent.NewClient(ent.Driver(drv))

	tdb := &TestDB{Client: client, DB: db}

	// Create the app schema and run Ent auto-migration
	tdb.setupSchema(t)
	tdb.seedDefaultTenant(t)

	// Stamp every write with the default test tenant so functional suites
	// satisfy the tenant_id NOT NULL columns. Functional DBs only ever hold the
	// default tenant's data, so reads need no extra filtering.
	client.Use(defaultTenantHook)

	return tdb
}

var migrateOnce sync.Once
var migrateErr error

// setupSchema applies the Atlas migration directory (not Ent's Schema.Create)
// so the test schema matches production exactly: ON DELETE rules, the
// per-tenant unique constraints, and — critically — the RLS policies. Applied
// once per test process against POSTGRES_TEST_DSN (privileged).
func (tdb *TestDB) setupSchema(t *testing.T) {
	t.Helper()

	migrateOnce.Do(func() {
		dir := migrationsDir(t)
		dsn := os.Getenv("POSTGRES_TEST_DSN")
		out, err := exec.Command(
			"atlas", "migrate", "apply",
			"--dir", "file://"+dir,
			"--url", dsn,
		).CombinedOutput()
		if err != nil {
			migrateErr = fmt.Errorf("atlas migrate apply: %v\n%s", err, out)
		}
	})
	if migrateErr != nil {
		t.Fatalf("failed to apply Atlas migrations: %v", migrateErr)
	}
}

func migrationsDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(wd, "..", "..", "db", "migrations")
}

func (tdb *TestDB) seedDefaultTenant(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	exists, err := tdb.Client.Tenant.Query().Where(tenant.IDEQ(DefaultTestTenantID)).Exist(ctx)
	if err != nil {
		t.Fatalf("failed to check default tenant: %v", err)
	}
	if exists {
		return
	}
	if err := tdb.Client.Tenant.Create().
		SetID(DefaultTestTenantID).
		SetSlug("test-default").
		SetName("Test Default Tenant").
		Exec(ctx); err != nil {
		t.Fatalf("failed to seed default tenant: %v", err)
	}
}

// defaultTenantHook stamps DefaultTestTenantID on writes to scoped entities.
func defaultTenantHook(next ent.Mutator) ent.Mutator {
	return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
		if !enttenant.IsScoped(m.Type()) {
			return next.Mutate(ctx, m)
		}
		if tm, ok := m.(interface {
			SetTenantID(string)
			TenantID() (string, bool)
		}); ok {
			if _, set := tm.TenantID(); !set {
				tm.SetTenantID(DefaultTestTenantID)
			}
		}
		return next.Mutate(ctx, m)
	})
}

// Cleanup truncates all tables for a fresh test state.
func (tdb *TestDB) Cleanup(t *testing.T) {
	t.Helper()

	// Tables ordered to respect foreign key constraints
	tables := []string{
		"tenant_plugin",
		"idempotency",
		"order_line_redemption",
		"inventory_ledger",
		"refund_line",
		"refund",
		"order_payment",
		"order_line",
		"\"order\"",
		"menu_slot_option",
		"menu_slot",
		"device_product",
		"device_binding",
		"device",
		"product",
		"jeton",
		"category",
		"settings",
		"session",
		"admin_invite",
		"verification",
		"\"user\"",
	}

	ctx := context.Background()
	for _, table := range tables {
		if _, err := tdb.DB.ExecContext(ctx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)); err != nil {
			// Ignore errors for non-existent tables
			if !strings.Contains(err.Error(), "does not exist") {
				t.Logf("warning: failed to truncate %s: %v", table, err)
			}
		}
	}
}

// Close closes the database connection.
func (tdb *TestDB) Close() {
	_ = tdb.Client.Close()
	_ = tdb.DB.Close()
}

// Repositories holds all repository instances for tests.
type Repositories struct {
	Client            *ent.Client
	Category          pgRepo.CategoryRepository
	Product           *pgRepo.ProductRepository
	ProductVariant    pgRepo.ProductVariantRepository
	Jeton             pgRepo.JetonRepository
	MenuSlot          pgRepo.MenuSlotRepository
	MenuSlotOption    pgRepo.MenuSlotOptionRepository
	Order             pgRepo.OrderRepository
	OrderLine         pgRepo.OrderLineRepository
	OrderPayment      pgRepo.OrderPaymentRepository
	OrderRedemption   pgRepo.OrderLineRedemptionRepository
	Refund            pgRepo.RefundRepository
	Inventory         pgRepo.InventoryLedgerRepository
	Device            pgRepo.DeviceRepository
	DeviceProduct     pgRepo.DeviceProductRepository
	Settings          pgRepo.SettingsRepository
	Idempotency       pgRepo.IdempotencyRepository
	VolunteerCampaign pgRepo.VolunteerCampaignRepository
	VolunteerRedeem   pgRepo.VolunteerRedemptionRepository
}

// NewRepositories creates all repository instances from an Ent client.
func NewRepositories(client *ent.Client) *Repositories {
	return &Repositories{
		Client:            client,
		Category:          pgRepo.NewCategoryRepository(client),
		Product:           pgRepo.NewProductRepository(client),
		ProductVariant:    pgRepo.NewProductVariantRepository(client),
		Jeton:             pgRepo.NewJetonRepository(client),
		MenuSlot:          pgRepo.NewMenuSlotRepository(client),
		MenuSlotOption:    pgRepo.NewMenuSlotOptionRepository(client),
		Order:             pgRepo.NewOrderRepository(client),
		OrderLine:         pgRepo.NewOrderLineRepository(client),
		OrderPayment:      pgRepo.NewOrderPaymentRepository(client),
		OrderRedemption:   pgRepo.NewOrderLineRedemptionRepository(client),
		Refund:            pgRepo.NewRefundRepository(client),
		Inventory:         pgRepo.NewInventoryLedgerRepository(client),
		Device:            pgRepo.NewDeviceRepository(client),
		DeviceProduct:     pgRepo.NewDeviceProductRepository(client),
		Settings:          pgRepo.NewSettingsRepository(client),
		Idempotency:       pgRepo.NewIdempotencyRepository(client),
		VolunteerCampaign: pgRepo.NewVolunteerCampaignRepository(client),
		VolunteerRedeem:   pgRepo.NewVolunteerRedemptionRepository(client),
	}
}

// Fixtures provides helpers to create test data.
type Fixtures struct {
	repos *Repositories
	ctx   context.Context
}

// NewFixtures creates a new Fixtures helper.
func NewFixtures(repos *Repositories) *Fixtures {
	return &Fixtures{repos: repos, ctx: context.Background()}
}

// testQREncryptor is a fixed-key AES-256-GCM encryptor used so the payment
// service can generate/persist per-tenant QR signing keys in tests. The key is
// non-secret (test-only).
func testQREncryptor() *crypto.Encryptor {
	key := make([]byte, crypto.KeySize)
	for i := range key {
		key[i] = byte(i + 1)
	}
	enc, err := crypto.NewEncryptor(key)
	if err != nil {
		panic(err)
	}
	return enc
}

// NewQRKeySvc builds a QRKeyService over the test repositories using the shared
// test encryptor, so a test can verify the public key the payment service signed
// with.
func NewQRKeySvc(repos *Repositories) service.QRKeyService {
	return service.NewQRKeyService(repos.Settings, testQREncryptor())
}

// NewPaymentSvc builds a PaymentService with a nil provider factory; it wires a
// real QRKeyService (fixed test key) so created orders get a signed qr_payload.
// Cross-tenant payment isolation is covered by the dedicated RLS tests.
func NewPaymentSvc(cfg config.Config, repos *Repositories, products service.ProductService) service.PaymentService {
	return service.NewPaymentService(
		cfg,
		nil, // provider factory
		testQREncryptor(),
		repos.Order,
		repos.OrderLine,
		repos.OrderPayment,
		products,
		repos.MenuSlot,
		repos.Inventory,
		nil, // inventory hub
		nil, // email service
		repos.Settings,
		nil, // tenant domain repo
		NewQRKeySvc(repos),
		nil, // app db
		zap.NewNop(),
	)
}

// NewProductSvc builds a ProductService wired to the test repositories.
// Useful for tests that need to pass a ProductService into other services.
func NewProductSvc(repos *Repositories) service.ProductService {
	return service.NewProductService(
		repos.Product,
		repos.ProductVariant,
		repos.Category,
		repos.MenuSlot,
		repos.MenuSlotOption,
		repos.Inventory,
		repos.Jeton,
		nil,
	)
}

// CreateCategory creates a test category.
func (f *Fixtures) CreateCategory(name string, position int, isActive bool) *ent.Category {
	cat, err := f.repos.Category.Create(f.ctx, name, position, isActive)
	if err != nil {
		panic(fmt.Sprintf("failed to create category: %v", err))
	}
	return cat
}

// CreateJeton creates a test jeton.
func (f *Fixtures) CreateJeton(name, color string) *ent.Jeton {
	jeton, err := f.repos.Jeton.Create(f.ctx, name, color)
	if err != nil {
		panic(fmt.Sprintf("failed to create jeton: %v", err))
	}
	return jeton
}

// CreateProduct creates a test product.
func (f *Fixtures) CreateProduct(name string, categoryID string, priceCents int64, productType product.Type, jetonID *string) *ent.Product {
	p, err := f.repos.Product.Create(f.ctx, categoryID, productType, name, priceCents, 0, true, nil, nil, jetonID, nil)
	if err != nil {
		panic(fmt.Sprintf("failed to create product: %v", err))
	}
	return p
}

// CreateProductWithDeposit creates a simple test product carrying a per-unit deposit.
func (f *Fixtures) CreateProductWithDeposit(name string, categoryID string, priceCents, depositCents int64) *ent.Product {
	p, err := f.repos.Product.Create(f.ctx, categoryID, product.TypeSimple, name, priceCents, depositCents, true, nil, nil, nil, nil)
	if err != nil {
		panic(fmt.Sprintf("failed to create product: %v", err))
	}
	return p
}

// CreateOrder creates a test order.
func (f *Fixtures) CreateOrder(totalCents int64, status order.Status, origin order.Origin) *ent.Order {
	ord, err := f.repos.Order.Create(f.ctx, totalCents, status, origin, nil, nil, nil, nil, nil)
	if err != nil {
		panic(fmt.Sprintf("failed to create order: %v", err))
	}
	return ord
}

// CreateOrderWithCustomer creates an order with a customer ID, ensuring the
// referenced (global) user row exists so the FK holds.
func (f *Fixtures) CreateOrderWithCustomer(totalCents int64, status order.Status, origin order.Origin, customerID string) *ent.Order {
	f.ensureUser(customerID)
	ord, err := f.repos.Order.Create(f.ctx, totalCents, status, origin, &customerID, nil, nil, nil, nil)
	if err != nil {
		panic(fmt.Sprintf("failed to create order: %v", err))
	}
	return ord
}

// ensureUser idempotently creates a global user row for FK satisfaction.
func (f *Fixtures) ensureUser(id string) {
	exists, err := f.repos.Client.User.Query().Where(user.IDEQ(id)).Exist(f.ctx)
	if err != nil {
		panic(fmt.Sprintf("failed to check user: %v", err))
	}
	if exists {
		return
	}
	if err := f.repos.Client.User.Create().SetID(id).Exec(f.ctx); err != nil {
		panic(fmt.Sprintf("failed to create user: %v", err))
	}
}

// CreateOrderLine creates a test order line.
func (f *Fixtures) CreateOrderLine(orderID, productID string, title string, quantity int, priceCents int64, lineType orderline.LineType) *ent.OrderLine {
	line, err := f.repos.OrderLine.Create(f.ctx, orderID, lineType, productID, title, quantity, priceCents, nil, nil, nil)
	if err != nil {
		panic(fmt.Sprintf("failed to create order line: %v", err))
	}
	return line
}

// CreateDevice creates a test device.
func (f *Fixtures) CreateDevice(name, deviceKey string, deviceType device.Type, status device.Status) *ent.Device {
	d, err := f.repos.Device.Create(f.ctx, name, deviceKey, deviceType, status, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		panic(fmt.Sprintf("failed to create device: %v", err))
	}
	return d
}

// AddInventory adds inventory for a product.
func (f *Fixtures) AddInventory(productID string, delta int, reason inventoryledger.Reason) *ent.InventoryLedger {
	entry, err := f.repos.Inventory.Create(f.ctx, productID, delta, reason, nil, nil, nil, nil, nil)
	if err != nil {
		panic(fmt.Sprintf("failed to create inventory entry: %v", err))
	}
	return entry
}

// CreateVariant creates a test product variant.
func (f *Fixtures) CreateVariant(productID, value string, priceAdjustmentCents int64, isActive bool, sequence int) *ent.ProductVariant {
	v, err := f.repos.ProductVariant.Create(f.ctx, productID, value, priceAdjustmentCents, isActive, sequence)
	if err != nil {
		panic(fmt.Sprintf("failed to create variant: %v", err))
	}
	return v
}

// AddVariantInventory adds inventory for a specific variant.
func (f *Fixtures) AddVariantInventory(productID, variantID string, delta int, reason inventoryledger.Reason) *ent.InventoryLedger {
	entry, err := f.repos.Inventory.Create(f.ctx, productID, delta, reason, nil, nil, nil, nil, &variantID)
	if err != nil {
		panic(fmt.Sprintf("failed to create variant inventory entry: %v", err))
	}
	return entry
}

// CreateMenuSlot creates a test menu slot.
func (f *Fixtures) CreateMenuSlot(menuProductID string, name string, sequence int) *ent.MenuSlot {
	slot, err := f.repos.MenuSlot.Create(f.ctx, menuProductID, name, sequence)
	if err != nil {
		panic(fmt.Sprintf("failed to create menu slot: %v", err))
	}
	return slot
}

// CreateMenuSlotOption creates a test menu slot option.
func (f *Fixtures) CreateMenuSlotOption(slotID, optionProductID string) *ent.MenuSlotOption {
	opt, err := f.repos.MenuSlotOption.Create(f.ctx, slotID, optionProductID)
	if err != nil {
		panic(fmt.Sprintf("failed to create menu slot option: %v", err))
	}
	return opt
}

// AssignProductToDevice assigns a product to a device (station).
func (f *Fixtures) AssignProductToDevice(deviceID, productID string) {
	if _, err := f.repos.DeviceProduct.Create(f.ctx, deviceID, productID); err != nil {
		panic(fmt.Sprintf("failed to assign product to device: %v", err))
	}
}

// TestConfig returns a minimal config for testing.
func TestConfig() config.Config {
	return config.Config{
		App: config.AppConfig{
			AppEnv:        "test",
			PublicBaseURL: "http://localhost:3000",
		},
		Payrexx: config.PayrexxConfig{
			InstanceName: "",
			APISecret:    "",
		},
	}
}

// TimeWithin checks if two times are within a duration of each other.
func TimeWithin(a, b time.Time, d time.Duration) bool {
	diff := a.Sub(b)
	if diff < 0 {
		diff = -diff
	}
	return diff <= d
}
