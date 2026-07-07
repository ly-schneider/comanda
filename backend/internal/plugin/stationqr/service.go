package stationqr

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"

	"backend/internal/crypto"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/device"
	"backend/internal/repository"
)

const Key = "stationqr"

var (
	ErrNotConfigured = errors.New("stationqr: no enrollment code configured")
	ErrDisabled      = errors.New("stationqr: enrollment code disabled")
	ErrInvalidToken  = errors.New("stationqr: invalid enrollment token")
)

type DeviceApprover interface {
	ApproveDirect(ctx context.Context, d *ent.Device, issuerUserID string) (*ent.Device, string, error)
}

type DeviceUpserter interface {
	UpsertPendingByDeviceKey(ctx context.Context, name, deviceModel, os, deviceKey string, deviceType device.Type) (*ent.Device, error)
}

type Code struct {
	Configured bool
	Disabled   bool
	Token      string
	IssuedBy   string
}

type EnrollResult struct {
	Device *ent.Device
	Token  string
}

type Service interface {
	Get(ctx context.Context) (Code, error)
	Rotate(ctx context.Context, issuerUserID string) (Code, error)
	SetDisabled(ctx context.Context, disabled bool) (Code, error)
	Enroll(ctx context.Context, token, deviceKey, name, model, os string) (EnrollResult, error)
}

type service struct {
	codes    EnrollCodeRepository
	devices  DeviceUpserter
	approver DeviceApprover
	enc      *crypto.Encryptor
}

func NewService(codes EnrollCodeRepository, devices DeviceUpserter, approver DeviceApprover, enc *crypto.Encryptor) Service {
	return &service{codes: codes, devices: devices, approver: approver, enc: enc}
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *service) decrypt(tokenEnc []byte) (string, error) {
	if s.enc == nil {
		return "", errors.New("stationqr: encryptor not configured")
	}
	return s.enc.DecryptString(string(tokenEnc))
}

func (s *service) toCode(row *ent.StationEnrollToken) (Code, error) {
	token, err := s.decrypt(row.TokenEnc)
	if err != nil {
		return Code{}, fmt.Errorf("decrypt token: %w", err)
	}
	return Code{
		Configured: true,
		Disabled:   row.DisabledAt != nil,
		Token:      token,
		IssuedBy:   row.IssuedBy,
	}, nil
}

func (s *service) Get(ctx context.Context) (Code, error) {
	row, err := s.codes.Get(ctx)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return Code{Configured: false}, nil
		}
		return Code{}, err
	}
	return s.toCode(row)
}

func (s *service) Rotate(ctx context.Context, issuerUserID string) (Code, error) {
	if s.enc == nil {
		return Code{}, errors.New("stationqr: encryptor not configured")
	}
	raw, err := generateToken()
	if err != nil {
		return Code{}, fmt.Errorf("generate token: %w", err)
	}
	blob, err := s.enc.EncryptString(raw)
	if err != nil {
		return Code{}, fmt.Errorf("encrypt token: %w", err)
	}
	row, err := s.codes.Upsert(ctx, []byte(blob), issuerUserID)
	if err != nil {
		return Code{}, err
	}
	return Code{Configured: true, Disabled: row.DisabledAt != nil, Token: raw, IssuedBy: row.IssuedBy}, nil
}

func (s *service) SetDisabled(ctx context.Context, disabled bool) (Code, error) {
	row, err := s.codes.SetDisabled(ctx, disabled)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return Code{}, ErrNotConfigured
		}
		return Code{}, err
	}
	return s.toCode(row)
}

func (s *service) Enroll(ctx context.Context, token, deviceKey, name, model, os string) (EnrollResult, error) {
	row, err := s.codes.Get(ctx)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return EnrollResult{}, ErrNotConfigured
		}
		return EnrollResult{}, err
	}
	if row.DisabledAt != nil {
		return EnrollResult{}, ErrDisabled
	}

	plain, err := s.decrypt(row.TokenEnc)
	if err != nil {
		return EnrollResult{}, err
	}
	if subtle.ConstantTimeCompare([]byte(plain), []byte(token)) != 1 {
		return EnrollResult{}, ErrInvalidToken
	}

	if name == "" {
		name = "Station"
	}

	d, err := s.devices.UpsertPendingByDeviceKey(ctx, name, model, os, deviceKey, device.TypeSTATION)
	if err != nil {
		return EnrollResult{}, err
	}

	approved, sessionToken, err := s.approver.ApproveDirect(ctx, d, row.IssuedBy)
	if err != nil {
		return EnrollResult{}, err
	}
	return EnrollResult{Device: approved, Token: sessionToken}, nil
}
