package club100

import (
	"context"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/club100redemption"
	"backend/internal/generated/ent/orderline"
	"backend/internal/generated/ent/predicate"
	"backend/internal/generated/ent/refund"
	"backend/internal/generated/ent/refundline"
	"backend/internal/repository"
)

type RedemptionRepository interface {
	Create(ctx context.Context, elvantoPersonID, elvantoPersonName string, orderID string, quantity int) (*ent.Club100Redemption, error)
	// both totals are net of what a refund or cancellation gave back
	GetTotalRedemptions(ctx context.Context, elvantoPersonID string) (int, error)
	GetTotalRedemptionsBatch(ctx context.Context, elvantoPersonIDs []string) (map[string]int, error)
	GetByOrderID(ctx context.Context, orderID string) ([]*ent.Club100Redemption, error)
}

type redemptionRepo struct {
	client *ent.Client
}

func NewRedemptionRepository(client *ent.Client) RedemptionRepository {
	return &redemptionRepo{client: client}
}

func (r *redemptionRepo) ec(ctx context.Context) *ent.Client {
	return repository.ClientFromContext(ctx, r.client)
}

func (r *redemptionRepo) Create(ctx context.Context, elvantoPersonID, elvantoPersonName string, orderID string, quantity int) (*ent.Club100Redemption, error) {
	e, err := r.ec(ctx).Club100Redemption.Create().
		SetElvantoPersonID(elvantoPersonID).
		SetElvantoPersonName(elvantoPersonName).
		SetOrderID(orderID).
		SetFreeProductQuantity(quantity).
		Save(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return e, nil
}

func (r *redemptionRepo) GetTotalRedemptions(ctx context.Context, elvantoPersonID string) (int, error) {
	totals, err := r.consumedToday(ctx, club100redemption.ElvantoPersonIDEQ(elvantoPersonID))
	if err != nil {
		return 0, err
	}
	return totals[elvantoPersonID], nil
}

func (r *redemptionRepo) GetTotalRedemptionsBatch(ctx context.Context, elvantoPersonIDs []string) (map[string]int, error) {
	if len(elvantoPersonIDs) == 0 {
		return map[string]int{}, nil
	}
	return r.consumedToday(ctx, club100redemption.ElvantoPersonIDIn(elvantoPersonIDs...))
}

// The release is derived from the refund ledger, not stored, so orders refunded
// before this existed count correctly too.
func (r *redemptionRepo) consumedToday(ctx context.Context, who predicate.Club100Redemption) (map[string]int, error) {
	dayStart, dayEnd := zurichDayBounds()

	rows, err := r.ec(ctx).Club100Redemption.Query().
		Where(
			who,
			club100redemption.CreatedAtGTE(dayStart),
			club100redemption.CreatedAtLT(dayEnd),
		).
		Order(club100redemption.ByCreatedAt()).
		All(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	if len(rows) == 0 {
		return map[string]int{}, nil
	}

	returned, err := r.returnedUnits(ctx, orderIDsOf(rows))
	if err != nil {
		return nil, err
	}
	return netRedemptions(rows, returned), nil
}

// Top-level lines only: a deposit return moves deposit money, not the comped
// product, and a cancellation writes lines for a bundle's components too.
func (r *redemptionRepo) returnedUnits(ctx context.Context, orderIDs []string) (map[string]int, error) {
	rows, err := r.ec(ctx).RefundLine.Query().
		Where(
			refundline.HasRefundWith(
				refund.OrderIDIn(orderIDs...),
				refund.StatusEQ(refund.StatusSucceeded),
			),
			refundline.HasOrderLineWith(
				orderline.LineTypeNEQ(orderline.LineTypeDeposit),
				orderline.ParentLineIDIsNil(),
			),
		).
		WithRefund().
		All(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}

	out := make(map[string]int, len(orderIDs))
	for _, l := range rows {
		if l.Edges.Refund == nil {
			continue
		}
		out[l.Edges.Refund.OrderID] += l.Quantity
	}
	return out, nil
}

func (r *redemptionRepo) GetByOrderID(ctx context.Context, orderID string) ([]*ent.Club100Redemption, error) {
	rows, err := r.ec(ctx).Club100Redemption.Query().
		Where(club100redemption.OrderIDEQ(orderID)).
		All(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return rows, nil
}

func orderIDsOf(rows []*ent.Club100Redemption) []string {
	seen := make(map[string]bool, len(rows))
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if seen[row.OrderID] {
			continue
		}
		seen[row.OrderID] = true
		out = append(out, row.OrderID)
	}
	return out
}

// An order can return more units than it comped (the rest of the basket was
// paid for), so a row never gives back more than it took.
func netRedemptions(rows []*ent.Club100Redemption, returnedByOrder map[string]int) map[string]int {
	budget := make(map[string]int, len(returnedByOrder))
	for orderID, qty := range returnedByOrder {
		budget[orderID] = qty
	}

	out := make(map[string]int, len(rows))
	for _, row := range rows {
		released := min(budget[row.OrderID], row.FreeProductQuantity)
		budget[row.OrderID] -= released
		out[row.ElvantoPersonID] += row.FreeProductQuantity - released
	}
	return out
}

func zurichDayBounds() (time.Time, time.Time) {
	loc, _ := time.LoadLocation("Europe/Zurich")
	now := time.Now().In(loc)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	return dayStart, dayStart.AddDate(0, 0, 1)
}
