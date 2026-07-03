package repository

import (
	"context"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/qatestresult"
	"backend/internal/generated/ent/qatestrun"
)

type QaRunCounts struct {
	Success int
	Error   int
}

// QaRepository stores platform-plane manual-QA runs; the qa_* RLS policies
// restrict every access to platform-admin requests.
type QaRepository interface {
	CreateRun(ctx context.Context, name string, note *string) (*ent.QaTestRun, error)
	ListRuns(ctx context.Context) ([]*ent.QaTestRun, error)
	GetRun(ctx context.Context, id string) (*ent.QaTestRun, error)
	UpdateRun(ctx context.Context, id string, name, note *string) (*ent.QaTestRun, error)
	DeleteRun(ctx context.Context, id string) error
	ListResults(ctx context.Context, runID string) ([]*ent.QaTestResult, error)
	CountsByRun(ctx context.Context) (map[string]QaRunCounts, error)
	// UpsertResult full-replaces one test's (status, note); nil clears.
	UpsertResult(ctx context.Context, runID, testKey string, status *qatestresult.Status, note *string) (*ent.QaTestResult, error)
}

type qaRepo struct {
	client *ent.Client
}

func NewQaRepository(client *ent.Client) QaRepository {
	return &qaRepo{client: client}
}

func (r *qaRepo) ec(ctx context.Context) *ent.Client {
	return ClientFromContext(ctx, r.client)
}

func (r *qaRepo) CreateRun(ctx context.Context, name string, note *string) (*ent.QaTestRun, error) {
	created, err := r.ec(ctx).QaTestRun.Create().
		SetName(name).
		SetNillableNote(note).
		Save(ctx)
	return created, translateError(err)
}

func (r *qaRepo) ListRuns(ctx context.Context) ([]*ent.QaTestRun, error) {
	list, err := r.ec(ctx).QaTestRun.Query().
		Order(qatestrun.ByCreatedAt(entDescOpt())).
		All(ctx)
	return list, translateError(err)
}

func (r *qaRepo) GetRun(ctx context.Context, id string) (*ent.QaTestRun, error) {
	run, err := r.ec(ctx).QaTestRun.Get(ctx, id)
	return run, translateError(err)
}

func (r *qaRepo) UpdateRun(ctx context.Context, id string, name, note *string) (*ent.QaTestRun, error) {
	upd := r.ec(ctx).QaTestRun.UpdateOneID(id)
	if name != nil {
		upd.SetName(*name)
	}
	if note != nil {
		if *note == "" {
			upd.ClearNote()
		} else {
			upd.SetNote(*note)
		}
	}
	updated, err := upd.Save(ctx)
	return updated, translateError(err)
}

func (r *qaRepo) DeleteRun(ctx context.Context, id string) error {
	err := r.ec(ctx).QaTestRun.DeleteOneID(id).Exec(ctx)
	return translateError(err)
}

func (r *qaRepo) ListResults(ctx context.Context, runID string) ([]*ent.QaTestResult, error) {
	list, err := r.ec(ctx).QaTestResult.Query().
		Where(qatestresult.RunID(runID)).
		Order(qatestresult.ByTestKey()).
		All(ctx)
	return list, translateError(err)
}

func (r *qaRepo) CountsByRun(ctx context.Context) (map[string]QaRunCounts, error) {
	var rows []struct {
		RunID  string  `json:"run_id"`
		Status *string `json:"status"`
		Count  int     `json:"count"`
	}
	err := r.ec(ctx).QaTestResult.Query().
		GroupBy(qatestresult.FieldRunID, qatestresult.FieldStatus).
		Aggregate(ent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		return nil, translateError(err)
	}
	counts := make(map[string]QaRunCounts, len(rows))
	for _, row := range rows {
		c := counts[row.RunID]
		if row.Status != nil {
			switch qatestresult.Status(*row.Status) {
			case qatestresult.StatusSuccess:
				c.Success += row.Count
			case qatestresult.StatusError:
				c.Error += row.Count
			}
		}
		counts[row.RunID] = c
	}
	return counts, nil
}

func (r *qaRepo) UpsertResult(ctx context.Context, runID, testKey string, status *qatestresult.Status, note *string) (*ent.QaTestResult, error) {
	err := r.ec(ctx).QaTestResult.Create().
		SetRunID(runID).
		SetTestKey(testKey).
		SetNillableStatus(status).
		SetNillableNote(note).
		OnConflictColumns(qatestresult.FieldRunID, qatestresult.FieldTestKey).
		Update(func(u *ent.QaTestResultUpsert) {
			if status != nil {
				u.SetStatus(*status)
			} else {
				u.ClearStatus()
			}
			if note != nil {
				u.SetNote(*note)
			} else {
				u.ClearNote()
			}
			u.SetUpdatedAt(time.Now())
		}).
		Exec(ctx)
	if err != nil {
		return nil, translateError(err)
	}
	stored, err := r.ec(ctx).QaTestResult.Query().
		Where(qatestresult.RunID(runID), qatestresult.TestKey(testKey)).
		Only(ctx)
	return stored, translateError(err)
}
