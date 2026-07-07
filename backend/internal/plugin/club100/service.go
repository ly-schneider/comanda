package club100

import (
	"context"
	"encoding/json"
	"fmt"

	"backend/internal/plugin"
	"backend/internal/repository"
)

const Key = "club100"

const defaultMaxRedemptions = 2

var ErrProductNotFree = fmt.Errorf("product_not_free_for_club100")

type Person struct {
	ID               string `json:"id"`
	FirstName        string `json:"firstName"`
	LastName         string `json:"lastName"`
	TotalRedemptions int    `json:"totalRedemptions"`
	Remaining        int    `json:"remaining"`
	Max              int    `json:"max"`
}

type Settings struct {
	FreeProductIDs []string `json:"freeProductIds"`
	MaxRedemptions int      `json:"maxRedemptions"`
}

type Service interface {
	GetPeopleWithRedemptions(ctx context.Context) ([]Person, error)
	GetRemainingRedemptions(ctx context.Context, elvantoPersonID string) (remaining int, max int, err error)
	RecordRedemption(ctx context.Context, elvantoPersonID, elvantoPersonName, orderID string, qty int) error
	ValidateOrderForRedemption(ctx context.Context, orderID string) error
	GetSettings(ctx context.Context) (Settings, error)
	SetSettings(ctx context.Context, freeProductIDs *[]string, maxRedemptions *int) (Settings, error)
}

type config struct {
	// pointer so an absent key falls back to the default rather than reading as 0
	MaxRedemptions *int     `json:"maxRedemptions,omitempty"`
	FreeProductIDs []string `json:"freeProductIds,omitempty"`
}

func (c config) max() int {
	if c.MaxRedemptions == nil {
		return defaultMaxRedemptions
	}
	return *c.MaxRedemptions
}

type service struct {
	plugins     plugin.Service
	redemptions RedemptionRepository
	elvanto     ElvantoSearcher
	orderLines  repository.OrderLineRepository
	products    *repository.ProductRepository
}

func NewService(
	plugins plugin.Service,
	redemptions RedemptionRepository,
	elvanto ElvantoSearcher,
	orderLines repository.OrderLineRepository,
	products *repository.ProductRepository,
) Service {
	return &service{
		plugins:     plugins,
		redemptions: redemptions,
		elvanto:     elvanto,
		orderLines:  orderLines,
		products:    products,
	}
}

func (s *service) config(ctx context.Context) (config, error) {
	raw, err := s.plugins.Config(ctx, Key)
	if err != nil {
		return config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg config
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return config{}, fmt.Errorf("parse config: %w", err)
		}
	}
	return cfg, nil
}

func (s *service) credentials(ctx context.Context) (apiKey, groupID string, err error) {
	secrets, err := s.plugins.Secrets(ctx, Key)
	if err != nil {
		return "", "", fmt.Errorf("read secrets: %w", err)
	}
	return secrets["apiKey"], secrets["groupId"], nil
}

func (s *service) GetPeopleWithRedemptions(ctx context.Context) ([]Person, error) {
	apiKey, groupID, err := s.credentials(ctx)
	if err != nil {
		return nil, err
	}
	if apiKey == "" {
		return []Person{}, nil
	}

	cfg, err := s.config(ctx)
	if err != nil {
		return nil, err
	}
	maxRedemptions := cfg.max()

	people, err := s.elvanto.SearchPeople(ctx, apiKey, groupID)
	if err != nil {
		return nil, fmt.Errorf("search people: %w", err)
	}

	ids := make([]string, 0, len(people))
	for _, p := range people {
		ids = append(ids, p.ID)
	}
	totals, err := s.redemptions.GetTotalRedemptionsBatch(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get redemptions: %w", err)
	}

	result := make([]Person, 0, len(people))
	for _, p := range people {
		total := totals[p.ID]
		remaining := maxRedemptions - total
		if remaining < 0 {
			remaining = 0
		}
		result = append(result, Person{
			ID:               p.ID,
			FirstName:        p.FirstName,
			LastName:         p.LastName,
			TotalRedemptions: total,
			Remaining:        remaining,
			Max:              maxRedemptions,
		})
	}
	return result, nil
}

func (s *service) GetRemainingRedemptions(ctx context.Context, elvantoPersonID string) (int, int, error) {
	cfg, err := s.config(ctx)
	if err != nil {
		return 0, 0, err
	}
	maxRedemptions := cfg.max()

	total, err := s.redemptions.GetTotalRedemptions(ctx, elvantoPersonID)
	if err != nil {
		return 0, 0, fmt.Errorf("get total redemptions: %w", err)
	}

	remaining := maxRedemptions - total
	if remaining < 0 {
		remaining = 0
	}
	return remaining, maxRedemptions, nil
}

func (s *service) RecordRedemption(ctx context.Context, elvantoPersonID, elvantoPersonName, orderID string, qty int) error {
	if qty <= 0 {
		return nil
	}

	remaining, _, err := s.GetRemainingRedemptions(ctx, elvantoPersonID)
	if err != nil {
		return fmt.Errorf("check remaining: %w", err)
	}
	if remaining < qty {
		return fmt.Errorf("insufficient remaining redemptions: have %d, need %d", remaining, qty)
	}

	if _, err := s.redemptions.Create(ctx, elvantoPersonID, elvantoPersonName, orderID, qty); err != nil {
		return fmt.Errorf("create redemption: %w", err)
	}
	return nil
}

func (s *service) ValidateOrderForRedemption(ctx context.Context, orderID string) error {
	cfg, err := s.config(ctx)
	if err != nil {
		return err
	}
	if len(cfg.FreeProductIDs) == 0 {
		return fmt.Errorf("%w: no free products configured", ErrProductNotFree)
	}

	freeSet := make(map[string]bool, len(cfg.FreeProductIDs))
	for _, id := range cfg.FreeProductIDs {
		freeSet[id] = true
	}

	lines, err := s.orderLines.GetByOrderID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("get order lines: %w", err)
	}
	for _, line := range lines {
		if !freeSet[line.ProductID] {
			return fmt.Errorf("%w: product %s is not in the free products list", ErrProductNotFree, line.ProductID)
		}
	}
	return nil
}

func (s *service) GetSettings(ctx context.Context) (Settings, error) {
	cfg, err := s.config(ctx)
	if err != nil {
		return Settings{}, err
	}
	ids, err := s.existingProductIDs(ctx, cfg.FreeProductIDs)
	if err != nil {
		return Settings{}, err
	}
	return Settings{FreeProductIDs: ids, MaxRedemptions: cfg.max()}, nil
}

func (s *service) SetSettings(ctx context.Context, freeProductIDs *[]string, maxRedemptions *int) (Settings, error) {
	cfg, err := s.config(ctx)
	if err != nil {
		return Settings{}, err
	}

	if maxRedemptions != nil {
		if *maxRedemptions < 0 {
			return Settings{}, fmt.Errorf("max redemptions must be non-negative")
		}
		v := *maxRedemptions
		cfg.MaxRedemptions = &v
	}
	if freeProductIDs != nil {
		ids := make([]string, 0, len(*freeProductIDs))
		for _, id := range *freeProductIDs {
			if id != "" {
				ids = append(ids, id)
			}
		}
		cfg.FreeProductIDs = ids
	}

	raw, err := json.Marshal(cfg)
	if err != nil {
		return Settings{}, fmt.Errorf("marshal config: %w", err)
	}
	if _, err := s.plugins.SetConfig(ctx, Key, raw); err != nil {
		return Settings{}, fmt.Errorf("save config: %w", err)
	}
	return s.GetSettings(ctx)
}

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
