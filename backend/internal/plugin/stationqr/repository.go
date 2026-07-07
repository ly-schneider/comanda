package stationqr

import (
	"context"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/repository"
)

type EnrollCodeRepository interface {
	Get(ctx context.Context) (*ent.StationEnrollToken, error)
	Upsert(ctx context.Context, tokenEnc []byte, issuedBy string) (*ent.StationEnrollToken, error)
	SetDisabled(ctx context.Context, disabled bool) (*ent.StationEnrollToken, error)
}

type enrollCodeRepo struct {
	client *ent.Client
}

func NewEnrollCodeRepository(client *ent.Client) EnrollCodeRepository {
	return &enrollCodeRepo{client: client}
}

func (r *enrollCodeRepo) ec(ctx context.Context) *ent.Client {
	return repository.ClientFromContext(ctx, r.client)
}

func (r *enrollCodeRepo) Get(ctx context.Context) (*ent.StationEnrollToken, error) {
	e, err := r.ec(ctx).StationEnrollToken.Query().First(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return e, nil
}

func (r *enrollCodeRepo) Upsert(ctx context.Context, tokenEnc []byte, issuedBy string) (*ent.StationEnrollToken, error) {
	existing, err := r.ec(ctx).StationEnrollToken.Query().First(ctx)
	if err == nil {
		updated, uerr := r.ec(ctx).StationEnrollToken.UpdateOneID(existing.ID).
			SetTokenEnc(tokenEnc).
			SetIssuedBy(issuedBy).
			ClearDisabledAt().
			Save(ctx)
		if uerr != nil {
			return nil, repository.TranslateError(uerr)
		}
		return updated, nil
	}
	if !ent.IsNotFound(err) {
		return nil, repository.TranslateError(err)
	}

	created, err := r.ec(ctx).StationEnrollToken.Create().
		SetTokenEnc(tokenEnc).
		SetIssuedBy(issuedBy).
		Save(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return created, nil
}

func (r *enrollCodeRepo) SetDisabled(ctx context.Context, disabled bool) (*ent.StationEnrollToken, error) {
	existing, err := r.ec(ctx).StationEnrollToken.Query().First(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	b := r.ec(ctx).StationEnrollToken.UpdateOneID(existing.ID)
	if disabled {
		b.SetDisabledAt(time.Now().UTC())
	} else {
		b.ClearDisabledAt()
	}
	updated, err := b.Save(ctx)
	if err != nil {
		return nil, repository.TranslateError(err)
	}
	return updated, nil
}
