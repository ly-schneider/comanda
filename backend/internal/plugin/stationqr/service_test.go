package stationqr

import (
	"context"
	"fmt"
	"testing"
	"time"

	"backend/internal/crypto"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/device"
	"backend/internal/repository"
	coreservice "backend/internal/service"

	"github.com/stretchr/testify/require"
)

type fakePresetRepo struct {
	seq      int
	rows     map[string]*ent.StationEnrollPreset
	products map[string][]string
}

func newFakePresetRepo() *fakePresetRepo {
	return &fakePresetRepo{rows: map[string]*ent.StationEnrollPreset{}, products: map[string][]string{}}
}

func (f *fakePresetRepo) Create(_ context.Context, name string, tokenEnc []byte, issuedBy string) (*ent.StationEnrollPreset, error) {
	f.seq++
	id := fmt.Sprintf("preset_%d", f.seq)
	now := time.Now()
	row := &ent.StationEnrollPreset{ID: id, Name: name, TokenEnc: tokenEnc, IssuedBy: issuedBy, CreatedAt: now, UpdatedAt: now}
	f.rows[id] = row
	return row, nil
}

func (f *fakePresetRepo) List(context.Context) ([]*ent.StationEnrollPreset, error) {
	out := make([]*ent.StationEnrollPreset, 0, len(f.rows))
	for i := 1; i <= f.seq; i++ {
		if row, ok := f.rows[fmt.Sprintf("preset_%d", i)]; ok {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f *fakePresetRepo) GetByID(_ context.Context, id string) (*ent.StationEnrollPreset, error) {
	row, ok := f.rows[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return row, nil
}

func (f *fakePresetRepo) Rename(_ context.Context, id, name string) (*ent.StationEnrollPreset, error) {
	row, ok := f.rows[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	row.Name = name
	return row, nil
}

func (f *fakePresetRepo) SetToken(_ context.Context, id string, tokenEnc []byte) (*ent.StationEnrollPreset, error) {
	row, ok := f.rows[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	row.TokenEnc = tokenEnc
	return row, nil
}

func (f *fakePresetRepo) SetDisabled(_ context.Context, id string, disabled bool) (*ent.StationEnrollPreset, error) {
	row, ok := f.rows[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	if disabled {
		now := time.Now()
		row.DisabledAt = &now
	} else {
		row.DisabledAt = nil
	}
	return row, nil
}

func (f *fakePresetRepo) Delete(_ context.Context, id string) error {
	if _, ok := f.rows[id]; !ok {
		return repository.ErrNotFound
	}
	delete(f.rows, id)
	delete(f.products, id)
	return nil
}

func (f *fakePresetRepo) ReplaceProducts(_ context.Context, presetID string, productIDs []string) error {
	f.products[presetID] = append([]string(nil), productIDs...)
	return nil
}

func (f *fakePresetRepo) ListProducts(_ context.Context, presetID string) ([]*ent.StationPresetProduct, error) {
	return f.linksFor(presetID), nil
}

func (f *fakePresetRepo) ListAllProducts(context.Context) ([]*ent.StationPresetProduct, error) {
	var out []*ent.StationPresetProduct
	for i := 1; i <= f.seq; i++ {
		out = append(out, f.linksFor(fmt.Sprintf("preset_%d", i))...)
	}
	return out, nil
}

func (f *fakePresetRepo) linksFor(presetID string) []*ent.StationPresetProduct {
	ids := f.products[presetID]
	out := make([]*ent.StationPresetProduct, 0, len(ids))
	for _, pid := range ids {
		out = append(out, &ent.StationPresetProduct{PresetID: presetID, ProductID: pid})
	}
	return out
}

type fakeUpserter struct {
	count    int
	upserted *ent.Device
}

func (f *fakeUpserter) UpsertPendingByDeviceKey(_ context.Context, name, model, os, deviceKey string, dt device.Type) (*ent.Device, error) {
	f.count++
	f.upserted = &ent.Device{ID: "dev_" + deviceKey, Name: name, DeviceKey: deviceKey, Type: dt, Status: device.StatusPending}
	return f.upserted, nil
}

type fakeApprover struct {
	issuer string
}

func (f *fakeApprover) ApproveDirect(_ context.Context, d *ent.Device, issuerUserID string) (*ent.Device, string, error) {
	f.issuer = issuerUserID
	d.Status = device.StatusApproved
	return d, "session_" + d.DeviceKey, nil
}

type fakeStations struct {
	coreservice.StationService
	assigned map[string][]string
}

func (f *fakeStations) SetStationProducts(_ context.Context, stationID string, productIDs []string) error {
	if f.assigned == nil {
		f.assigned = map[string][]string{}
	}
	f.assigned[stationID] = append([]string(nil), productIDs...)
	return nil
}

// fakeProducts stands in for the RLS-scoped product repo. known == nil treats
// every requested id as existing; a non-nil set drops ids outside it (a
// cross-tenant product id).
type fakeProducts struct {
	known map[string]bool
}

func (f *fakeProducts) GetByIDs(_ context.Context, ids []string) ([]*ent.Product, error) {
	out := make([]*ent.Product, 0, len(ids))
	for _, id := range ids {
		if f.known == nil || f.known[id] {
			out = append(out, &ent.Product{ID: id})
		}
	}
	return out, nil
}

func newSvc(t *testing.T) (*service, *fakePresetRepo, *fakeUpserter, *fakeApprover, *fakeStations) {
	t.Helper()
	key := make([]byte, crypto.KeySize)
	for i := range key {
		key[i] = byte(i + 7)
	}
	enc, err := crypto.NewEncryptor(key)
	require.NoError(t, err)
	presets := newFakePresetRepo()
	up := &fakeUpserter{}
	ap := &fakeApprover{}
	st := &fakeStations{}
	return &service{presets: presets, devices: up, approver: ap, stations: st, products: &fakeProducts{}, enc: enc}, presets, up, ap, st
}

func TestCreatePresetStoresEncryptedAndReturnsToken(t *testing.T) {
	svc, repo, _, _, _ := newSvc(t)

	p, err := svc.CreatePreset(context.Background(), "Burger", []string{"prod_1", "prod_2"}, "user_admin")
	require.NoError(t, err)
	require.NotEmpty(t, p.ID)
	require.Equal(t, "Burger", p.Name)
	require.NotEmpty(t, p.Token)
	require.Equal(t, []string{"prod_1", "prod_2"}, p.ProductIDs)
	require.NotContains(t, string(repo.rows[p.ID].TokenEnc), p.Token, "raw token must be encrypted at rest")
}

func TestCreatePresetDropsForeignProducts(t *testing.T) {
	svc, _, _, _, _ := newSvc(t)
	svc.products = &fakeProducts{known: map[string]bool{"prod_1": true}}

	p, err := svc.CreatePreset(context.Background(), "Burger", []string{"prod_1", "prod_foreign"}, "u")
	require.NoError(t, err)
	require.Equal(t, []string{"prod_1"}, p.ProductIDs, "cross-tenant product id must be dropped")
}

func TestUpdatePresetDropsForeignProducts(t *testing.T) {
	svc, _, _, _, _ := newSvc(t)
	p, err := svc.CreatePreset(context.Background(), "Burger", nil, "u")
	require.NoError(t, err)

	svc.products = &fakeProducts{known: map[string]bool{"prod_2": true}}
	replaced := []string{"prod_2", "prod_foreign"}
	updated, err := svc.UpdatePreset(context.Background(), p.ID, UpdatePresetInput{ProductIDs: &replaced})
	require.NoError(t, err)
	require.Equal(t, []string{"prod_2"}, updated.ProductIDs, "cross-tenant product id must be dropped on update")
}

func TestCreatePresetRequiresName(t *testing.T) {
	svc, _, _, _, _ := newSvc(t)
	_, err := svc.CreatePreset(context.Background(), "   ", nil, "u")
	require.ErrorIs(t, err, ErrNameRequired)
}

func TestCreatePresetEmptyProductsAllowed(t *testing.T) {
	svc, _, _, _, _ := newSvc(t)
	p, err := svc.CreatePreset(context.Background(), "Enroll only", nil, "u")
	require.NoError(t, err)
	require.Empty(t, p.ProductIDs)
}

func TestListPresetsGroupsProducts(t *testing.T) {
	svc, _, _, _, _ := newSvc(t)
	a, err := svc.CreatePreset(context.Background(), "Burger", []string{"prod_1"}, "u")
	require.NoError(t, err)
	b, err := svc.CreatePreset(context.Background(), "Drinks", []string{"prod_2", "prod_3"}, "u")
	require.NoError(t, err)

	list, err := svc.ListPresets(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 2)

	byID := map[string]Preset{list[0].ID: list[0], list[1].ID: list[1]}
	require.Equal(t, []string{"prod_1"}, byID[a.ID].ProductIDs)
	require.ElementsMatch(t, []string{"prod_2", "prod_3"}, byID[b.ID].ProductIDs)
	require.NotEmpty(t, byID[a.ID].Token, "list must decrypt tokens for reprinting")
}

func TestUpdatePresetPartial(t *testing.T) {
	svc, _, _, _, _ := newSvc(t)
	p, err := svc.CreatePreset(context.Background(), "Burger", []string{"prod_1"}, "u")
	require.NoError(t, err)

	newName := "Grill"
	renamed, err := svc.UpdatePreset(context.Background(), p.ID, UpdatePresetInput{Name: &newName})
	require.NoError(t, err)
	require.Equal(t, "Grill", renamed.Name)
	require.Equal(t, []string{"prod_1"}, renamed.ProductIDs, "products untouched when only renaming")

	disabled := true
	toggled, err := svc.UpdatePreset(context.Background(), p.ID, UpdatePresetInput{Disabled: &disabled})
	require.NoError(t, err)
	require.True(t, toggled.Disabled)

	newProducts := []string{"prod_2", "prod_3"}
	repl, err := svc.UpdatePreset(context.Background(), p.ID, UpdatePresetInput{ProductIDs: &newProducts})
	require.NoError(t, err)
	require.ElementsMatch(t, newProducts, repl.ProductIDs)

	empty := []string{}
	cleared, err := svc.UpdatePreset(context.Background(), p.ID, UpdatePresetInput{ProductIDs: &empty})
	require.NoError(t, err)
	require.Empty(t, cleared.ProductIDs)
}

func TestUpdatePresetNotFound(t *testing.T) {
	svc, _, _, _, _ := newSvc(t)
	name := "x"
	_, err := svc.UpdatePreset(context.Background(), "preset_missing", UpdatePresetInput{Name: &name})
	require.ErrorIs(t, err, ErrNotFound)
}

func TestRotatePresetChangesTokenKeepsProducts(t *testing.T) {
	svc, _, _, _, _ := newSvc(t)
	p, err := svc.CreatePreset(context.Background(), "Burger", []string{"prod_1"}, "u")
	require.NoError(t, err)

	rotated, err := svc.RotatePreset(context.Background(), p.ID)
	require.NoError(t, err)
	require.NotEqual(t, p.Token, rotated.Token)
	require.Equal(t, []string{"prod_1"}, rotated.ProductIDs)

	_, err = svc.Enroll(context.Background(), p.Token, "dk1", "", "", "")
	require.ErrorIs(t, err, ErrInvalidToken, "old token invalidated by rotate")
}

func TestDeletePreset(t *testing.T) {
	svc, _, _, _, _ := newSvc(t)
	p, err := svc.CreatePreset(context.Background(), "Burger", nil, "u")
	require.NoError(t, err)
	require.NoError(t, svc.DeletePreset(context.Background(), p.ID))
	require.ErrorIs(t, svc.DeletePreset(context.Background(), p.ID), ErrNotFound)
}

func TestEnrollCopiesPresetProductsWithIssuer(t *testing.T) {
	svc, _, up, ap, st := newSvc(t)
	p, err := svc.CreatePreset(context.Background(), "Burger", []string{"prod_1", "prod_2"}, "user_admin")
	require.NoError(t, err)

	out, err := svc.Enroll(context.Background(), p.Token, "dk1", "Grill 1", "Pixel", "Android")
	require.NoError(t, err)
	require.Equal(t, "session_dk1", out.Token)
	require.Equal(t, device.StatusApproved, out.Device.Status)
	require.Equal(t, device.TypeSTATION, up.upserted.Type)
	require.Equal(t, "Grill 1", up.upserted.Name)
	require.Equal(t, "user_admin", ap.issuer, "decided_by must be the preset issuer")
	require.ElementsMatch(t, []string{"prod_1", "prod_2"}, st.assigned["dev_dk1"], "preset products copied to device")
}

func TestEnrollEmptyPresetAssignsNoProducts(t *testing.T) {
	svc, _, _, _, st := newSvc(t)
	p, err := svc.CreatePreset(context.Background(), "Enroll only", nil, "u")
	require.NoError(t, err)

	_, err = svc.Enroll(context.Background(), p.Token, "dk1", "", "", "")
	require.NoError(t, err)
	require.Empty(t, st.assigned["dev_dk1"], "empty preset still enrolls, assigns no products")
}

func TestEnrollResolvesAcrossPresets(t *testing.T) {
	svc, _, _, _, st := newSvc(t)
	_, err := svc.CreatePreset(context.Background(), "Burger", []string{"prod_1"}, "u")
	require.NoError(t, err)
	drinks, err := svc.CreatePreset(context.Background(), "Drinks", []string{"prod_9"}, "u")
	require.NoError(t, err)

	_, err = svc.Enroll(context.Background(), drinks.Token, "dk1", "", "", "")
	require.NoError(t, err)
	require.Equal(t, []string{"prod_9"}, st.assigned["dev_dk1"], "the scanned preset's products, not another's")
}

func TestEnrollIsMultiUse(t *testing.T) {
	svc, _, up, _, _ := newSvc(t)
	p, err := svc.CreatePreset(context.Background(), "Burger", nil, "u")
	require.NoError(t, err)

	_, err = svc.Enroll(context.Background(), p.Token, "dk1", "", "", "")
	require.NoError(t, err)
	_, err = svc.Enroll(context.Background(), p.Token, "dk2", "", "", "")
	require.NoError(t, err)
	require.Equal(t, 2, up.count, "the same preset QR enrolls multiple devices")
}

func TestEnrollDefaultsName(t *testing.T) {
	svc, _, up, _, _ := newSvc(t)
	p, err := svc.CreatePreset(context.Background(), "Burger", nil, "u")
	require.NoError(t, err)
	_, err = svc.Enroll(context.Background(), p.Token, "dk1", "", "", "")
	require.NoError(t, err)
	require.Equal(t, "Station", up.upserted.Name)
}

func TestEnrollWrongToken(t *testing.T) {
	svc, _, _, _, _ := newSvc(t)
	_, err := svc.CreatePreset(context.Background(), "Burger", nil, "u")
	require.NoError(t, err)
	_, err = svc.Enroll(context.Background(), "not-the-token", "dk1", "", "", "")
	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestEnrollNoPresets(t *testing.T) {
	svc, _, _, _, _ := newSvc(t)
	_, err := svc.Enroll(context.Background(), "whatever", "dk1", "", "", "")
	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestEnrollDisabledPreset(t *testing.T) {
	svc, _, _, _, _ := newSvc(t)
	p, err := svc.CreatePreset(context.Background(), "Burger", nil, "u")
	require.NoError(t, err)
	disabled := true
	_, err = svc.UpdatePreset(context.Background(), p.ID, UpdatePresetInput{Disabled: &disabled})
	require.NoError(t, err)

	_, err = svc.Enroll(context.Background(), p.Token, "dk1", "", "", "")
	require.ErrorIs(t, err, ErrDisabled)

	enabled := false
	_, err = svc.UpdatePreset(context.Background(), p.ID, UpdatePresetInput{Disabled: &enabled})
	require.NoError(t, err)
	_, err = svc.Enroll(context.Background(), p.Token, "dk1", "", "", "")
	require.NoError(t, err)
}
