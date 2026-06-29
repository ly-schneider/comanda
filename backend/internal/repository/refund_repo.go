package repository

import (
	"context"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/refund"
	"backend/internal/generated/ent/refundline"
)

type RefundRepository interface {
	CreateRefund(ctx context.Context, params CreateRefundParams) (*ent.Refund, error)
	CreateRefundLines(ctx context.Context, lines []CreateRefundLineParams) ([]*ent.RefundLine, error)
	SetStatus(ctx context.Context, id string, status refund.Status) error
	SumSucceededByOrder(ctx context.Context, orderID string) (int64, error)
	SumSucceededLineQtyByOrder(ctx context.Context, orderID string) (map[string]int, error)
	ListByOrder(ctx context.Context, orderID string) ([]*ent.Refund, error)
	List(ctx context.Context, limit, offset int) ([]*ent.Refund, error)
}

type CreateRefundParams struct {
	OrderID              string
	AmountCents          int64
	Kind                 refund.Kind
	Method               refund.Method
	Reason               *string
	ActorType            refund.ActorType
	ActorID              string
	DeviceID             *string
	PayrexxTransactionID *int
	Status               refund.Status
}

type CreateRefundLineParams struct {
	RefundID    string
	OrderLineID string
	Quantity    int
	AmountCents int64
}

type refundRepo struct {
	client *ent.Client
}

func NewRefundRepository(client *ent.Client) RefundRepository {
	return &refundRepo{client: client}
}

func (r *refundRepo) ec(ctx context.Context) *ent.Client {
	return ClientFromContext(ctx, r.client)
}

func (r *refundRepo) CreateRefund(ctx context.Context, params CreateRefundParams) (*ent.Refund, error) {
	builder := r.ec(ctx).Refund.Create().
		SetOrderID(params.OrderID).
		SetAmountCents(params.AmountCents).
		SetKind(params.Kind).
		SetMethod(params.Method).
		SetActorType(params.ActorType).
		SetActorID(params.ActorID).
		SetStatus(params.Status)
	if params.Reason != nil {
		builder.SetReason(*params.Reason)
	}
	if params.DeviceID != nil {
		builder.SetDeviceID(*params.DeviceID)
	}
	if params.PayrexxTransactionID != nil {
		builder.SetPayrexxTransactionID(*params.PayrexxTransactionID)
	}
	created, err := builder.Save(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return created, nil
}

func (r *refundRepo) CreateRefundLines(ctx context.Context, lines []CreateRefundLineParams) ([]*ent.RefundLine, error) {
	if len(lines) == 0 {
		return nil, nil
	}
	builders := make([]*ent.RefundLineCreate, len(lines))
	for i, l := range lines {
		builders[i] = r.ec(ctx).RefundLine.Create().
			SetRefundID(l.RefundID).
			SetOrderLineID(l.OrderLineID).
			SetQuantity(l.Quantity).
			SetAmountCents(l.AmountCents)
	}
	created, err := r.ec(ctx).RefundLine.CreateBulk(builders...).Save(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return created, nil
}

func (r *refundRepo) SetStatus(ctx context.Context, id string, status refund.Status) error {
	_, err := r.ec(ctx).Refund.UpdateOneID(id).
		SetStatus(status).
		Save(ctx)
	return translateError(err)
}

// SumSucceededByOrder totals the amount of every succeeded refund for an order.
// Refund counts per order are small, so the rows are summed in Go rather than
// pushing a NUMERIC aggregate through the driver.
func (r *refundRepo) SumSucceededByOrder(ctx context.Context, orderID string) (int64, error) {
	rows, err := r.ec(ctx).Refund.Query().
		Where(
			refund.OrderIDEQ(orderID),
			refund.StatusEQ(refund.StatusSucceeded),
		).
		All(ctx)
	if err != nil {
		return 0, translateError(err)
	}
	var sum int64
	for _, x := range rows {
		sum += x.AmountCents
	}
	return sum, nil
}

// SumSucceededLineQtyByOrder returns, per order_line_id, the quantity already
// refunded across this order's succeeded refunds.
func (r *refundRepo) SumSucceededLineQtyByOrder(ctx context.Context, orderID string) (map[string]int, error) {
	rows, err := r.ec(ctx).RefundLine.Query().
		Where(refundline.HasRefundWith(
			refund.OrderIDEQ(orderID),
			refund.StatusEQ(refund.StatusSucceeded),
		)).
		All(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	out := make(map[string]int)
	for _, l := range rows {
		out[l.OrderLineID] += l.Quantity
	}
	return out, nil
}

func (r *refundRepo) ListByOrder(ctx context.Context, orderID string) ([]*ent.Refund, error) {
	rows, err := r.ec(ctx).Refund.Query().
		Where(refund.OrderIDEQ(orderID)).
		WithRefundLines().
		Order(refund.ByCreatedAt(entDescOpt())).
		All(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return rows, nil
}

func (r *refundRepo) List(ctx context.Context, limit, offset int) ([]*ent.Refund, error) {
	rows, err := r.ec(ctx).Refund.Query().
		WithRefundLines().
		Order(refund.ByCreatedAt(entDescOpt())).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	return rows, nil
}
