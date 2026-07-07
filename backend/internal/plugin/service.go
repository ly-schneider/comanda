package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"backend/internal/crypto"
	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/repository"
	"backend/internal/response"

	"go.uber.org/zap"
)

var (
	ErrUnknownPlugin  = errors.New("plugin: unknown plugin key")
	ErrPluginDisabled = errors.New("plugin: not enabled for tenant")
	ErrNoPaymentHook  = errors.New("plugin: does not implement PaymentHook")
)

const enabledCacheTTL = 5 * time.Second

type PublicPlugin struct {
	Key    string
	Name   string
	Config json.RawMessage
}

type TenantPluginState struct {
	Key                 string
	Name                string
	Enabled             bool
	Config              json.RawMessage
	SecretsConfiguredAt *time.Time
}

// Nil fields are left unchanged; a non-nil Secrets map replaces stored secrets wholesale.
type UpdateInput struct {
	Enabled *bool
	Config  json.RawMessage
	Secrets map[string]string
}

type Service interface {
	Enabled(ctx context.Context, key string) (bool, error)
	PaymentHook(ctx context.Context, key string) (PaymentHook, error)
	RequirePlugin(key string) func(http.Handler) http.Handler
	ListPublic(ctx context.Context) ([]PublicPlugin, error)
	Config(ctx context.Context, key string) (json.RawMessage, error)
	SetConfig(ctx context.Context, key string, config json.RawMessage) (json.RawMessage, error)
	Secrets(ctx context.Context, key string) (map[string]string, error)
	ListForTenant(ctx context.Context, tenantID string) ([]TenantPluginState, error)
	SetForTenant(ctx context.Context, tenantID, key string, in UpdateInput) (TenantPluginState, error)
}

type cacheEntry struct {
	enabled   bool
	expiresAt time.Time
}

type service struct {
	registry map[string]Plugin
	order    []Plugin
	repo     repository.TenantPluginRepository
	enc      *crypto.Encryptor
	logger   *zap.Logger

	mu    sync.RWMutex
	cache map[string]cacheEntry
}

func NewService(plugins []Plugin, repo repository.TenantPluginRepository, enc *crypto.Encryptor, logger *zap.Logger) Service {
	reg := make(map[string]Plugin, len(plugins))
	for _, p := range plugins {
		reg[p.Key()] = p
	}
	return &service{
		registry: reg,
		order:    plugins,
		repo:     repo,
		enc:      enc,
		logger:   logger,
		cache:    make(map[string]cacheEntry),
	}
}

func (s *service) Enabled(ctx context.Context, key string) (bool, error) {
	if _, ok := s.registry[key]; !ok {
		return false, nil
	}
	tenantID, ok := repository.TenantIDFromContext(ctx)
	if !ok || tenantID == "" {
		return false, nil
	}

	ck := tenantID + "\x00" + key
	s.mu.RLock()
	if e, ok := s.cache[ck]; ok && time.Now().Before(e.expiresAt) {
		s.mu.RUnlock()
		return e.enabled, nil
	}
	s.mu.RUnlock()

	enabled := false
	row, err := s.repo.GetByKey(ctx, key)
	switch {
	case err == nil:
		enabled = row.Enabled
	case errors.Is(err, repository.ErrNotFound):
		enabled = false
	default:
		return false, err
	}

	s.mu.Lock()
	s.cache[ck] = cacheEntry{enabled: enabled, expiresAt: time.Now().Add(enabledCacheTTL)}
	s.mu.Unlock()
	return enabled, nil
}

func (s *service) PaymentHook(ctx context.Context, key string) (PaymentHook, error) {
	p, ok := s.registry[key]
	if !ok {
		return nil, ErrUnknownPlugin
	}
	enabled, err := s.Enabled(ctx, key)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, ErrPluginDisabled
	}
	hook, ok := p.(PaymentHook)
	if !ok {
		return nil, ErrNoPaymentHook
	}
	return hook, nil
}

func (s *service) RequirePlugin(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			enabled, err := s.Enabled(r.Context(), key)
			if err != nil {
				s.logger.Warn("plugin gate check failed", zap.String("plugin", key), zap.Error(err))
			}
			if !enabled {
				response.WriteJSON(w, http.StatusNotFound, generated.Error{Code: "not_found", Message: "Not found"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *service) ListPublic(ctx context.Context) ([]PublicPlugin, error) {
	rows, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	enabled := make(map[string]*ent.TenantPlugin, len(rows))
	for _, row := range rows {
		enabled[row.PluginKey] = row
	}
	out := make([]PublicPlugin, 0, len(rows))
	for _, p := range s.order {
		row, ok := enabled[p.Key()]
		if !ok {
			continue
		}
		out = append(out, PublicPlugin{Key: p.Key(), Name: p.Name(), Config: normConfig(row.Config)})
	}
	return out, nil
}

func (s *service) Config(ctx context.Context, key string) (json.RawMessage, error) {
	row, err := s.repo.GetByKey(ctx, key)
	switch {
	case err == nil:
		return normConfig(row.Config), nil
	case errors.Is(err, repository.ErrNotFound):
		return json.RawMessage("{}"), nil
	default:
		return nil, err
	}
}

func (s *service) SetConfig(ctx context.Context, key string, config json.RawMessage) (json.RawMessage, error) {
	tenantID, ok := repository.TenantIDFromContext(ctx)
	if !ok || tenantID == "" {
		return nil, errors.New("plugin: no tenant in context")
	}
	state, err := s.SetForTenant(ctx, tenantID, key, UpdateInput{Config: config})
	if err != nil {
		return nil, err
	}
	return state.Config, nil
}

func (s *service) Secrets(ctx context.Context, key string) (map[string]string, error) {
	row, err := s.repo.GetByKey(ctx, key)
	if err != nil {
		return nil, err
	}
	if row.SecretsEnc == nil || len(*row.SecretsEnc) == 0 {
		return map[string]string{}, nil
	}
	if s.enc == nil {
		return nil, errors.New("plugin: encryptor not configured")
	}
	plaintext, err := s.enc.DecryptString(string(*row.SecretsEnc))
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	if err := json.Unmarshal([]byte(plaintext), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *service) ListForTenant(ctx context.Context, tenantID string) ([]TenantPluginState, error) {
	rows, err := s.repo.ListByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]*ent.TenantPlugin, len(rows))
	for _, row := range rows {
		byKey[row.PluginKey] = row
	}
	out := make([]TenantPluginState, 0, len(s.order))
	for _, p := range s.order {
		st := TenantPluginState{Key: p.Key(), Name: p.Name(), Config: json.RawMessage("{}")}
		if row, ok := byKey[p.Key()]; ok {
			st.Enabled = row.Enabled
			st.Config = normConfig(row.Config)
			st.SecretsConfiguredAt = row.SecretsConfiguredAt
		}
		out = append(out, st)
	}
	return out, nil
}

func (s *service) SetForTenant(ctx context.Context, tenantID, key string, in UpdateInput) (TenantPluginState, error) {
	p, ok := s.registry[key]
	if !ok {
		return TenantPluginState{}, ErrUnknownPlugin
	}

	rows, err := s.repo.ListByTenant(ctx, tenantID)
	if err != nil {
		return TenantPluginState{}, err
	}
	var existing *ent.TenantPlugin
	for _, row := range rows {
		if row.PluginKey == key {
			existing = row
			break
		}
	}

	up := repository.TenantPluginUpsert{PluginKey: key}
	switch {
	case in.Enabled != nil:
		up.Enabled = *in.Enabled
	case existing != nil:
		up.Enabled = existing.Enabled
	}
	switch {
	case in.Config != nil:
		up.Config = in.Config
	case existing != nil && len(existing.Config) > 0:
		up.Config = existing.Config
	default:
		up.Config = json.RawMessage("{}")
	}
	if in.Secrets != nil {
		blob, err := s.encryptSecrets(in.Secrets)
		if err != nil {
			return TenantPluginState{}, err
		}
		now := time.Now()
		up.SecretsEnc = blob
		up.SecretsConfiguredAt = &now
	}

	row, err := s.repo.Upsert(ctx, tenantID, up)
	if err != nil {
		return TenantPluginState{}, err
	}
	s.evict(tenantID, key)

	return TenantPluginState{
		Key:                 p.Key(),
		Name:                p.Name(),
		Enabled:             row.Enabled,
		Config:              normConfig(row.Config),
		SecretsConfiguredAt: row.SecretsConfiguredAt,
	}, nil
}

func (s *service) encryptSecrets(secrets map[string]string) ([]byte, error) {
	if s.enc == nil {
		return nil, errors.New("plugin: encryptor not configured")
	}
	raw, err := json.Marshal(secrets)
	if err != nil {
		return nil, err
	}
	blob, err := s.enc.EncryptString(string(raw))
	if err != nil {
		return nil, err
	}
	return []byte(blob), nil
}

func (s *service) evict(tenantID, key string) {
	s.mu.Lock()
	delete(s.cache, tenantID+"\x00"+key)
	s.mu.Unlock()
}

func normConfig(cfg json.RawMessage) json.RawMessage {
	if len(cfg) == 0 {
		return json.RawMessage("{}")
	}
	return cfg
}
