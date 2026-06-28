package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"backend/internal/config"
	"backend/internal/crypto"
	"backend/internal/database"
	"backend/internal/enttenant"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/inventoryledger"
	"backend/internal/generated/ent/order"
	"backend/internal/generated/ent/orderline"
	"backend/internal/generated/ent/orderpayment"
	"backend/internal/generated/ent/product"
	"backend/internal/inventory"
	"backend/internal/payment"
	"backend/internal/payrexx"
	"backend/internal/qrsign"
	"backend/internal/repository"
	"backend/internal/trace"

	nanoid "backend/internal/id"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

type PaymentService interface {
	// IsPaymentConfigured requires the tenant transaction (app.tenant_id set).
	IsPaymentConfigured(ctx context.Context) bool
	// PrepareAndCreateOrder validates items and creates a pending order with inventory reservation.
	PrepareAndCreateOrder(ctx context.Context, in CreateCheckoutInput, userID *string, attemptID *string) (*CheckoutPreparation, error)
	// CreatePaymentCheckout creates a hosted checkout for the prepared order.
	CreatePaymentCheckout(ctx context.Context, prep *CheckoutPreparation, successURL, failedURL, cancelURL string) (*payment.CheckoutResult, error)
	// MarkOrderPaidByPayrexx marks an order as paid based on Payrexx webhook data.
	MarkOrderPaidByPayrexx(ctx context.Context, orderID string, gatewayID, transactionID int, contactEmail *string) error
	// MarkOrderPaidDev marks an order as paid in dev mode (no Payrexx gateway/transaction IDs).
	MarkOrderPaidDev(ctx context.Context, orderID string) error
	// FindPendingOrderByAttemptID finds a pending order by payment attempt ID.
	FindPendingOrderByAttemptID(ctx context.Context, attemptID string) (*ent.Order, error)
	// SetOrderAttemptID sets the payment attempt ID on an order.
	SetOrderAttemptID(ctx context.Context, orderID string, attemptID string) error
	// CleanupPendingOrderByID deletes a pending order and releases inventory.
	CleanupPendingOrderByID(ctx context.Context, orderID string) error
	// CleanupOtherPendingOrdersByAttemptID deletes other pending orders with the same attempt ID.
	CleanupOtherPendingOrdersByAttemptID(ctx context.Context, attemptID string, keepOrderID string) (int64, error)
	// GetPaymentCheckout retrieves a hosted checkout by its provider gateway ID.
	GetPaymentCheckout(ctx context.Context, gatewayID int) (*payment.CheckoutResult, error)
	// ConnectPayrexxAPI validates + stores the instance and API key (staged step
	// one), returning status with the webhook URL to register in Payrexx.
	ConnectPayrexxAPI(ctx context.Context, instance, apiSecret string) (PayrexxStatus, error)
	// SetPayrexxWebhookSecret stores the webhook secret (staged step two),
	// completing configuration.
	SetPayrexxWebhookSecret(ctx context.Context, webhookSecret string) (PayrexxStatus, error)
	// TestPayrexxConnection live-validates the stored API credentials.
	TestPayrexxConnection(ctx context.Context) error
	// RecordPayrexxWebhookReceived stamps the time of the last verified inbound
	// webhook, surfaced in the setup UI as a delivery confirmation.
	RecordPayrexxWebhookReceived(ctx context.Context) error
	// GetPayrexxStatus reports the staged setup state, webhook URL and last webhook.
	GetPayrexxStatus(ctx context.Context) (PayrexxStatus, error)
}

// PayrexxState is the staged-setup lifecycle of a tenant's Payrexx connection.
type PayrexxState string

const (
	PayrexxStateNotStarted   PayrexxState = "not_started"
	PayrexxStateAPIConnected PayrexxState = "api_connected"
	PayrexxStateConfigured   PayrexxState = "configured"
)

type PayrexxStatus struct {
	State         PayrexxState
	Instance      string
	WebhookURL    string
	LastWebhookAt *time.Time
}

type CreateCheckoutInput struct {
	Items []CheckoutItemInput `json:"items"`
	// CustomerEmail is optional; if user is logged in, their email is used.
	CustomerEmail *string `json:"customerEmail,omitempty"`
	// Origin is the order origin (shop or pos). Defaults to shop if empty.
	Origin order.Origin `json:"-"`
}

type CheckoutItemInput struct {
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
	// Configuration maps slotID -> selected productID for menu items.
	Configuration map[string]string `json:"configuration,omitempty"`
}

type CheckoutPreparation struct {
	OrderID       string
	TotalCents    int64
	LineItems     []payment.LineItem
	CustomerEmail *string
	UserID        *string
	Order         *ent.Order
	QRPayload     string
}

type paymentService struct {
	cfg              config.Config
	providers        *payment.Factory
	encryptor        *crypto.Encryptor
	orderRepo        repository.OrderRepository
	orderLineRepo    repository.OrderLineRepository
	orderPaymentRepo repository.OrderPaymentRepository
	products         ProductService
	menuSlotRepo     repository.MenuSlotRepository
	inventoryRepo    repository.InventoryLedgerRepository
	inventoryHub     *inventory.Hub
	emailService     EmailService
	settingsRepo     repository.SettingsRepository
	tenantDomainRepo repository.TenantDomainRepository
	qrKeys           QRKeyService
	appDB            *database.AppClient
	logger           *zap.Logger
}

func NewPaymentService(
	cfg config.Config,
	providers *payment.Factory,
	encryptor *crypto.Encryptor,
	orderRepo repository.OrderRepository,
	orderLineRepo repository.OrderLineRepository,
	orderPaymentRepo repository.OrderPaymentRepository,
	products ProductService,
	menuSlotRepo repository.MenuSlotRepository,
	inventoryRepo repository.InventoryLedgerRepository,
	inventoryHub *inventory.Hub,
	emailService EmailService,
	settingsRepo repository.SettingsRepository,
	tenantDomainRepo repository.TenantDomainRepository,
	qrKeys QRKeyService,
	appDB *database.AppClient,
	logger *zap.Logger,
) PaymentService {
	return &paymentService{
		cfg:              cfg,
		providers:        providers,
		encryptor:        encryptor,
		orderRepo:        orderRepo,
		orderLineRepo:    orderLineRepo,
		orderPaymentRepo: orderPaymentRepo,
		products:         products,
		menuSlotRepo:     menuSlotRepo,
		inventoryRepo:    inventoryRepo,
		inventoryHub:     inventoryHub,
		emailService:     emailService,
		settingsRepo:     settingsRepo,
		tenantDomainRepo: tenantDomainRepo,
		qrKeys:           qrKeys,
		appDB:            appDB,
		logger:           logger,
	}
}

func (s *paymentService) IsPaymentConfigured(ctx context.Context) bool {
	return s.providers != nil && s.providers.IsConfigured(ctx)
}

func (s *paymentService) PrepareAndCreateOrder(ctx context.Context, in CreateCheckoutInput, userID *string, attemptID *string) (*CheckoutPreparation, error) {
	ctx, finish := trace.StartSpan(ctx, "service", "payment.prepare_checkout")
	defer finish()
	trace.Data(ctx, "checkout.item_count", len(in.Items))
	trace.Data(ctx, "checkout.origin", string(in.Origin))
	if userID != nil {
		trace.Data(ctx, "checkout.user_id", *userID)
	}

	if len(in.Items) == 0 {
		trace.Err(ctx, fmt.Errorf("no items"))
		return nil, fmt.Errorf("no items")
	}

	// Collect all product IDs
	productIDSet := make(map[string]struct{})
	for _, it := range in.Items {
		if !nanoid.Valid(it.ProductID) {
			return nil, fmt.Errorf("invalid productId: %s", it.ProductID)
		}
		productIDSet[it.ProductID] = struct{}{}
		// Also collect configured child products
		for _, childID := range it.Configuration {
			if childID == "" {
				continue
			}
			if !nanoid.Valid(childID) {
				return nil, fmt.Errorf("invalid configuration productId: %s", childID)
			}
			productIDSet[childID] = struct{}{}
		}
	}

	ids := make([]string, 0, len(productIDSet))
	for id := range productIDSet {
		ids = append(ids, id)
	}

	var (
		products       []*ent.Product
		slots          []*ent.MenuSlot
		preloadedStock map[string]int
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		products, err = s.products.GetByIDs(gctx, ids)
		if err != nil {
			return fmt.Errorf("load products: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		slots, err = s.menuSlotRepo.GetByMenuProductIDs(gctx, ids)
		if err != nil {
			return fmt.Errorf("load menu slots: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		preloadedStock, err = s.inventoryRepo.GetCurrentStockBatch(gctx, ids)
		if err != nil {
			return fmt.Errorf("check inventory: %w", err)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	productMap := make(map[string]*ent.Product, len(products))
	for _, p := range products {
		productMap[p.ID] = p
	}

	slotByID := make(map[string]*ent.MenuSlot, len(slots))
	allowedBySlot := make(map[string]map[string]struct{}, len(slots))
	for _, slot := range slots {
		slotByID[slot.ID] = slot
		allowed := make(map[string]struct{})
		for _, opt := range slot.Edges.Options {
			allowed[opt.OptionProductID] = struct{}{}
		}
		allowedBySlot[slot.ID] = allowed
	}

	// Calculate total and validate
	var totalCents int64
	for _, it := range in.Items {
		p, ok := productMap[it.ProductID]
		if !ok {
			return nil, fmt.Errorf("unknown product: %s", it.ProductID)
		}
		totalCents += p.PriceCents * int64(it.Quantity)

		// TWINT limit: 5000 CHF per transaction
		if p.PriceCents*int64(it.Quantity) > 500000 {
			return nil, fmt.Errorf("item exceeds TWINT max: %s", p.Name)
		}
	}
	if totalCents > 500000 {
		return nil, fmt.Errorf("order exceeds TWINT max (5000 CHF)")
	}

	// Validate inventory availability
	requiredQuantities := make(map[string]int)
	for _, it := range in.Items {
		p := productMap[it.ProductID]
		if p.Type == product.TypeSimple {
			requiredQuantities[it.ProductID] += it.Quantity
		}
		for _, childID := range it.Configuration {
			if childID == "" {
				continue
			}
			requiredQuantities[childID] += it.Quantity
		}
	}

	for pid, required := range requiredQuantities {
		available := preloadedStock[pid]
		if available < required {
			pName := "unknown"
			if p, ok := productMap[pid]; ok {
				pName = p.Name
			}
			return nil, fmt.Errorf("insufficient inventory for %s: requested %d, available %d", pName, required, available)
		}
	}

	origin := in.Origin
	if origin == "" {
		origin = order.OriginShop
	}

	// Create the order
	ord, err := s.orderRepo.Create(ctx, totalCents, order.StatusPending, origin, userID, in.CustomerEmail, attemptID, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}

	// Build order lines
	var orderLines []repository.OrderLineCreateParams
	var inventoryEntries []repository.InventoryLedgerCreateParams

	for _, it := range in.Items {
		p := productMap[it.ProductID]

		// Determine parent line type
		lt := orderline.LineTypeSimple
		if p.Type == product.TypeMenu {
			lt = orderline.LineTypeBundle
		}

		parentLineID := nanoid.New()
		parentLine := repository.OrderLineCreateParams{
			ID:             &parentLineID,
			OrderID:        ord.ID,
			LineType:       lt,
			ProductID:      p.ID,
			Title:          p.Name,
			Quantity:       it.Quantity,
			UnitPriceCents: p.PriceCents,
		}
		orderLines = append(orderLines, parentLine)

		// Reserve inventory for simple products
		if p.Type == product.TypeSimple && it.Quantity > 0 {
			inventoryEntries = append(inventoryEntries, repository.InventoryLedgerCreateParams{
				ProductID: p.ID,
				Delta:     -it.Quantity,
				Reason:    inventoryledger.ReasonSale,
				OrderID:   &ord.ID,
			})
		}

		// Process menu configurations
		if p.Type == product.TypeMenu && len(it.Configuration) > 0 {
			for slotID, childProdID := range it.Configuration {
				if childProdID == "" {
					continue
				}
				if !nanoid.Valid(slotID) {
					return nil, fmt.Errorf("invalid menu slot id: %s", slotID)
				}
				slot, ok := slotByID[slotID]
				if !ok || slot.MenuProductID != p.ID {
					return nil, fmt.Errorf("slot does not belong to product: %s", slotID)
				}
				if !nanoid.Valid(childProdID) {
					return nil, fmt.Errorf("invalid configured product id: %s", childProdID)
				}
				// Validate allowed
				if allowed := allowedBySlot[slotID]; allowed != nil {
					if _, ok := allowed[childProdID]; !ok {
						return nil, fmt.Errorf("product not allowed in slot")
					}
				}

				childProd := productMap[childProdID]
				slotName := slot.Name
				childLine := repository.OrderLineCreateParams{
					OrderID:        ord.ID,
					LineType:       orderline.LineTypeComponent,
					ProductID:      childProdID,
					Title:          childProd.Name,
					Quantity:       it.Quantity,
					UnitPriceCents: 0,
					ParentLineID:   &parentLineID,
					MenuSlotID:     &slotID,
					MenuSlotName:   &slotName,
				}
				orderLines = append(orderLines, childLine)

				// Reserve inventory for component
				if it.Quantity > 0 {
					inventoryEntries = append(inventoryEntries, repository.InventoryLedgerCreateParams{
						ProductID: childProdID,
						Delta:     -it.Quantity,
						Reason:    inventoryledger.ReasonSale,
						OrderID:   &ord.ID,
					})
				}
			}
		}
	}

	// Insert order lines
	if _, err := s.orderLineRepo.CreateBatch(ctx, orderLines); err != nil {
		return nil, fmt.Errorf("insert order lines: %w", err)
	}

	// Reserve inventory
	if len(inventoryEntries) > 0 {
		if _, err := s.inventoryRepo.CreateMany(ctx, inventoryEntries); err != nil {
			return nil, fmt.Errorf("reserve inventory: %w", err)
		}
		s.publishInventoryUpdates(ctx, inventoryEntries, preloadedStock)
	}

	// An order is redeemed offline from its signed token, so one without a token
	// is unredeemable: a signing or persist failure must fail the order (tx rolls back).
	qrToken, err := s.signOrderQR(ctx, ord.ID, orderLines)
	if err != nil {
		return nil, fmt.Errorf("sign order qr: %w", err)
	}
	if err := s.orderRepo.SetQRPayload(ctx, ord.ID, qrToken); err != nil {
		return nil, fmt.Errorf("persist qr payload: %w", err)
	}
	ord.QrPayload = &qrToken

	// Prepare provider line items from the params (we have all the data we need)
	lineItems := make([]payment.LineItem, 0)
	for _, line := range orderLines {
		if line.ParentLineID == nil && line.UnitPriceCents > 0 && line.Quantity > 0 {
			lineItems = append(lineItems, payment.LineItem{
				Name:     line.Title,
				Quantity: line.Quantity,
				Amount:   int(line.UnitPriceCents),
			})
		}
	}

	trace.Data(ctx, "checkout.order_id", ord.ID)
	trace.Data(ctx, "checkout.total_cents", totalCents)
	trace.Data(ctx, "checkout.line_count", len(orderLines))

	return &CheckoutPreparation{
		OrderID:       ord.ID,
		TotalCents:    totalCents,
		LineItems:     lineItems,
		CustomerEmail: in.CustomerEmail,
		UserID:        userID,
		Order:         ord,
		QRPayload:     qrToken,
	}, nil
}

// signOrderQR signs the pickup token over the order's redeemable physical items:
// simple products and the chosen menu components — the items actually handed
// over and tracked in inventory. A configured menu's bundle parent is a grouping
// row with no stock of its own, so it is omitted; its components carry the items.
// A degenerate bundle with no components falls back to the parent so the item is
// never lost from the token.
func (s *paymentService) signOrderQR(ctx context.Context, orderID string, lines []repository.OrderLineCreateParams) (string, error) {
	ctx, finish := trace.StartSpan(ctx, "service", "payment.sign_qr")
	defer finish()

	if s.qrKeys == nil {
		return "", fmt.Errorf("qr signing not configured")
	}
	tenantID, ok := repository.TenantIDFromContext(ctx)
	if !ok {
		return "", fmt.Errorf("qr sign: no tenant in context")
	}

	hasComponents := make(map[string]bool, len(lines))
	for _, l := range lines {
		if l.ParentLineID != nil {
			hasComponents[*l.ParentLineID] = true
		}
	}

	redeemable := make([]qrsign.Line, 0, len(lines))
	for _, l := range lines {
		switch l.LineType {
		case orderline.LineTypeSimple, orderline.LineTypeComponent:
			redeemable = append(redeemable, qrsign.Line{ProductID: l.ProductID, Quantity: l.Quantity})
		case orderline.LineTypeBundle:
			if l.ID == nil || !hasComponents[*l.ID] {
				redeemable = append(redeemable, qrsign.Line{ProductID: l.ProductID, Quantity: l.Quantity})
			}
		}
	}

	priv, _, err := s.qrKeys.EnsureTenantKey(ctx)
	if err != nil {
		return "", fmt.Errorf("qr sign: ensure tenant key: %w", err)
	}

	token, err := qrsign.Sign(priv, qrsign.Payload{
		Version:  qrsign.Version,
		TenantID: tenantID,
		OrderID:  orderID,
		IssuedAt: time.Now().Unix(),
		Lines:    redeemable,
	})
	if err != nil {
		return "", fmt.Errorf("qr sign: sign payload: %w", err)
	}
	return token, nil
}

func (s *paymentService) CreatePaymentCheckout(ctx context.Context, prep *CheckoutPreparation, successURL, failedURL, cancelURL string) (*payment.CheckoutResult, error) {
	ctx, finish := trace.StartSpan(ctx, "service", "payment.create_checkout")
	defer finish()
	trace.Data(ctx, "gateway.order_id", prep.OrderID)
	trace.Data(ctx, "gateway.amount_cents", prep.TotalCents)

	if s.providers == nil {
		return nil, fmt.Errorf("payment provider factory not configured")
	}
	provider, err := s.providers.For(ctx)
	if err != nil {
		trace.Err(ctx, err)
		return nil, err
	}

	currency := "CHF"
	if s.settingsRepo != nil {
		if st, err := s.settingsRepo.Get(ctx); err == nil && st.Currency != "" {
			currency = st.Currency
		}
	}

	gatewayCtx, gatewayCancel := context.WithTimeout(ctx, payrexx.DefaultRequestTimeout)
	defer gatewayCancel()
	// TODO(P4): the TWINT payment-means and Payrexx's 5000-unit TWINT limit are
	// CHF-specific; revisit when non-CHF currencies / providers land.
	result, err := provider.CreateCheckout(gatewayCtx, payment.CheckoutRequest{
		AmountCents:        int(prep.TotalCents),
		Currency:           currency,
		ReferenceID:        prep.OrderID,
		SuccessRedirectURL: successURL,
		FailedRedirectURL:  failedURL,
		CancelRedirectURL:  cancelURL,
		PaymentMeans:       []string{"twint"},
		LineItems:          prep.LineItems,
		CustomerEmail:      safeStr(prep.CustomerEmail),
		Purpose:            "Comanda Order",
		ValidityMinutes:    15,
	})
	if err != nil {
		return nil, fmt.Errorf("create payment checkout: %w", err)
	}

	// Update order with gateway ID - get current order first to preserve all fields
	ord, err := s.orderRepo.GetByID(ctx, prep.OrderID)
	if err != nil {
		return nil, err
	}
	gwID := result.GatewayID
	if _, err := s.orderRepo.Update(ctx, ord.ID, ord.TotalCents, ord.Status, ord.Origin, ord.CustomerID, ord.ContactEmail, ord.PaymentAttemptID, &gwID, ord.PayrexxTransactionID); err != nil {
		return nil, fmt.Errorf("update order with gateway: %w", err)
	}

	return result, nil
}

func (s *paymentService) MarkOrderPaidByPayrexx(ctx context.Context, orderID string, gatewayID, transactionID int, contactEmail *string) error {
	ctx, finish := trace.StartSpan(ctx, "service", "payment.mark_paid_payrexx")
	defer finish()
	trace.Data(ctx, "payment.order_id", orderID)
	trace.Data(ctx, "payment.gateway_id", gatewayID)
	trace.Data(ctx, "payment.transaction_id", transactionID)

	ord, err := s.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("order not found: %w", err)
	}

	ce := ord.ContactEmail
	if contactEmail != nil && *contactEmail != "" {
		ce = contactEmail
	}

	now := time.Now()

	if _, err := s.orderPaymentRepo.Create(ctx, ord.ID, orderpayment.MethodTWINT, ord.TotalCents, now, nil); err != nil {
		return fmt.Errorf("create order payment: %w", err)
	}

	if _, err = s.orderRepo.Update(ctx, ord.ID, ord.TotalCents, order.StatusPaid, ord.Origin, ord.CustomerID, ce, ord.PaymentAttemptID, &gatewayID, &transactionID); err != nil {
		return err
	}

	email := ""
	if ce != nil {
		email = *ce
	}
	if email != "" {
		s.logger.Info("scheduling receipt email",
			zap.String("orderId", ord.ID),
			zap.String("to", email),
		)
		go s.sendReceipt(ctx, ord.ID, email, ord.TotalCents, now, "TWINT")
	} else {
		s.logger.Info("no contact email, skipping receipt",
			zap.String("orderId", ord.ID),
		)
	}

	return nil
}

func (s *paymentService) MarkOrderPaidDev(ctx context.Context, orderID string) error {
	ord, err := s.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("order not found: %w", err)
	}

	if ord.Status != order.StatusPending {
		return fmt.Errorf("order is not pending")
	}

	if err := s.orderRepo.UpdateStatus(ctx, orderID, order.StatusPaid); err != nil {
		return err
	}

	email := ""
	if ord.ContactEmail != nil {
		email = *ord.ContactEmail
	}
	if email != "" {
		s.logger.Info("scheduling receipt email (dev)",
			zap.String("orderId", ord.ID),
			zap.String("to", email),
		)
		go s.sendReceipt(ctx, ord.ID, email, ord.TotalCents, time.Now(), "TWINT (Dev)")
	}

	return nil
}

func (s *paymentService) FindPendingOrderByAttemptID(ctx context.Context, attemptID string) (*ent.Order, error) {
	if attemptID == "" {
		return nil, fmt.Errorf("missing attempt id")
	}
	return s.orderRepo.FindPendingByAttemptID(ctx, attemptID)
}

func (s *paymentService) SetOrderAttemptID(ctx context.Context, orderID string, attemptID string) error {
	return s.orderRepo.SetPaymentAttemptID(ctx, orderID, attemptID)
}

func (s *paymentService) CleanupPendingOrderByID(ctx context.Context, orderID string) error {
	ctx, finish := trace.StartSpan(ctx, "service", "payment.cleanup_pending")
	defer finish()
	trace.Data(ctx, "cleanup.order_id", orderID)

	ord, err := s.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		trace.Err(ctx, err)
		return err
	}
	if ord.Status != order.StatusPending {
		trace.Data(ctx, "cleanup.skipped", "not_pending")
		return nil
	}

	// Load order lines to release inventory
	lines, err := s.orderLineRepo.GetByOrderID(ctx, orderID)
	if err != nil {
		return err
	}

	// Release inventory
	var releaseEntries []repository.InventoryLedgerCreateParams
	for _, line := range lines {
		if line.Quantity > 0 {
			// Only release for simple products or components (not menu bundles themselves)
			if line.LineType == orderline.LineTypeSimple || line.LineType == orderline.LineTypeComponent {
				releaseEntries = append(releaseEntries, repository.InventoryLedgerCreateParams{
					ProductID: line.ProductID,
					Delta:     line.Quantity, // Positive to add back
					Reason:    inventoryledger.ReasonCorrection,
					OrderID:   &orderID,
				})
			}
		}
	}
	if len(releaseEntries) > 0 {
		_, _ = s.inventoryRepo.CreateMany(ctx, releaseEntries)
		s.publishInventoryUpdates(ctx, releaseEntries, nil)
	}

	// Delete order (cascade deletes order lines)
	_, err = s.orderRepo.DeleteIfPending(ctx, orderID)
	return err
}

func (s *paymentService) CleanupOtherPendingOrdersByAttemptID(ctx context.Context, attemptID string, keepOrderID string) (int64, error) {
	return s.orderRepo.DeletePendingByAttemptIDExcept(ctx, attemptID, keepOrderID)
}

func (s *paymentService) GetPaymentCheckout(ctx context.Context, gatewayID int) (*payment.CheckoutResult, error) {
	if s.providers == nil {
		return nil, fmt.Errorf("payment provider factory not configured")
	}
	provider, err := s.providers.For(ctx)
	if err != nil {
		return nil, err
	}
	return provider.GetCheckout(ctx, gatewayID)
}

func (s *paymentService) ConnectPayrexxAPI(ctx context.Context, instance, apiSecret string) (PayrexxStatus, error) {
	if s.encryptor == nil {
		return PayrexxStatus{}, fmt.Errorf("payment master key not configured")
	}
	if instance == "" || apiSecret == "" {
		return PayrexxStatus{}, fmt.Errorf("instance and api key are required")
	}

	normalized := payrexx.NormalizeInstance(instance)
	if normalized == "" {
		return PayrexxStatus{}, fmt.Errorf("instance and api key are required")
	}

	if err := payment.ValidatePayrexxCredentials(ctx, normalized, apiSecret); err != nil {
		return PayrexxStatus{}, err
	}

	apiEnc, err := s.encryptor.EncryptString(apiSecret)
	if err != nil {
		return PayrexxStatus{}, fmt.Errorf("encrypt api key: %w", err)
	}

	token, err := s.settingsRepo.EnsureWebhookToken(ctx)
	if err != nil {
		return PayrexxStatus{}, fmt.Errorf("ensure webhook token: %w", err)
	}

	if err := s.settingsRepo.UpsertPayrexxAPICredentials(ctx, normalized, apiEnc, token); err != nil {
		return PayrexxStatus{}, fmt.Errorf("store payrexx api credentials: %w", err)
	}

	return s.GetPayrexxStatus(ctx)
}

func (s *paymentService) SetPayrexxWebhookSecret(ctx context.Context, webhookSecret string) (PayrexxStatus, error) {
	if s.encryptor == nil {
		return PayrexxStatus{}, fmt.Errorf("payment master key not configured")
	}
	if webhookSecret == "" {
		return PayrexxStatus{}, fmt.Errorf("webhook secret is required")
	}

	enc, err := s.encryptor.EncryptString(webhookSecret)
	if err != nil {
		return PayrexxStatus{}, fmt.Errorf("encrypt webhook secret: %w", err)
	}

	if err := s.settingsRepo.SetPayrexxWebhookSecret(ctx, enc); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return PayrexxStatus{}, fmt.Errorf("connect the Payrexx API first")
		}
		return PayrexxStatus{}, fmt.Errorf("store payrexx webhook secret: %w", err)
	}

	return s.GetPayrexxStatus(ctx)
}

func (s *paymentService) TestPayrexxConnection(ctx context.Context) error {
	if s.encryptor == nil {
		return fmt.Errorf("payment master key not configured")
	}
	instance, apiEnc, err := s.settingsRepo.GetPayrexxAPICredentials(ctx)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return fmt.Errorf("connect the Payrexx API first")
		}
		return err
	}
	apiSecret, err := s.encryptor.DecryptString(apiEnc)
	if err != nil {
		return fmt.Errorf("decrypt api key: %w", err)
	}
	return payment.ValidatePayrexxCredentials(ctx, instance, apiSecret)
}

func (s *paymentService) RecordPayrexxWebhookReceived(ctx context.Context) error {
	return s.settingsRepo.TouchPayrexxWebhookReceived(ctx)
}

func (s *paymentService) GetPayrexxStatus(ctx context.Context) (PayrexxStatus, error) {
	cfg, err := s.settingsRepo.GetPayrexxConfig(ctx)
	if err != nil {
		return PayrexxStatus{}, err
	}

	status := PayrexxStatus{
		State:         PayrexxStateNotStarted,
		Instance:      cfg.Instance,
		LastWebhookAt: cfg.LastWebhookAt,
	}
	if cfg.Instance != "" && cfg.WebhookToken != "" {
		status.WebhookURL = s.webhookURL(cfg.WebhookToken)
	}
	switch {
	case cfg.HasAPISecret && cfg.HasWebhookSecret:
		status.State = PayrexxStateConfigured
	case cfg.HasAPISecret:
		status.State = PayrexxStateAPIConnected
	}
	return status, nil
}

func (s *paymentService) webhookURL(token string) string {
	base := strings.TrimRight(s.cfg.App.PublicBaseURL, "/")
	return base + "/v1/payments/webhooks/payrexx/" + token
}

// beginTenantTx opens a short transaction on the RLS-bound app pool, sets
// app.tenant_id, and returns a context carrying a tenant-guarded Ent client
// bound to that tx plus a cleanup func that commits (read-only here) and closes
// the connection. Used by detached goroutines that run after the request tx.
func (s *paymentService) beginTenantTx(ctx context.Context, tenantID string) (context.Context, func(), error) {
	if s.appDB == nil {
		return nil, nil, fmt.Errorf("payment service has no app db configured")
	}

	conn, err := s.appDB.DB.Conn(ctx)
	if err != nil {
		return nil, nil, err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	// Seed both GUCs atomically, mirroring the request tenant-tx middleware: a
	// per-tenant background job is never a platform admin, so app.is_platform_admin
	// is pinned false rather than left to the NULL→false default.
	if _, err := tx.ExecContext(ctx,
		"SELECT set_config('app.tenant_id', $1, true), set_config('app.is_platform_admin', 'false', true)",
		tenantID,
	); err != nil {
		_ = tx.Rollback()
		_ = conn.Close()
		return nil, nil, err
	}

	txClient := ent.NewClient(ent.Driver(database.NewTxEntDriver(tx)))
	enttenant.Register(txClient)

	ctx = repository.ContextWithClient(ctx, txClient)
	ctx = repository.ContextWithTenantID(ctx, tenantID)
	ctx = enttenant.WithTenant(ctx, tenantID)

	cleanup := func() {
		_ = tx.Commit()
		_ = conn.Close()
	}
	return ctx, cleanup, nil
}

func (s *paymentService) sendReceipt(reqCtx context.Context, orderID string, to string, totalCents int64, paidAt time.Time, method string) {
	if s.emailService == nil {
		return
	}

	// The receipt is built on a detached background context after the request
	// tx has committed, so open a fresh, short tenant transaction on the app
	// pool with the same app.tenant_id GUC. Without it the RLS-bound app
	// connection returns zero order lines and the receipt has no items.
	tenantID, ok := repository.TenantIDFromContext(reqCtx)
	if !ok {
		s.logger.Error("receipt: no tenant in context; skipping", zap.String("orderId", orderID))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ctx, commit, err := s.beginTenantTx(ctx, tenantID)
	if err != nil {
		s.logger.Error("receipt: failed to open tenant tx", zap.Error(err), zap.String("orderId", orderID))
		return
	}
	defer commit()

	lines, err := s.orderLineRepo.GetByOrderID(ctx, orderID)
	if err != nil {
		s.logger.Error("receipt: failed to load order lines", zap.Error(err), zap.String("orderId", orderID))
		return
	}

	childrenByParent := make(map[string][]ReceiptLineItem)
	var roots []*ent.OrderLine
	for _, l := range lines {
		if l.ParentLineID != nil {
			childrenByParent[*l.ParentLineID] = append(childrenByParent[*l.ParentLineID], ReceiptLineItem{
				Title:    l.Title,
				Quantity: l.Quantity,
				Cents:    l.UnitPriceCents,
			})
		} else {
			roots = append(roots, l)
		}
	}

	items := make([]ReceiptLineItem, 0, len(roots))
	for _, r := range roots {
		items = append(items, ReceiptLineItem{
			Title:    r.Title,
			Quantity: r.Quantity,
			Cents:    r.UnitPriceCents,
			Children: childrenByParent[r.ID],
		})
	}

	orderURL := s.tenantOrderURL(ctx, orderID)

	data := ReceiptEmailData{
		OrderID:    orderID,
		OrderURL:   orderURL,
		OrderDate:  formatOrderDate(paidAt),
		Items:      items,
		TotalCents: totalCents,
		Method:     method,
	}

	if err := s.emailService.SendReceiptEmail(ctx, to, data); err != nil {
		s.logger.Error("receipt: failed to send", zap.Error(err), zap.String("orderId", orderID), zap.String("to", to))
	}
}

// Order link must use the tenant's primary verified domain, not the platform
// apex (which has no tenant context). Runs inside the receipt's tenant tx.
func (s *paymentService) tenantOrderURL(ctx context.Context, orderID string) string {
	fallback := strings.TrimRight(s.cfg.App.PublicBaseURL, "/") + "/food/orders/" + orderID
	if s.tenantDomainRepo == nil {
		return fallback
	}

	domains, err := s.tenantDomainRepo.List(ctx)
	if err != nil {
		s.logger.Warn("receipt: failed to load tenant domains; using platform base",
			zap.Error(err), zap.String("orderId", orderID))
		return fallback
	}

	host := ""
	for _, d := range domains {
		if d.VerifiedAt == nil {
			continue
		}
		if d.IsPrimary {
			host = d.Hostname
			break
		}
		if host == "" {
			host = d.Hostname
		}
	}
	if host == "" {
		return fallback
	}
	return "https://" + host + "/food/orders/" + orderID
}

func safeStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (s *paymentService) publishInventoryUpdates(ctx context.Context, entries []repository.InventoryLedgerCreateParams, preStock map[string]int) {
	if s.inventoryHub == nil {
		return
	}
	productIDs := make([]string, 0, len(entries))
	deltaByProduct := make(map[string]int)
	for _, entry := range entries {
		if _, seen := deltaByProduct[entry.ProductID]; !seen {
			productIDs = append(productIDs, entry.ProductID)
		}
		deltaByProduct[entry.ProductID] += entry.Delta
	}
	stocks := make(map[string]int, len(productIDs))
	missing := make([]string, 0)
	for _, pid := range productIDs {
		if pre, ok := preStock[pid]; ok {
			stocks[pid] = pre + deltaByProduct[pid]
		} else {
			missing = append(missing, pid)
		}
	}
	if len(missing) > 0 {
		fetched, err := s.inventoryRepo.GetCurrentStockBatch(ctx, missing)
		if err != nil {
			return
		}
		for pid, v := range fetched {
			stocks[pid] = v
		}
	}
	now := time.Now()
	for _, productID := range productIDs {
		s.inventoryHub.Publish(inventory.Update{
			ProductID: productID,
			NewStock:  stocks[productID],
			Delta:     deltaByProduct[productID],
			Timestamp: now,
		})
	}
}
