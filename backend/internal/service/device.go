package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/device"
	"backend/internal/generated/ent/devicebinding"
	"backend/internal/repository"
)

// PairingResult is the result of creating a new device pairing code.
type PairingResult struct {
	Code      string
	ExpiresAt time.Time
}

// PairingStatusResult is the result of polling a device pairing status.
type PairingStatusResult struct {
	Status string
	Token  *string
	Device *ent.Device
}

type DeviceService interface {
	ListAll(ctx context.Context, deviceType *string, status *string) ([]*ent.Device, error)
	GetByID(ctx context.Context, id string) (*ent.Device, error)
	Remove(ctx context.Context, id string) error
	CreatePairing(ctx context.Context, deviceKey, name, deviceType string, model, os *string) (*PairingResult, error)
	GetPairingStatus(ctx context.Context, code string) (*PairingStatusResult, error)
	CompletePairing(ctx context.Context, code string, adminUserID string) (*ent.Device, error)
	ApproveDirect(ctx context.Context, d *ent.Device, issuerUserID string) (*ent.Device, string, error)
}

type deviceService struct {
	client         *ent.Client
	devices        repository.DeviceRepository
	deviceProducts repository.DeviceProductRepository
	bindings       repository.DeviceBindingRepository
	sessionRepo    repository.SessionRepository
}

func NewDeviceService(
	client *ent.Client,
	devices repository.DeviceRepository,
	deviceProducts repository.DeviceProductRepository,
	bindings repository.DeviceBindingRepository,
	sessionRepo repository.SessionRepository,
) DeviceService {
	return &deviceService{
		client:         client,
		devices:        devices,
		deviceProducts: deviceProducts,
		bindings:       bindings,
		sessionRepo:    sessionRepo,
	}
}

func (s *deviceService) ListAll(ctx context.Context, deviceType *string, status *string) ([]*ent.Device, error) {
	q := repository.ClientFromContext(ctx, s.client).Device.Query()
	if deviceType != nil && *deviceType != "" {
		q = q.Where(device.TypeEQ(device.Type(*deviceType)))
	}
	if status != nil && *status != "" {
		q = q.Where(device.StatusEQ(device.Status(*status)))
	}
	return q.Order(device.ByCreatedAt()).All(ctx)
}

func (s *deviceService) GetByID(ctx context.Context, id string) (*ent.Device, error) {
	return s.devices.GetByID(ctx, id)
}

func (s *deviceService) Remove(ctx context.Context, id string) error {
	d, err := s.devices.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// device_binding.device_id is a loose link, not a foreign key, so deleting
	// the device leaves its bindings authorizing. Revoke them first — this also
	// evicts the token-hash auth cache.
	bindings, err := s.bindings.ListActiveByDeviceID(ctx, d.ID)
	if err != nil {
		return err
	}
	for _, b := range bindings {
		if err := s.bindings.Revoke(ctx, b.ID); err != nil {
			return err
		}
	}

	// device_product FKs are ON DELETE NO ACTION and would block the delete.
	if err := s.deviceProducts.DeleteByDeviceID(ctx, d.ID); err != nil {
		return err
	}

	return s.devices.Delete(ctx, d.ID)
}

func (s *deviceService) CreatePairing(ctx context.Context, deviceKey, name, deviceType string, model, os *string) (*PairingResult, error) {
	mdl := ""
	if model != nil {
		mdl = *model
	}
	osStr := ""
	if os != nil {
		osStr = *os
	}

	d, err := s.devices.GeneratePairingCode(ctx, name, mdl, osStr, deviceKey, device.Type(deviceType))
	if err != nil {
		return nil, err
	}

	if d.PairingCode == nil || d.PairingCodeExpiresAt == nil {
		return nil, errors.New("pairing code generation failed")
	}

	return &PairingResult{
		Code:      *d.PairingCode,
		ExpiresAt: *d.PairingCodeExpiresAt,
	}, nil
}

func (s *deviceService) GetPairingStatus(ctx context.Context, code string) (*PairingStatusResult, error) {
	d, err := s.devices.GetByPairingCode(ctx, code)
	if err != nil {
		return nil, err
	}

	result := &PairingStatusResult{
		Status: string(d.Status),
		Device: d,
	}

	// If device has been approved and has a pending session token, return it.
	if d.Status == device.StatusApproved && d.PendingSessionToken != nil {
		result.Token = d.PendingSessionToken
	}

	return result, nil
}

// deviceSessionExpiry matches the Better Auth session TTL (90 days).
const deviceSessionExpiry = 90 * 24 * time.Hour

func (s *deviceService) CompletePairing(ctx context.Context, code string, adminUserID string) (*ent.Device, error) {
	d, err := s.devices.GetByPairingCode(ctx, code)
	if err != nil {
		return nil, err
	}

	if d.Status != device.StatusPending {
		return nil, fmt.Errorf("device is not pending approval")
	}

	updated, sessionToken, err := s.ApproveDirect(ctx, d, adminUserID)
	if err != nil {
		return nil, err
	}

	// Store the raw session token so the polling device can pick it up.
	if err := s.devices.SetPendingSessionToken(ctx, d.ID, sessionToken); err != nil {
		return nil, err
	}

	return updated, nil
}

func (s *deviceService) ApproveDirect(ctx context.Context, d *ent.Device, issuerUserID string) (*ent.Device, string, error) {
	sessionToken, err := s.sessionRepo.CreateSession(ctx, issuerUserID, deviceSessionExpiry)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create device session: %w", err)
	}

	tokenHash := repository.HashToken(sessionToken)

	now := time.Now().UTC()
	updated, err := s.devices.Update(ctx, d.ID, d.Name, d.DeviceKey, d.Type, device.StatusApproved,
		d.Model, d.Os, &issuerUserID, &now, d.ExpiresAt,
		nil, d.PairingCode, d.PairingCodeExpiresAt)
	if err != nil {
		return nil, "", err
	}

	bindingType := devicebinding.DeviceTypePOS
	if d.Type == device.TypeSTATION {
		bindingType = devicebinding.DeviceTypeSTATION
	}
	deviceID := d.ID
	if _, err := s.bindings.Create(ctx, bindingType, tokenHash, issuerUserID, &d.Name, &deviceID, nil); err != nil {
		return nil, "", fmt.Errorf("failed to create device binding: %w", err)
	}

	return updated, sessionToken, nil
}
