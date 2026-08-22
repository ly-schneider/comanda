package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/inventoryledger"
	"backend/internal/generated/ent/order"
	"backend/internal/generated/ent/orderline"
	"backend/internal/generated/ent/product"
	"backend/internal/generated/ent/refund"
	"backend/internal/inventory"
	"backend/internal/payment"
	"backend/internal/qrsign"
	"backend/internal/repository"

	"go.uber.org/zap"
)

// ErrProviderRefund wraps a failure to resolve or call the payment provider for
// a TWINT refund, so the HTTP layer can return a generic gateway error rather
// than leaking the provider's message to the client.
var ErrProviderRefund = errors.New("payment provider refund failed")

// RefundProviderResolver yields the per-tenant payment provider used to issue
// TWINT refunds. *payment.Factory satisfies it; tests substitute a stub.
type RefundProviderResolver interface {
	For(ctx context.Context) (payment.Provider, error)
}

type RefundService interface {
	GetRefundView(ctx context.Context, orderID string) (*RefundView, error)
	RefundedSummary(ctx context.Context, orderID string) (RefundedSummary, error)
	ListRefunds(ctx context.Context, limit, offset int) ([]*ent.Refund, error)
	LookupForPOS(ctx context.Context, query string) ([]*ent.Order, error)
	RefundLines(ctx context.Context, in RefundLinesInput) (*ent.Refund, error)
	RefundArbitrary(ctx context.Context, in RefundArbitraryInput) (*ent.Refund, error)
	ReturnDeposit(ctx context.Context, in DepositReturnInput) (*ent.Refund, error)
	CancelOrder(ctx context.Context, in CancelOrderInput) (*ent.Refund, error)
	ReconcileFromWebhook(ctx context.Context, result *payment.WebhookResult) error
}

// RefundActor identifies who issued a refund: a POS operator (device) or an admin.
type RefundActor struct {
	Type     refund.ActorType
	ID       string
	DeviceID *string
}

type RefundLineQty struct {
	OrderLineID string
	Quantity    int
}

type RefundLinesInput struct {
	OrderID string
	Lines   []RefundLineQty
	Reason  *string
	Actor   RefundActor
}

type RefundArbitraryInput struct {
	OrderID     string
	AmountCents int64
	Reason      string
	Actor       RefundActor
}

type CancelOrderInput struct {
	OrderID string
	Reason  *string
	Actor   RefundActor
}

// DepositReturnInput returns deposit units for an order. Token, when present, is
// the scanned signed pickup token and must resolve to OrderID; Lines reference
// the order's deposit lines.
type DepositReturnInput struct {
	OrderID string
	Token   *string
	Lines   []RefundLineQty
	Reason  *string
	Actor   RefundActor
}

type RefundViewLine struct {
	Line                *ent.OrderLine
	RefundableUnitCents int64
	BoughtQuantity      int
	RefundedQuantity    int
	RefundableQty       int
	Redeemed            bool
}

// RefundViewDeposit is a returnable deposit line with its return accounting.
type RefundViewDeposit struct {
	Line                *ent.OrderLine
	RefundableUnitCents int64
	BoughtQuantity      int
	ReturnedQuantity    int
	OutstandingQty      int
}

type RefundView struct {
	Order                  *ent.Order
	Lines                  []RefundViewLine
	Deposits               []RefundViewDeposit
	Refunds                []*ent.Refund
	RefundableBalanceCents int64
}

type RefundedSummary struct {
	TotalCents int64
	ByLine     map[string]int
}

type refundService struct {
	orders       repository.OrderRepository
	refunds      repository.RefundRepository
	inventory    repository.InventoryLedgerRepository
	providers    RefundProviderResolver
	qrKeys       QRKeyService
	inventoryHub inventory.Publisher
	logger       *zap.Logger
}

func NewRefundService(
	orders repository.OrderRepository,
	refunds repository.RefundRepository,
	inventoryRepo repository.InventoryLedgerRepository,
	providers RefundProviderResolver,
	qrKeys QRKeyService,
	inventoryHub inventory.Publisher,
	logger *zap.Logger,
) RefundService {
	return &refundService{
		orders:       orders,
		refunds:      refunds,
		inventory:    inventoryRepo,
		providers:    providers,
		qrKeys:       qrKeys,
		inventoryHub: inventoryHub,
		logger:       logger,
	}
}

const posLookupLimit = 10

func (s *refundService) GetRefundView(ctx context.Context, orderID string) (*RefundView, error) {
	ord, err := s.orders.GetByIDWithRelations(ctx, orderID)
	if err != nil {
		return nil, err
	}
	succeededSum, refundedQty, err := s.refundedSoFar(ctx, ord.ID)
	if err != nil {
		return nil, err
	}
	refundsList, err := s.refunds.ListByOrder(ctx, ord.ID)
	if err != nil {
		return nil, err
	}

	var viewLines []RefundViewLine
	var deposits []RefundViewDeposit
	for _, l := range ord.Edges.Lines {
		if l.LineType == orderline.LineTypeDeposit {
			returned := refundedQty[l.ID]
			outstanding := l.Quantity - returned
			if outstanding < 0 {
				outstanding = 0
			}
			deposits = append(deposits, RefundViewDeposit{
				Line:                l,
				RefundableUnitCents: refundUnitPrice(ord, l),
				BoughtQuantity:      l.Quantity,
				ReturnedQuantity:    returned,
				OutstandingQty:      outstanding,
			})
			continue
		}
		if l.ParentLineID != nil {
			continue
		}
		refunded := refundedQty[l.ID]
		refundable := l.Quantity - refunded
		if refundable < 0 {
			refundable = 0
		}
		viewLines = append(viewLines, RefundViewLine{
			Line:                l,
			RefundableUnitCents: refundUnitPrice(ord, l),
			BoughtQuantity:      l.Quantity,
			RefundedQuantity:    refunded,
			RefundableQty:       refundable,
			Redeemed:            l.Edges.Redemption != nil,
		})
	}

	return &RefundView{
		Order:                  ord,
		Lines:                  viewLines,
		Deposits:               deposits,
		Refunds:                refundsList,
		RefundableBalanceCents: refundableBalanceCents(collectedCents(ord), succeededSum, outstandingDepositCents(ord, refundedQty)),
	}, nil
}

func (s *refundService) RefundedSummary(ctx context.Context, orderID string) (RefundedSummary, error) {
	sum, byLine, err := s.refundedSoFar(ctx, orderID)
	if err != nil {
		return RefundedSummary{}, err
	}
	return RefundedSummary{TotalCents: sum, ByLine: byLine}, nil
}

func (s *refundService) ListRefunds(ctx context.Context, limit, offset int) ([]*ent.Refund, error) {
	return s.refunds.List(ctx, limit, offset)
}

func (s *refundService) LookupForPOS(ctx context.Context, query string) ([]*ent.Order, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty lookup query")
	}
	if ord, ok := s.lookupByToken(ctx, query); ok {
		return []*ent.Order{ord}, nil
	}
	return s.orders.FindByIDPrefix(ctx, query, posLookupLimit)
}

// lookupByToken decodes a signed pickup token to its order. A non-token query
// (e.g. a short id prefix) fails verification and falls through to prefix search.
func (s *refundService) lookupByToken(ctx context.Context, token string) (*ent.Order, bool) {
	if s.qrKeys == nil {
		return nil, false
	}
	pub, err := s.qrKeys.TenantPublicKey(ctx)
	if err != nil {
		return nil, false
	}
	payload, err := qrsign.Verify(pub, token)
	if err != nil {
		return nil, false
	}
	ord, err := s.orders.GetByIDWithRelations(ctx, payload.OrderID)
	if err != nil {
		return nil, false
	}
	return ord, true
}

func (s *refundService) RefundLines(ctx context.Context, in RefundLinesInput) (*ent.Refund, error) {
	if len(in.Lines) == 0 {
		return nil, fmt.Errorf("no refund lines")
	}
	ord, err := s.loadRefundableOrder(ctx, in.OrderID)
	if err != nil {
		return nil, err
	}
	succeededSum, refundedQty, err := s.refundedSoFar(ctx, ord.ID)
	if err != nil {
		return nil, err
	}
	lineIndex := indexOrderLines(ord)

	var lineSpecs []refundLineSpec
	var amount int64
	added := make(map[string]int)
	for _, rl := range in.Lines {
		line, ok := lineIndex[rl.OrderLineID]
		if !ok {
			return nil, fmt.Errorf("order line %s does not belong to order %s", rl.OrderLineID, ord.ID)
		}
		if line.LineType == orderline.LineTypeDeposit {
			return nil, fmt.Errorf("deposit line %s must be returned via the deposit-return endpoint", rl.OrderLineID)
		}
		if err := validateRefundLineQty(line.Quantity, refundedQty[rl.OrderLineID]+added[rl.OrderLineID], rl.Quantity); err != nil {
			return nil, err
		}
		lineAmount := refundUnitPrice(ord, line) * int64(rl.Quantity)
		amount += lineAmount
		added[rl.OrderLineID] += rl.Quantity
		lineSpecs = append(lineSpecs, refundLineSpec{
			orderLineID: line.ID,
			productID:   line.ProductID,
			variantID:   line.VariantID,
			productType: productTypeOf(line),
			lineType:    line.LineType,
			quantity:    rl.Quantity,
			amountCents: lineAmount,
		})
	}

	balance := refundableBalanceCents(collectedCents(ord), succeededSum, outstandingDepositCents(ord, refundedQty))
	if amount < 0 {
		return nil, fmt.Errorf("refund amount must not be negative")
	}
	if amount > balance {
		return nil, fmt.Errorf("refund amount %d exceeds refundable balance %d", amount, balance)
	}

	// Refunding a product line releases its linked deposit (same money movement);
	// each release is capped at that deposit line's outstanding balance and draws
	// from the deposit balance, not the product-refundable balance above.
	for _, ds := range linkedDepositSpecs(ord, lineSpecs, refundedQty) {
		amount += ds.amountCents
		added[ds.orderLineID] += ds.quantity
		lineSpecs = append(lineSpecs, ds)
	}

	newRefundedQty := mergeQty(refundedQty, added)
	newStatus := orderStatusAfterLineRefund(collectedCents(ord), succeededSum+amount, unitsOutstanding(ord, newRefundedQty))
	return s.commitRefund(ctx, ord, amount, refund.KindLine, in.Reason, in.Actor, lineSpecs, newStatus, qrReissue, newRefundedQty)
}

func (s *refundService) RefundArbitrary(ctx context.Context, in RefundArbitraryInput) (*ent.Refund, error) {
	if strings.TrimSpace(in.Reason) == "" {
		return nil, fmt.Errorf("arbitrary refund requires a reason")
	}
	ord, err := s.loadRefundableOrder(ctx, in.OrderID)
	if err != nil {
		return nil, err
	}
	succeededSum, refundedQty, err := s.refundedSoFar(ctx, ord.ID)
	if err != nil {
		return nil, err
	}
	balance := refundableBalanceCents(collectedCents(ord), succeededSum, outstandingDepositCents(ord, refundedQty))
	amount := cappedArbitraryAmount(in.AmountCents, balance)
	if amount <= 0 {
		return nil, fmt.Errorf("no refundable balance")
	}
	reason := in.Reason
	newStatus := orderStatusAfterLineRefund(collectedCents(ord), succeededSum+amount, unitsOutstanding(ord, refundedQty))
	return s.commitRefund(ctx, ord, amount, refund.KindArbitrary, &reason, in.Actor, nil, newStatus, qrKeep, nil)
}

func (s *refundService) CancelOrder(ctx context.Context, in CancelOrderInput) (*ent.Refund, error) {
	ord, err := s.loadRefundableOrder(ctx, in.OrderID)
	if err != nil {
		return nil, err
	}
	succeededSum, refundedQty, err := s.refundedSoFar(ctx, ord.ID)
	if err != nil {
		return nil, err
	}

	var lineSpecs []refundLineSpec
	added := make(map[string]int)
	for _, l := range ord.Edges.Lines {
		// Deposit lines are settled by settleDepositsOnFullRefund (their money is
		// already part of the full remaining balance below); including them here
		// would double-restore the linked product's inventory.
		if l.LineType == orderline.LineTypeDeposit {
			continue
		}
		remaining := l.Quantity - refundedQty[l.ID]
		if remaining <= 0 {
			continue
		}
		added[l.ID] += remaining
		lineSpecs = append(lineSpecs, refundLineSpec{
			orderLineID: l.ID,
			productID:   l.ProductID,
			variantID:   l.VariantID,
			productType: productTypeOf(l),
			lineType:    l.LineType,
			quantity:    remaining,
			amountCents: refundUnitPrice(ord, l) * int64(remaining),
		})
	}

	// Cancellation refunds everything still owed — products and outstanding deposits.
	amount := collectedCents(ord) - succeededSum
	if amount < 0 {
		amount = 0
	}
	return s.commitRefund(ctx, ord, amount, refund.KindLine, in.Reason, in.Actor, lineSpecs, order.StatusCancelled, qrClear, mergeQty(refundedQty, added))
}

// ReturnDeposit refunds outstanding deposit units for the given deposit lines.
// It mirrors RefundLines but is the only path that may touch deposit lines, and
// it never re-issues the pickup token (deposits aren't pickup items).
func (s *refundService) ReturnDeposit(ctx context.Context, in DepositReturnInput) (*ent.Refund, error) {
	if len(in.Lines) == 0 {
		return nil, fmt.Errorf("no deposit return lines")
	}
	ord, err := s.loadRefundableOrder(ctx, in.OrderID)
	if err != nil {
		return nil, err
	}
	if in.Token != nil {
		if err := s.verifyPickupToken(ctx, *in.Token, ord.ID); err != nil {
			return nil, err
		}
	}
	succeededSum, refundedQty, err := s.refundedSoFar(ctx, ord.ID)
	if err != nil {
		return nil, err
	}
	lineIndex := indexOrderLines(ord)

	var lineSpecs []refundLineSpec
	var amount int64
	added := make(map[string]int)
	for _, rl := range in.Lines {
		line, ok := lineIndex[rl.OrderLineID]
		if !ok {
			return nil, fmt.Errorf("order line %s does not belong to order %s", rl.OrderLineID, ord.ID)
		}
		if line.LineType != orderline.LineTypeDeposit {
			return nil, fmt.Errorf("order line %s is not a deposit line", rl.OrderLineID)
		}
		if rl.Quantity <= 0 {
			return nil, fmt.Errorf("return quantity must be positive")
		}
		outstanding := line.Quantity - refundedQty[rl.OrderLineID] - added[rl.OrderLineID]
		if rl.Quantity > outstanding {
			return nil, fmt.Errorf("return quantity %d exceeds %d outstanding", rl.Quantity, outstanding)
		}
		lineAmount := refundUnitPrice(ord, line) * int64(rl.Quantity)
		amount += lineAmount
		added[rl.OrderLineID] += rl.Quantity
		lineSpecs = append(lineSpecs, refundLineSpec{
			orderLineID: line.ID,
			productID:   line.ProductID,
			productType: productTypeOf(line),
			lineType:    orderline.LineTypeDeposit,
			quantity:    rl.Quantity,
			amountCents: lineAmount,
		})
	}
	if amount < 0 {
		return nil, fmt.Errorf("deposit return amount must not be negative")
	}

	newRefundedQty := mergeQty(refundedQty, added)
	newStatus := orderStatusAfterLineRefund(collectedCents(ord), succeededSum+amount, unitsOutstanding(ord, newRefundedQty))
	return s.commitRefund(ctx, ord, amount, refund.KindLine, depositReturnReason(in.Reason), in.Actor, lineSpecs, newStatus, qrKeep, newRefundedQty)
}

// depositReturnReason defaults a deposit return's reason so the admin refunds list
// doesn't fill with blank rows; a caller-supplied reason still wins.
func depositReturnReason(reason *string) *string {
	if reason == nil || strings.TrimSpace(*reason) == "" {
		def := "Depot-Rückgabe"
		return &def
	}
	return reason
}

// verifyPickupToken checks a scanned pickup token's signature against the tenant
// key and that it resolves to the expected order — rejecting forged, wrong-tenant
// (signed by another key), or mismatched tokens.
func (s *refundService) verifyPickupToken(ctx context.Context, token, orderID string) error {
	if s.qrKeys == nil {
		return fmt.Errorf("qr verification unavailable")
	}
	pub, err := s.qrKeys.TenantPublicKey(ctx)
	if err != nil {
		return fmt.Errorf("qr verification unavailable: %w", err)
	}
	payload, err := qrsign.Verify(pub, token)
	if err != nil {
		return fmt.Errorf("invalid pickup token: %w", err)
	}
	if payload.OrderID != orderID {
		return fmt.Errorf("pickup token does not match order")
	}
	return nil
}

func (s *refundService) ReconcileFromWebhook(ctx context.Context, result *payment.WebhookResult) error {
	if result == nil || !result.IsRefund {
		return nil
	}
	// Lock first to serialize behind an in-flight synchronous refund (which holds the
	// order FOR UPDATE across its slow Payrexx call); otherwise we'd read the pre-commit
	// ledger and record a duplicate reconcile row.
	if err := s.orders.LockByID(ctx, result.ReferenceID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return err
	}
	ord, err := s.orders.GetByID(ctx, result.ReferenceID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return err
	}
	if ord.Status != order.StatusPaid && ord.Status != order.StatusPartiallyRefunded {
		return nil
	}
	succeededSum, err := s.refunds.SumSucceededByOrder(ctx, ord.ID)
	if err != nil {
		return err
	}

	// Reconcile against the cumulative refunded amount, not the (unchanged) charge
	// amount, so a partial refund isn't escalated to a full one — recording only
	// the portion our ledger doesn't already hold.
	providerRefunded := int64(result.RefundedAmountCents)
	if providerRefunded <= 0 {
		// Older payloads omit refundedAmount: only a full-refund status can be sized
		// (== total); a partial one can't, so skip it.
		if !result.IsFullRefund {
			s.logger.Warn("payrexx webhook: partial refund without refundedAmount; skipping reconcile",
				zap.String("orderId", ord.ID))
			return nil
		}
		providerRefunded = ord.TotalCents
	}

	amount := reconcilableRefundDelta(ord.TotalCents, providerRefunded, succeededSum)
	if amount <= 0 {
		return nil
	}
	reason := "reconciled from payrexx webhook"
	txnID := result.TransactionID
	if _, err := s.refunds.CreateRefund(ctx, repository.CreateRefundParams{
		OrderID:              ord.ID,
		AmountCents:          amount,
		Kind:                 refund.KindArbitrary,
		Method:               refund.MethodTwint,
		Reason:               &reason,
		ActorType:            refund.ActorTypeAdmin,
		ActorID:              "system",
		PayrexxTransactionID: &txnID,
		Status:               refund.StatusSucceeded,
	}); err != nil {
		return err
	}
	return s.setOrderStatus(ctx, ord, orderStatusAfterRefund(ord.TotalCents, succeededSum+amount))
}

// reconcilableRefundDelta is the not-yet-recorded portion of a provider's
// cumulative refund: min(providerRefunded, orderTotal) - alreadyRecorded, floored
// at zero — so it can't escalate a partial into a full refund or double-count.
func reconcilableRefundDelta(orderTotal, providerRefunded, alreadyRecorded int64) int64 {
	if providerRefunded > orderTotal {
		providerRefunded = orderTotal
	}
	delta := providerRefunded - alreadyRecorded
	if delta <= 0 {
		return 0
	}
	return delta
}

// commitRefund performs the write side shared by line, arbitrary and cancel
// refunds: insert the (pending) refund + lines, restore inventory, transition the
// order, re-issue the pickup token, and — last, so its failure rolls the whole
// request tx back — issue the provider refund before marking the row succeeded.
func (s *refundService) commitRefund(
	ctx context.Context,
	ord *ent.Order,
	amountCents int64,
	kind refund.Kind,
	reason *string,
	actor RefundActor,
	lineSpecs []refundLineSpec,
	newStatus order.Status,
	qr qrPlan,
	newRefundedQty map[string]int,
) (*ent.Refund, error) {
	method := refundMethodFor(ord.PayrexxTransactionID)

	created, err := s.refunds.CreateRefund(ctx, repository.CreateRefundParams{
		OrderID:              ord.ID,
		AmountCents:          amountCents,
		Kind:                 kind,
		Method:               method,
		Reason:               reason,
		ActorType:            actor.Type,
		ActorID:              actor.ID,
		DeviceID:             actor.DeviceID,
		PayrexxTransactionID: ord.PayrexxTransactionID,
		Status:               refund.StatusPending,
	})
	if err != nil {
		return nil, err
	}

	rlParams := make([]repository.CreateRefundLineParams, 0, len(lineSpecs))
	for _, ls := range lineSpecs {
		rlParams = append(rlParams, repository.CreateRefundLineParams{
			RefundID:    created.ID,
			OrderLineID: ls.orderLineID,
			Quantity:    ls.quantity,
			AmountCents: ls.amountCents,
		})
	}
	if _, err := s.refunds.CreateRefundLines(ctx, rlParams); err != nil {
		return nil, err
	}

	if err := s.restoreInventory(ctx, ord.ID, lineSpecs); err != nil {
		return nil, err
	}

	if err := s.setOrderStatus(ctx, ord, newStatus); err != nil {
		return nil, err
	}
	if newStatus == order.StatusRefunded || newStatus == order.StatusCancelled {
		if err := s.settleDepositsOnFullRefund(ctx, ord, created.ID, newRefundedQty); err != nil {
			return nil, err
		}
	}

	switch qr {
	case qrReissue:
		token, err := s.reissuePickupToken(ctx, ord, newRefundedQty)
		if err != nil {
			return nil, err
		}
		if err := s.orders.SetQRPayload(ctx, ord.ID, token); err != nil {
			return nil, err
		}
	case qrClear:
		if err := s.orders.ClearQRPayload(ctx, ord.ID); err != nil {
			return nil, err
		}
	case qrKeep:
	}

	if method == refund.MethodTwint && amountCents > 0 {
		if ord.PayrexxTransactionID == nil {
			return nil, fmt.Errorf("twint refund requires a payrexx transaction id")
		}
		if s.providers == nil {
			return nil, fmt.Errorf("payment provider not configured")
		}
		provider, err := s.providers.For(ctx)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrProviderRefund, err)
		}
		if err := provider.Refund(ctx, *ord.PayrexxTransactionID, int(amountCents)); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrProviderRefund, err)
		}
	}

	if err := s.refunds.SetStatus(ctx, created.ID, refund.StatusSucceeded); err != nil {
		return nil, err
	}
	created.Status = refund.StatusSucceeded
	return created, nil
}

func (s *refundService) loadRefundableOrder(ctx context.Context, orderID string) (*ent.Order, error) {
	// Serialize concurrent refunds of the same order: hold a FOR UPDATE lock for
	// the rest of the request tx so a second refund reads this one's committed
	// state (and its over-refund guard) rather than racing it into a double payment.
	if err := s.orders.LockByID(ctx, orderID); err != nil {
		return nil, err
	}
	ord, err := s.orders.GetByIDWithRelations(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if ord.Status != order.StatusPaid && ord.Status != order.StatusPartiallyRefunded {
		return nil, fmt.Errorf("order %s is not refundable (status %s)", orderID, ord.Status)
	}
	return ord, nil
}

func (s *refundService) refundedSoFar(ctx context.Context, orderID string) (int64, map[string]int, error) {
	sum, err := s.refunds.SumSucceededByOrder(ctx, orderID)
	if err != nil {
		return 0, nil, err
	}
	qty, err := s.refunds.SumSucceededLineQtyByOrder(ctx, orderID)
	if err != nil {
		return 0, nil, err
	}
	return sum, qty, nil
}

func (s *refundService) setOrderStatus(ctx context.Context, ord *ent.Order, newStatus order.Status) error {
	if !isValidStatusTransition(ord.Status, newStatus) {
		return fmt.Errorf("invalid status transition from %s to %s", ord.Status, newStatus)
	}
	return s.orders.UpdateStatus(ctx, ord.ID, newStatus)
}

func (s *refundService) reissuePickupToken(ctx context.Context, ord *ent.Order, newRefundedQty map[string]int) (string, error) {
	tenantID, ok := repository.TenantIDFromContext(ctx)
	if !ok {
		return "", fmt.Errorf("qr reissue: no tenant in context")
	}
	lines := remainingPickupLines(flattenRedeemableLines(ord), newRefundedQty)
	return signPickupToken(ctx, s.qrKeys, tenantID, ord.ID, lines)
}

func (s *refundService) restoreInventory(ctx context.Context, orderID string, lineSpecs []refundLineSpec) error {
	var entries []repository.InventoryLedgerCreateParams
	for _, ls := range lineSpecs {
		if ls.quantity <= 0 || ls.lineType == orderline.LineTypeDeposit || ls.productType != product.TypeSimple {
			continue
		}
		olID := ls.orderLineID
		entries = append(entries, repository.InventoryLedgerCreateParams{
			ProductID:   ls.productID,
			Delta:       ls.quantity,
			Reason:      inventoryledger.ReasonRefund,
			OrderID:     &orderID,
			OrderLineID: &olID,
			VariantID:   ls.variantID,
		})
	}
	if len(entries) == 0 {
		return nil
	}
	if _, err := s.inventory.CreateMany(ctx, entries); err != nil {
		return err
	}
	s.publishInventoryUpdates(ctx, entries)
	return nil
}

func (s *refundService) publishInventoryUpdates(ctx context.Context, entries []repository.InventoryLedgerCreateParams) {
	if s.inventoryHub == nil {
		return
	}
	type stockKey struct{ productID, variantID string }
	keys := make([]stockKey, 0, len(entries))
	deltaByKey := make(map[stockKey]int)
	productIDs := make([]string, 0, len(entries))
	seenProducts := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		key := stockKey{productID: e.ProductID}
		if e.VariantID != nil {
			key.variantID = *e.VariantID
		}
		if _, seen := deltaByKey[key]; !seen {
			keys = append(keys, key)
		}
		deltaByKey[key] += e.Delta
		if _, seen := seenProducts[e.ProductID]; !seen {
			seenProducts[e.ProductID] = struct{}{}
			productIDs = append(productIDs, e.ProductID)
		}
	}
	stocks, err := s.inventory.GetCurrentStockBatch(ctx, productIDs)
	if err != nil {
		return
	}
	variantStocks, err := s.inventory.GetCurrentStockByVariantBatch(ctx, productIDs)
	if err != nil {
		return
	}
	tenantID, _ := repository.TenantIDFromContext(ctx)
	now := time.Now()
	for _, key := range keys {
		update := inventory.Update{
			TenantID:  tenantID,
			ProductID: key.productID,
			NewStock:  stocks[key.productID],
			Delta:     deltaByKey[key],
			Timestamp: now,
		}
		if key.variantID != "" {
			vid := key.variantID
			update.VariantID = &vid
			update.NewStock = variantStocks[vid]
		}
		s.inventoryHub.Publish(ctx, update)
	}
}

// settleDepositsOnFullRefund records any deposit units still outstanding against
// this refund when the order is fully refunded or cancelled, so a later return
// scan finds nothing. newRefundedQty already reflects deposits released by the
// current refund, so settled deposits are never double-counted.
func (s *refundService) settleDepositsOnFullRefund(ctx context.Context, ord *ent.Order, refundID string, newRefundedQty map[string]int) error {
	var params []repository.CreateRefundLineParams
	for _, l := range ord.Edges.Lines {
		if l.LineType != orderline.LineTypeDeposit {
			continue
		}
		outstanding := l.Quantity - newRefundedQty[l.ID]
		if outstanding <= 0 {
			continue
		}
		params = append(params, repository.CreateRefundLineParams{
			RefundID:    refundID,
			OrderLineID: l.ID,
			Quantity:    outstanding,
			AmountCents: refundUnitPrice(ord, l) * int64(outstanding),
		})
	}
	if len(params) == 0 {
		return nil
	}
	_, err := s.refunds.CreateRefundLines(ctx, params)
	return err
}

// --- pure helpers (unit-tested without a database) ---

type refundLineSpec struct {
	orderLineID string
	productID   string
	variantID   *string
	productType product.Type
	lineType    orderline.LineType
	quantity    int
	amountCents int64
}

// outstandingDepositCents is the deposit balance still owed on an order, held out
// of the product-refundable balance: per deposit line, units not yet returned
// (incl. deposits released by a product-line refund) times the per-unit deposit.
func outstandingDepositCents(ord *ent.Order, refundedQty map[string]int) int64 {
	var sum int64
	for _, l := range ord.Edges.Lines {
		if l.LineType != orderline.LineTypeDeposit {
			continue
		}
		outstanding := l.Quantity - refundedQty[l.ID]
		if outstanding > 0 {
			sum += int64(outstanding) * refundUnitPrice(ord, l)
		}
	}
	return sum
}

// linkedDepositSpecs returns the deposit refund specs released alongside the
// given refunded product lines: each refunded product unit refunds its sibling
// deposit unit, capped at that deposit line's outstanding balance.
func linkedDepositSpecs(ord *ent.Order, productSpecs []refundLineSpec, refundedQty map[string]int) []refundLineSpec {
	depositByParent := make(map[string]*ent.OrderLine)
	for _, l := range ord.Edges.Lines {
		if l.LineType == orderline.LineTypeDeposit && l.ParentLineID != nil {
			depositByParent[*l.ParentLineID] = l
		}
	}
	released := make(map[string]int)
	var out []refundLineSpec
	for _, ps := range productSpecs {
		dep, ok := depositByParent[ps.orderLineID]
		if !ok {
			continue
		}
		// Cap against the deposit's outstanding balance, less anything already
		// released for it in this same call — a product line id may legally repeat
		// across in.Lines, and a deposit line may have fewer units than its parent.
		release := ps.quantity
		if outstanding := dep.Quantity - refundedQty[dep.ID] - released[dep.ID]; release > outstanding {
			release = outstanding
		}
		if release <= 0 {
			continue
		}
		released[dep.ID] += release
		out = append(out, refundLineSpec{
			orderLineID: dep.ID,
			productID:   dep.ProductID,
			productType: productTypeOf(dep),
			lineType:    orderline.LineTypeDeposit,
			quantity:    release,
			amountCents: refundUnitPrice(ord, dep) * int64(release),
		})
	}
	return out
}

type qrPlan int

const (
	qrReissue qrPlan = iota // re-sign from the remaining un-redeemed lines
	qrClear                 // drop the token (full cancellation)
	qrKeep                  // leave the token unchanged (money-only refund)
)

type redeemableLine struct {
	OrderLineID string
	ProductID   string
	VariantID   string
	LineType    orderline.LineType
	HasChildren bool
	Redeemed    bool
	BoughtQty   int
}

// A comp keeps its nominal total for receipts and reporting but collected
// nothing — not even the deposit, which the POS never charges on a comp.
func gratisOrder(ord *ent.Order) bool {
	method, ok := primaryPaymentMethod(ord)
	return ok && isGratisMethod(method)
}

func collectedCents(ord *ent.Order) int64 {
	if gratisOrder(ord) {
		return 0
	}
	return ord.TotalCents
}

func refundUnitPrice(ord *ent.Order, line *ent.OrderLine) int64 {
	if gratisOrder(ord) {
		return 0
	}
	return line.UnitPriceCents
}

func unitsOutstanding(ord *ent.Order, refundedQty map[string]int) int {
	total := 0
	for _, l := range ord.Edges.Lines {
		if l.LineType == orderline.LineTypeDeposit || l.ParentLineID != nil {
			continue
		}
		if remaining := l.Quantity - refundedQty[l.ID]; remaining > 0 {
			total += remaining
		}
	}
	return total
}

func refundMethodFor(payrexxTransactionID *int) refund.Method {
	if payrexxTransactionID != nil {
		return refund.MethodTwint
	}
	return refund.MethodCash
}

func refundableBalanceCents(collected, succeededSum, outstandingDeposit int64) int64 {
	bal := collected - succeededSum - outstandingDeposit
	if bal < 0 {
		return 0
	}
	return bal
}

func orderStatusAfterRefund(collected, succeededAfter int64) order.Status {
	if succeededAfter >= collected {
		return order.StatusRefunded
	}
	return order.StatusPartiallyRefunded
}

// An order that collected nothing can't measure completion in money.
func orderStatusAfterLineRefund(collected, succeededAfter int64, unitsOut int) order.Status {
	if collected <= 0 && unitsOut > 0 {
		return order.StatusPartiallyRefunded
	}
	return orderStatusAfterRefund(collected, succeededAfter)
}

func cappedArbitraryAmount(requested, refundable int64) int64 {
	if requested > refundable {
		return refundable
	}
	return requested
}

func validateRefundLineQty(bought, alreadyRefunded, requested int) error {
	if requested <= 0 {
		return fmt.Errorf("refund quantity must be positive")
	}
	if alreadyRefunded+requested > bought {
		return fmt.Errorf("refund quantity %d exceeds %d remaining", requested, bought-alreadyRefunded)
	}
	return nil
}

// remainingPickupLines rebuilds the signed pickup set after refunds: redeemed
// lines were already handed over (money-only, excluded) and each remaining line's
// quantity is reduced by what has been refunded. A configured bundle parent
// carries no pickup item of its own; its components do.
func remainingPickupLines(lines []redeemableLine, refundedQty map[string]int) []qrsign.Line {
	out := make([]qrsign.Line, 0, len(lines))
	for _, l := range lines {
		if l.Redeemed {
			continue
		}
		switch l.LineType {
		case orderline.LineTypeSimple, orderline.LineTypeComponent:
		case orderline.LineTypeBundle:
			if l.HasChildren {
				continue
			}
		default:
			continue
		}
		remaining := l.BoughtQty - refundedQty[l.OrderLineID]
		if remaining <= 0 {
			continue
		}
		out = append(out, qrsign.Line{ProductID: l.ProductID, VariantID: l.VariantID, Quantity: remaining})
	}
	return out
}

// flattenRedeemableLines projects an order's loaded lines (parents and children
// share order_id, so Edges.Lines is already flat) into the redeemable view the
// pickup token is built from.
func flattenRedeemableLines(ord *ent.Order) []redeemableLine {
	hasChildren := make(map[string]bool)
	for _, l := range ord.Edges.Lines {
		if l.ParentLineID != nil {
			hasChildren[*l.ParentLineID] = true
		}
	}
	out := make([]redeemableLine, 0, len(ord.Edges.Lines))
	for _, l := range ord.Edges.Lines {
		out = append(out, redeemableLine{
			OrderLineID: l.ID,
			ProductID:   l.ProductID,
			VariantID:   safeStr(l.VariantID),
			LineType:    l.LineType,
			HasChildren: hasChildren[l.ID],
			Redeemed:    l.Edges.Redemption != nil,
			BoughtQty:   l.Quantity,
		})
	}
	return out
}

func indexOrderLines(ord *ent.Order) map[string]*ent.OrderLine {
	out := make(map[string]*ent.OrderLine, len(ord.Edges.Lines))
	for _, l := range ord.Edges.Lines {
		out[l.ID] = l
	}
	return out
}

func productTypeOf(line *ent.OrderLine) product.Type {
	if line.Edges.Product == nil {
		return ""
	}
	return line.Edges.Product.Type
}

func mergeQty(base, add map[string]int) map[string]int {
	out := make(map[string]int, len(base))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range add {
		out[k] += v
	}
	return out
}
