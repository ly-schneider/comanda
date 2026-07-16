package artistvoucher

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"backend/internal/generated/ent"
	"backend/internal/qrsign"
	"backend/internal/repository"

	"github.com/stretchr/testify/require"
)

const testTenantID = "tenant_test1"

type fakeRepo struct {
	rows        map[string]*ent.Voucher
	order       []string
	revokeCalls int
	deleteCalls int
}

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[string]*ent.Voucher{}} }

func (f *fakeRepo) Create(_ context.Context, p CreateParams) (*ent.Voucher, error) {
	v := &ent.Voucher{
		ID:         p.ID,
		Label:      p.Label,
		ValidFrom:  p.ValidFrom,
		ValidUntil: p.ValidUntil,
		Categories: p.Categories,
		Token:      p.Token,
		CreatedAt:  time.Now(),
	}
	f.rows[p.ID] = v
	f.order = append(f.order, p.ID)
	return v, nil
}

func (f *fakeRepo) GetByID(_ context.Context, id string) (*ent.Voucher, error) {
	v, ok := f.rows[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return v, nil
}

func (f *fakeRepo) List(_ context.Context) ([]*ent.Voucher, error) {
	out := make([]*ent.Voucher, 0, len(f.order))
	for i := len(f.order) - 1; i >= 0; i-- {
		out = append(out, f.rows[f.order[i]])
	}
	return out, nil
}

func (f *fakeRepo) SetRevoked(_ context.Context, id string, at time.Time) (*ent.Voucher, error) {
	v, ok := f.rows[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	f.revokeCalls++
	t := at
	v.RevokedAt = &t
	return v, nil
}

func (f *fakeRepo) Delete(_ context.Context, id string) error {
	if _, ok := f.rows[id]; !ok {
		return repository.ErrNotFound
	}
	f.deleteCalls++
	delete(f.rows, id)
	for i, oid := range f.order {
		if oid == id {
			f.order = append(f.order[:i], f.order[i+1:]...)
			break
		}
	}
	return nil
}

func (f *fakeRepo) ListActiveRevoked(_ context.Context, now time.Time) ([]*ent.Voucher, error) {
	out := []*ent.Voucher{}
	for _, id := range f.order {
		v := f.rows[id]
		if v.RevokedAt != nil && v.ValidUntil.After(now) {
			out = append(out, v)
		}
	}
	return out, nil
}

type fakeCategories struct {
	byID map[string]*ent.Category
}

func (f *fakeCategories) GetByID(_ context.Context, id string) (*ent.Category, error) {
	c, ok := f.byID[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return c, nil
}

type fakeKeys struct {
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
}

func (f *fakeKeys) EnsureTenantKey(context.Context) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	return f.priv, f.pub, nil
}

func newTestService(t *testing.T, cats map[string]*ent.Category) (*service, *fakeRepo, *fakeKeys) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	repo := newFakeRepo()
	keys := &fakeKeys{priv: priv, pub: pub}
	svc := NewService(repo, &fakeCategories{byID: cats}, keys).(*service)
	return svc, repo, keys
}

func tenantCtx() context.Context {
	return repository.ContextWithTenantID(context.Background(), testTenantID)
}

func categoryFixtures() map[string]*ent.Category {
	return map[string]*ent.Category{
		"cat_drinks01": {ID: "cat_drinks01", Name: "Drinks"},
		"cat_food0001": {ID: "cat_food0001", Name: "Food"},
	}
}

func validCreateInput() CreateInput {
	now := time.Now()
	return CreateInput{
		Label:       "Live Band",
		ValidFrom:   now.Add(-time.Hour),
		ValidUntil:  now.Add(24 * time.Hour),
		CategoryIDs: []string{"cat_drinks01", "cat_food0001"},
	}
}

func TestCreateRejectsUnknownCategory(t *testing.T) {
	svc, _, _ := newTestService(t, categoryFixtures())
	in := validCreateInput()
	in.CategoryIDs = []string{"cat_drinks01", "cat_foreign99"}

	_, err := svc.Create(tenantCtx(), in)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrUnknownCategory))
	require.True(t, IsValidation(err))
}

func TestCreateValidationErrors(t *testing.T) {
	svc, _, _ := newTestService(t, categoryFixtures())

	cases := map[string]func(CreateInput) CreateInput{
		"empty label":        func(in CreateInput) CreateInput { in.Label = ""; return in },
		"label too long":     func(in CreateInput) CreateInput { in.Label = string(make([]rune, 81)); return in },
		"window not forward": func(in CreateInput) CreateInput { in.ValidUntil = in.ValidFrom; return in },
		"no categories":      func(in CreateInput) CreateInput { in.CategoryIDs = nil; return in },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Create(tenantCtx(), mut(validCreateInput()))
			require.Error(t, err)
			require.Truef(t, IsValidation(err), "expected validation error, got %v", err)
		})
	}
}

func TestCreateRejectsMultibyteLabelOverByteLimit(t *testing.T) {
	svc, _, _ := newTestService(t, categoryFixtures())
	in := validCreateInput()
	in.Label = strings.Repeat("ä", 41) // 41 runes (≤80), 82 bytes (>80)
	require.Equal(t, 41, utf8.RuneCountInString(in.Label))
	require.Equal(t, 82, len(in.Label))

	_, err := svc.Create(tenantCtx(), in)
	require.True(t, errors.Is(err, ErrLabelTooLong))
}

func TestCreateSnapshotOrderPreservedAndSigned(t *testing.T) {
	svc, _, keys := newTestService(t, categoryFixtures())
	in := validCreateInput()
	in.CategoryIDs = []string{"cat_food0001", "cat_drinks01"}

	v, err := svc.Create(tenantCtx(), in)
	require.NoError(t, err)

	require.Equal(t, []Category{{ID: "cat_food0001", Name: "Food"}, {ID: "cat_drinks01", Name: "Drinks"}}, v.Categories)

	payload, err := qrsign.VerifyVoucher(keys.pub, v.Token)
	require.NoError(t, err)
	require.Equal(t, testTenantID, payload.TenantID)
	require.Equal(t, v.ID, payload.VoucherID)
	require.Equal(t, "Live Band", payload.Label)
	require.Equal(t, []string{"Food", "Drinks"}, payload.Categories)
	require.Equal(t, in.ValidFrom.Unix(), payload.ValidFrom)
	require.Equal(t, in.ValidUntil.Unix(), payload.ValidUntil)
}

func TestComputeStatusTransitions(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	revoked := now.Add(-time.Minute)

	cases := []struct {
		name string
		v    *ent.Voucher
		want Status
	}{
		{"scheduled", &ent.Voucher{ValidFrom: now.Add(time.Hour), ValidUntil: now.Add(2 * time.Hour)}, StatusScheduled},
		{"active", &ent.Voucher{ValidFrom: now.Add(-time.Hour), ValidUntil: now.Add(time.Hour)}, StatusActive},
		{"expired", &ent.Voucher{ValidFrom: now.Add(-2 * time.Hour), ValidUntil: now.Add(-time.Hour)}, StatusExpired},
		{"revoked beats window", &ent.Voucher{ValidFrom: now.Add(-time.Hour), ValidUntil: now.Add(time.Hour), RevokedAt: &revoked}, StatusRevoked},
		{"revoked beats expired", &ent.Voucher{ValidFrom: now.Add(-2 * time.Hour), ValidUntil: now.Add(-time.Hour), RevokedAt: &revoked}, StatusRevoked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, computeStatus(now, tc.v))
		})
	}
}

func TestRevokeIsIdempotent(t *testing.T) {
	svc, repo, _ := newTestService(t, categoryFixtures())
	v, err := svc.Create(tenantCtx(), validCreateInput())
	require.NoError(t, err)

	first, err := svc.Revoke(tenantCtx(), v.ID)
	require.NoError(t, err)
	require.Equal(t, StatusRevoked, first.Status)
	require.NotNil(t, first.RevokedAt)

	second, err := svc.Revoke(tenantCtx(), v.ID)
	require.NoError(t, err)
	require.Equal(t, StatusRevoked, second.Status)
	require.Equal(t, *first.RevokedAt, *second.RevokedAt)
	require.Equal(t, 1, repo.revokeCalls, "second revoke must not write again")
}

func TestRevokeNotFound(t *testing.T) {
	svc, _, _ := newTestService(t, categoryFixtures())
	_, err := svc.Revoke(tenantCtx(), "missing12345")
	require.True(t, errors.Is(err, repository.ErrNotFound))
}

func seedVoucher(repo *fakeRepo, id string, revoked bool, validUntil time.Time) {
	now := time.Now()
	cats, _ := json.Marshal([]Category{{ID: "cat_food0001", Name: "Food"}})
	v := &ent.Voucher{ID: id, Label: "x", ValidFrom: now.Add(-2 * time.Hour), ValidUntil: validUntil, Categories: cats, CreatedAt: now}
	if revoked {
		r := now.Add(-time.Minute)
		v.RevokedAt = &r
	}
	repo.rows[id] = v
	repo.order = append(repo.order, id)
}

func TestDeleteRejectsStillValid(t *testing.T) {
	svc, repo, _ := newTestService(t, categoryFixtures())
	v, err := svc.Create(tenantCtx(), validCreateInput())
	require.NoError(t, err)

	err = svc.Delete(tenantCtx(), v.ID)
	require.True(t, errors.Is(err, ErrNotExpired))
	require.Equal(t, 0, repo.deleteCalls)
	_, ok := repo.rows[v.ID]
	require.True(t, ok, "still-valid voucher must remain")
}

func TestDeleteRevokedButStillValidRejected(t *testing.T) {
	svc, repo, _ := newTestService(t, categoryFixtures())
	seedVoucher(repo, "v_revokedlive", true, time.Now().Add(time.Hour))

	err := svc.Delete(tenantCtx(), "v_revokedlive")
	require.True(t, errors.Is(err, ErrNotExpired), "deleting a live revocation would un-block it at stations")
	require.Equal(t, 0, repo.deleteCalls)
}

func TestDeleteExpiredVoucher(t *testing.T) {
	svc, repo, _ := newTestService(t, categoryFixtures())
	seedVoucher(repo, "v_expired0001", false, time.Now().Add(-time.Hour))

	require.NoError(t, svc.Delete(tenantCtx(), "v_expired0001"))
	require.Equal(t, 1, repo.deleteCalls)
	_, ok := repo.rows["v_expired0001"]
	require.False(t, ok, "expired voucher must be removed")
}

func TestDeleteExpiredRevokedVoucher(t *testing.T) {
	svc, repo, _ := newTestService(t, categoryFixtures())
	seedVoucher(repo, "v_expiredrev1", true, time.Now().Add(-time.Hour))

	require.NoError(t, svc.Delete(tenantCtx(), "v_expiredrev1"))
	require.Equal(t, 1, repo.deleteCalls)
}

func TestDeleteNotFound(t *testing.T) {
	svc, _, _ := newTestService(t, categoryFixtures())
	err := svc.Delete(tenantCtx(), "missing12345")
	require.True(t, errors.Is(err, repository.ErrNotFound))
}

func TestGetNotFound(t *testing.T) {
	svc, _, _ := newTestService(t, categoryFixtures())
	_, err := svc.Get(tenantCtx(), "missing12345")
	require.True(t, errors.Is(err, repository.ErrNotFound))
}

func TestGetReturnsVoucherWithStatus(t *testing.T) {
	svc, _, _ := newTestService(t, categoryFixtures())
	created, err := svc.Create(tenantCtx(), validCreateInput())
	require.NoError(t, err)

	got, err := svc.Get(tenantCtx(), created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	require.Equal(t, "Live Band", got.Label)
	require.Equal(t, StatusActive, got.Status)
}

func TestRevocationsFeedBound(t *testing.T) {
	svc, repo, _ := newTestService(t, categoryFixtures())
	now := time.Now()

	seed := func(id string, revoked bool, validUntil time.Time) {
		cats, _ := json.Marshal([]Category{{ID: "cat_food0001", Name: "Food"}})
		v := &ent.Voucher{ID: id, Label: "x", ValidFrom: now.Add(-time.Hour), ValidUntil: validUntil, Categories: cats, CreatedAt: now}
		if revoked {
			r := now.Add(-time.Minute)
			v.RevokedAt = &r
		}
		repo.rows[id] = v
		repo.order = append(repo.order, id)
	}

	seed("v_activelive1", false, now.Add(time.Hour)) // not revoked → excluded
	seed("v_revokedlive", true, now.Add(time.Hour))  // revoked, not expired → included
	seed("v_revokedgone", true, now.Add(-time.Hour)) // revoked but expired → excluded

	feed, err := svc.Revocations(tenantCtx())
	require.NoError(t, err)
	require.Equal(t, []string{"v_revokedlive"}, feed.RevokedIDs)
	require.False(t, feed.AsOf.IsZero())
}

func TestCreateRequiresTenant(t *testing.T) {
	svc, _, _ := newTestService(t, categoryFixtures())
	_, err := svc.Create(context.Background(), validCreateInput())
	require.True(t, errors.Is(err, ErrNoTenant))
}
