package stationqr

import (
	"context"
	"testing"
	"time"

	"backend/internal/crypto"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/device"
	"backend/internal/repository"

	"github.com/stretchr/testify/require"
)

type fakeCodeRepo struct {
	row *ent.StationEnrollToken
}

func (f *fakeCodeRepo) Get(context.Context) (*ent.StationEnrollToken, error) {
	if f.row == nil {
		return nil, repository.ErrNotFound
	}
	return f.row, nil
}

func (f *fakeCodeRepo) Upsert(_ context.Context, tokenEnc []byte, issuedBy string) (*ent.StationEnrollToken, error) {
	if f.row == nil {
		f.row = &ent.StationEnrollToken{ID: "sec_1"}
	}
	f.row.TokenEnc = tokenEnc
	f.row.IssuedBy = issuedBy
	f.row.DisabledAt = nil
	return f.row, nil
}

func (f *fakeCodeRepo) SetDisabled(_ context.Context, disabled bool) (*ent.StationEnrollToken, error) {
	if f.row == nil {
		return nil, repository.ErrNotFound
	}
	if disabled {
		now := time.Now()
		f.row.DisabledAt = &now
	} else {
		f.row.DisabledAt = nil
	}
	return f.row, nil
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

func newSvc(t *testing.T) (*service, *fakeCodeRepo, *fakeUpserter, *fakeApprover) {
	t.Helper()
	key := make([]byte, crypto.KeySize)
	for i := range key {
		key[i] = byte(i + 7)
	}
	enc, err := crypto.NewEncryptor(key)
	require.NoError(t, err)
	codes := &fakeCodeRepo{}
	up := &fakeUpserter{}
	ap := &fakeApprover{}
	return &service{codes: codes, devices: up, approver: ap, enc: enc}, codes, up, ap
}

func TestGetUnconfigured(t *testing.T) {
	svc, _, _, _ := newSvc(t)
	code, err := svc.Get(context.Background())
	require.NoError(t, err)
	require.False(t, code.Configured)
}

func TestRotateStoresEncryptedAndReadsBack(t *testing.T) {
	svc, codes, _, _ := newSvc(t)

	code, err := svc.Rotate(context.Background(), "user_admin")
	require.NoError(t, err)
	require.True(t, code.Configured)
	require.NotEmpty(t, code.Token)
	require.NotContains(t, string(codes.row.TokenEnc), code.Token, "raw token must be encrypted at rest")

	got, err := svc.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, code.Token, got.Token, "Get must decrypt back to the same token for reprinting")
	require.Equal(t, "user_admin", got.IssuedBy)
}

func TestRotateInvalidatesPreviousToken(t *testing.T) {
	svc, _, _, _ := newSvc(t)
	first, err := svc.Rotate(context.Background(), "u")
	require.NoError(t, err)
	second, err := svc.Rotate(context.Background(), "u")
	require.NoError(t, err)
	require.NotEqual(t, first.Token, second.Token)

	_, err = svc.Enroll(context.Background(), first.Token, "dk1", "", "", "")
	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestEnrollApprovesStationWithIssuerAttribution(t *testing.T) {
	svc, _, up, ap := newSvc(t)
	code, err := svc.Rotate(context.Background(), "user_admin")
	require.NoError(t, err)

	out, err := svc.Enroll(context.Background(), code.Token, "dk1", "Grill 1", "Pixel", "Android")
	require.NoError(t, err)
	require.Equal(t, "session_dk1", out.Token)
	require.Equal(t, device.StatusApproved, out.Device.Status)
	require.Equal(t, device.TypeSTATION, up.upserted.Type)
	require.Equal(t, "Grill 1", up.upserted.Name)
	require.Equal(t, "user_admin", ap.issuer, "decided_by must be the code issuer")
}

func TestEnrollIsMultiUse(t *testing.T) {
	svc, _, up, _ := newSvc(t)
	code, err := svc.Rotate(context.Background(), "u")
	require.NoError(t, err)

	_, err = svc.Enroll(context.Background(), code.Token, "dk1", "", "", "")
	require.NoError(t, err)
	_, err = svc.Enroll(context.Background(), code.Token, "dk2", "", "", "")
	require.NoError(t, err)
	require.Equal(t, 2, up.count, "the same durable code enrolls multiple devices")
}

func TestEnrollDefaultsName(t *testing.T) {
	svc, _, up, _ := newSvc(t)
	code, err := svc.Rotate(context.Background(), "u")
	require.NoError(t, err)
	_, err = svc.Enroll(context.Background(), code.Token, "dk1", "", "", "")
	require.NoError(t, err)
	require.Equal(t, "Station", up.upserted.Name)
}

func TestEnrollWrongToken(t *testing.T) {
	svc, _, _, _ := newSvc(t)
	_, err := svc.Rotate(context.Background(), "u")
	require.NoError(t, err)
	_, err = svc.Enroll(context.Background(), "not-the-token", "dk1", "", "", "")
	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestEnrollNotConfigured(t *testing.T) {
	svc, _, _, _ := newSvc(t)
	_, err := svc.Enroll(context.Background(), "whatever", "dk1", "", "", "")
	require.ErrorIs(t, err, ErrNotConfigured)
}

func TestEnrollDisabled(t *testing.T) {
	svc, _, _, _ := newSvc(t)
	code, err := svc.Rotate(context.Background(), "u")
	require.NoError(t, err)
	_, err = svc.SetDisabled(context.Background(), true)
	require.NoError(t, err)

	_, err = svc.Enroll(context.Background(), code.Token, "dk1", "", "", "")
	require.ErrorIs(t, err, ErrDisabled)

	_, err = svc.SetDisabled(context.Background(), false)
	require.NoError(t, err)
	_, err = svc.Enroll(context.Background(), code.Token, "dk1", "", "", "")
	require.NoError(t, err)
}

func TestSetDisabledNotConfigured(t *testing.T) {
	svc, _, _, _ := newSvc(t)
	_, err := svc.SetDisabled(context.Background(), true)
	require.ErrorIs(t, err, ErrNotConfigured)
}
