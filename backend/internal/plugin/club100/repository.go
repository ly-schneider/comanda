package club100

import (
	"context"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/club100redemption"
	"backend/internal/repository"

	"entgo.io/ent/dialect/sql"
)

type RedemptionRepository interface {
	Create(ctx context.Context, elvantoPersonID, elvantoPersonName string, orderID string, quantity int) (*ent.Club100Redemption, error)
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
	dayStart, dayEnd := zurichDayBounds()

	rows, err := r.ec(ctx).Club100Redemption.Query().
		Where(
			club100redemption.ElvantoPersonIDEQ(elvantoPersonID),
			club100redemption.CreatedAtGTE(dayStart),
			club100redemption.CreatedAtLT(dayEnd),
		).
		All(ctx)
	if err != nil {
		return 0, repository.TranslateError(err)
	}
	total := 0
	for _, row := range rows {
		total += row.FreeProductQuantity
	}
	return total, nil
}

func (r *redemptionRepo) GetTotalRedemptionsBatch(ctx context.Context, elvantoPersonIDs []string) (map[string]int, error) {
	result := make(map[string]int, len(elvantoPersonIDs))
	if len(elvantoPersonIDs) == 0 {
		return result, nil
	}

	dayStart, dayEnd := zurichDayBounds()

	var rows []struct {
		ElvantoPersonID string `json:"elvanto_person_id"`
		Total           int    `json:"total"`
	}
	err := r.ec(ctx).Club100Redemption.Query().
		Where(
			club100redemption.ElvantoPersonIDIn(elvantoPersonIDs...),
			club100redemption.CreatedAtGTE(dayStart),
			club100redemption.CreatedAtLT(dayEnd),
		).
		Modify(func(s *sql.Selector) {
			s.Select(
				s.C(club100redemption.FieldElvantoPersonID),
				sql.As(sql.Sum(s.C(club100redemption.FieldFreeProductQuantity)), "total"),
			).GroupBy(s.C(club100redemption.FieldElvantoPersonID))
		}).
		Scan(ctx, &rows)
	if err != nil {
		return nil, repository.TranslateError(err)
	}

	for _, row := range rows {
		result[row.ElvantoPersonID] = row.Total
	}
	return result, nil
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

func zurichDayBounds() (time.Time, time.Time) {
	loc, _ := time.LoadLocation("Europe/Zurich")
	now := time.Now().In(loc)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	return dayStart, dayStart.AddDate(0, 0, 1)
}
