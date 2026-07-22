package stationqr

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"backend/internal/config"
	"backend/internal/crypto"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/device"
	"backend/internal/repository"
	coreservice "backend/internal/service"

	"go.uber.org/zap"
)

const Key = "stationqr"

var (
	ErrNameRequired = errors.New("stationqr: preset name required")
	ErrNotFound     = errors.New("stationqr: preset not found")
	ErrDisabled     = errors.New("stationqr: enrollment preset disabled")
	ErrInvalidToken = errors.New("stationqr: invalid enrollment token")
)

type DeviceApprover interface {
	ApproveDirect(ctx context.Context, d *ent.Device, issuerUserID string) (*ent.Device, string, error)
}

type DeviceUpserter interface {
	UpsertPendingByDeviceKey(ctx context.Context, name, deviceModel, os, deviceKey string, deviceType device.Type) (*ent.Device, error)
}

type ProductLookup interface {
	GetByIDs(ctx context.Context, ids []string) ([]*ent.Product, error)
}

type TenantDomains interface {
	List(ctx context.Context) ([]*ent.TenantDomain, error)
}

type Preset struct {
	ID         string
	Name       string
	Disabled   bool
	Token      string
	EnrollURL  string
	IssuedBy   string
	ProductIDs []string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type UpdatePresetInput struct {
	Name       *string
	Disabled   *bool
	ProductIDs *[]string
}

type EnrollResult struct {
	Device *ent.Device
	Token  string
}

type Service interface {
	ListPresets(ctx context.Context) ([]Preset, error)
	CreatePreset(ctx context.Context, name string, productIDs []string, issuerUserID string) (Preset, error)
	UpdatePreset(ctx context.Context, id string, in UpdatePresetInput) (Preset, error)
	RotatePreset(ctx context.Context, id string) (Preset, error)
	DeletePreset(ctx context.Context, id string) error
	Enroll(ctx context.Context, token, deviceKey, name, model, os string) (EnrollResult, error)
}

type service struct {
	presets  PresetRepository
	devices  DeviceUpserter
	approver DeviceApprover
	stations coreservice.StationService
	products ProductLookup
	domains  TenantDomains
	enc      *crypto.Encryptor
	cfg      config.Config
	logger   *zap.Logger
}

func NewService(presets PresetRepository, devices DeviceUpserter, approver DeviceApprover, stations coreservice.StationService, products ProductLookup, domains TenantDomains, enc *crypto.Encryptor, cfg config.Config, logger *zap.Logger) Service {
	return &service{presets: presets, devices: devices, approver: approver, stations: stations, products: products, domains: domains, enc: enc, cfg: cfg, logger: logger}
}

// enrollBase resolves the origin for enrollment deep-links: the tenant's
// primary verified domain, so a native camera scan opens the station app with
// the code already applied.
func (s *service) enrollBase(ctx context.Context) string {
	return coreservice.TenantBaseURL(ctx, s.domains, s.cfg.App.PublicBaseURL, s.logger)
}

func enrollURL(base, token string) string {
	return base + "/station?code=" + url.QueryEscape(token)
}

// existingProductIDs re-resolves caller-supplied product IDs through the
// RLS-scoped product repo and drops any that don't belong to the tenant —
// Postgres FK checks bypass RLS, so a raw insert would otherwise link a
// foreign product's id into the preset.
func (s *service) existingProductIDs(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return []string{}, nil
	}
	products, err := s.products.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("validate products: %w", err)
	}
	present := make(map[string]bool, len(products))
	for _, p := range products {
		present[p.ID] = true
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if present[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *service) encrypt(raw string) ([]byte, error) {
	if s.enc == nil {
		return nil, errors.New("stationqr: encryptor not configured")
	}
	blob, err := s.enc.EncryptString(raw)
	if err != nil {
		return nil, fmt.Errorf("encrypt token: %w", err)
	}
	return []byte(blob), nil
}

func (s *service) decrypt(tokenEnc []byte) (string, error) {
	if s.enc == nil {
		return "", errors.New("stationqr: encryptor not configured")
	}
	return s.enc.DecryptString(string(tokenEnc))
}

func (s *service) toPreset(row *ent.StationEnrollPreset, productIDs []string, base string) (Preset, error) {
	token, err := s.decrypt(row.TokenEnc)
	if err != nil {
		return Preset{}, fmt.Errorf("decrypt token: %w", err)
	}
	if productIDs == nil {
		productIDs = []string{}
	}
	return Preset{
		ID:         row.ID,
		Name:       row.Name,
		Disabled:   row.DisabledAt != nil,
		Token:      token,
		EnrollURL:  enrollURL(base, token),
		IssuedBy:   row.IssuedBy,
		ProductIDs: productIDs,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}, nil
}

func (s *service) ListPresets(ctx context.Context) ([]Preset, error) {
	rows, err := s.presets.List(ctx)
	if err != nil {
		return nil, err
	}
	links, err := s.presets.ListAllProducts(ctx)
	if err != nil {
		return nil, err
	}
	byPreset := make(map[string][]string, len(rows))
	for _, l := range links {
		byPreset[l.PresetID] = append(byPreset[l.PresetID], l.ProductID)
	}
	base := s.enrollBase(ctx)
	out := make([]Preset, 0, len(rows))
	for _, row := range rows {
		p, err := s.toPreset(row, byPreset[row.ID], base)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *service) CreatePreset(ctx context.Context, name string, productIDs []string, issuerUserID string) (Preset, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Preset{}, ErrNameRequired
	}
	raw, err := generateToken()
	if err != nil {
		return Preset{}, fmt.Errorf("generate token: %w", err)
	}
	blob, err := s.encrypt(raw)
	if err != nil {
		return Preset{}, err
	}
	productIDs, err = s.existingProductIDs(ctx, productIDs)
	if err != nil {
		return Preset{}, err
	}
	row, err := s.presets.Create(ctx, name, blob, issuerUserID)
	if err != nil {
		return Preset{}, err
	}
	if len(productIDs) > 0 {
		if err := s.presets.ReplaceProducts(ctx, row.ID, productIDs); err != nil {
			return Preset{}, err
		}
	}
	return s.toPreset(row, productIDs, s.enrollBase(ctx))
}

func (s *service) UpdatePreset(ctx context.Context, id string, in UpdatePresetInput) (Preset, error) {
	if _, err := s.presets.GetByID(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return Preset{}, ErrNotFound
		}
		return Preset{}, err
	}

	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return Preset{}, ErrNameRequired
		}
		if _, err := s.presets.Rename(ctx, id, name); err != nil {
			return Preset{}, err
		}
	}
	if in.Disabled != nil {
		if _, err := s.presets.SetDisabled(ctx, id, *in.Disabled); err != nil {
			return Preset{}, err
		}
	}
	if in.ProductIDs != nil {
		ids, err := s.existingProductIDs(ctx, *in.ProductIDs)
		if err != nil {
			return Preset{}, err
		}
		if err := s.presets.ReplaceProducts(ctx, id, ids); err != nil {
			return Preset{}, err
		}
	}
	return s.loadPreset(ctx, id)
}

func (s *service) RotatePreset(ctx context.Context, id string) (Preset, error) {
	if _, err := s.presets.GetByID(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return Preset{}, ErrNotFound
		}
		return Preset{}, err
	}
	raw, err := generateToken()
	if err != nil {
		return Preset{}, fmt.Errorf("generate token: %w", err)
	}
	blob, err := s.encrypt(raw)
	if err != nil {
		return Preset{}, err
	}
	if _, err := s.presets.SetToken(ctx, id, blob); err != nil {
		return Preset{}, err
	}
	return s.loadPreset(ctx, id)
}

func (s *service) DeletePreset(ctx context.Context, id string) error {
	if err := s.presets.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *service) loadPreset(ctx context.Context, id string) (Preset, error) {
	row, err := s.presets.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return Preset{}, ErrNotFound
		}
		return Preset{}, err
	}
	ids, err := s.productIDs(ctx, id)
	if err != nil {
		return Preset{}, err
	}
	return s.toPreset(row, ids, s.enrollBase(ctx))
}

func (s *service) productIDs(ctx context.Context, presetID string) ([]string, error) {
	links, err := s.presets.ListProducts(ctx, presetID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.ProductID)
	}
	return ids, nil
}

func (s *service) Enroll(ctx context.Context, token, deviceKey, name, model, os string) (EnrollResult, error) {
	presets, err := s.presets.List(ctx)
	if err != nil {
		return EnrollResult{}, err
	}

	// Resolve the token to a preset in constant time over the tenant's presets:
	// compare every candidate before deciding, so a match doesn't leak timing.
	var matched *ent.StationEnrollPreset
	for _, p := range presets {
		plain, err := s.decrypt(p.TokenEnc)
		if err != nil {
			return EnrollResult{}, err
		}
		if subtle.ConstantTimeCompare([]byte(plain), []byte(token)) == 1 {
			matched = p
		}
	}
	if matched == nil {
		return EnrollResult{}, ErrInvalidToken
	}
	if matched.DisabledAt != nil {
		return EnrollResult{}, ErrDisabled
	}

	if name == "" {
		name = "Station"
	}

	d, err := s.devices.UpsertPendingByDeviceKey(ctx, name, model, os, deviceKey, device.TypeSTATION)
	if err != nil {
		return EnrollResult{}, err
	}

	approved, sessionToken, err := s.approver.ApproveDirect(ctx, d, matched.IssuedBy)
	if err != nil {
		return EnrollResult{}, err
	}

	productIDs, err := s.productIDs(ctx, matched.ID)
	if err != nil {
		return EnrollResult{}, err
	}
	if err := s.stations.SetStationProducts(ctx, approved.ID, productIDs); err != nil {
		return EnrollResult{}, fmt.Errorf("assign preset products: %w", err)
	}

	return EnrollResult{Device: approved, Token: sessionToken}, nil
}
