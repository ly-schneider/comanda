package club100

import (
	"context"
	"encoding/json"
	"testing"

	"backend/internal/generated/ent"
	"backend/internal/plugin"
	"backend/internal/repository"

	"github.com/stretchr/testify/require"
)

type stubPlugins struct {
	plugin.Service
	config  json.RawMessage
	secrets map[string]string
}

func (s *stubPlugins) Config(context.Context, string) (json.RawMessage, error) {
	if len(s.config) == 0 {
		return json.RawMessage("{}"), nil
	}
	return s.config, nil
}
func (s *stubPlugins) SetConfig(_ context.Context, _ string, cfg json.RawMessage) (json.RawMessage, error) {
	s.config = cfg
	return cfg, nil
}
func (s *stubPlugins) Secrets(context.Context, string) (map[string]string, error) {
	return s.secrets, nil
}

type fakeProducts struct{ existing map[string]bool }

func (f fakeProducts) GetByIDs(_ context.Context, ids []string) ([]*ent.Product, error) {
	out := []*ent.Product{}
	for _, id := range ids {
		if f.existing[id] {
			out = append(out, &ent.Product{ID: id})
		}
	}
	return out, nil
}

type fakeRedemptions struct {
	totals      map[string]int
	batchCalls  int
	singleCalls int
	created     int
}

func (f *fakeRedemptions) Create(context.Context, string, string, string, int) (*ent.Club100Redemption, error) {
	f.created++
	return &ent.Club100Redemption{}, nil
}
func (f *fakeRedemptions) GetTotalRedemptions(_ context.Context, id string) (int, error) {
	f.singleCalls++
	return f.totals[id], nil
}
func (f *fakeRedemptions) GetTotalRedemptionsBatch(_ context.Context, ids []string) (map[string]int, error) {
	f.batchCalls++
	out := make(map[string]int, len(ids))
	for _, id := range ids {
		out[id] = f.totals[id]
	}
	return out, nil
}
func (f *fakeRedemptions) GetByOrderID(context.Context, string) ([]*ent.Club100Redemption, error) {
	return nil, nil
}

type stubElvanto struct{ people []ElvantoPerson }

func (s stubElvanto) SearchPeople(context.Context, string, string) ([]ElvantoPerson, error) {
	return s.people, nil
}

type fakeOrderLines struct {
	repository.OrderLineRepository
	lines []*ent.OrderLine
}

func (f fakeOrderLines) GetByOrderID(context.Context, string) ([]*ent.OrderLine, error) {
	return f.lines, nil
}

func newService(t *testing.T, cfg string, red *fakeRedemptions, elv stubElvanto, lines fakeOrderLines) Service {
	t.Helper()
	plugins := &stubPlugins{config: json.RawMessage(cfg), secrets: map[string]string{"apiKey": "k", "groupId": "g"}}
	return NewService(plugins, red, elv, lines, nil)
}

func TestService_GetPeopleWithRedemptions_NoNPlusOne(t *testing.T) {
	people := []ElvantoPerson{
		{ID: "p1", FirstName: "Alice", LastName: "A"},
		{ID: "p2", FirstName: "Bob", LastName: "B"},
		{ID: "p3", FirstName: "Carol", LastName: "C"},
	}
	red := &fakeRedemptions{totals: map[string]int{"p1": 2, "p2": 1}}
	svc := newService(t, `{"maxRedemptions":2}`, red, stubElvanto{people: people}, fakeOrderLines{})

	result, err := svc.GetPeopleWithRedemptions(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 3)

	require.Equal(t, 1, red.batchCalls, "must batch redemption counts")
	require.Zero(t, red.singleCalls, "must not query per member")

	byID := map[string]Person{}
	for _, p := range result {
		byID[p.ID] = p
	}
	require.Equal(t, 0, byID["p1"].Remaining, "p1 at cap")
	require.Equal(t, 1, byID["p2"].Remaining)
	require.Equal(t, 2, byID["p3"].Remaining, "no redemptions yet")
	require.Equal(t, 2, byID["p1"].Max)
}

func TestService_GetPeople_UnconfiguredReturnsEmpty(t *testing.T) {
	plugins := &stubPlugins{config: json.RawMessage(`{}`), secrets: map[string]string{}}
	svc := NewService(plugins, &fakeRedemptions{}, stubElvanto{}, fakeOrderLines{}, nil)
	result, err := svc.GetPeopleWithRedemptions(context.Background())
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestService_ValidateOrderForRedemption(t *testing.T) {
	cfg := `{"maxRedemptions":2,"freeProductIds":["prd1","prd2"]}`

	free := fakeOrderLines{lines: []*ent.OrderLine{{ProductID: "prd1"}, {ProductID: "prd2"}}}
	svc := newService(t, cfg, &fakeRedemptions{}, stubElvanto{}, free)
	require.NoError(t, svc.ValidateOrderForRedemption(context.Background(), "o1"))

	mixed := fakeOrderLines{lines: []*ent.OrderLine{{ProductID: "prd1"}, {ProductID: "prd9"}}}
	svc = newService(t, cfg, &fakeRedemptions{}, stubElvanto{}, mixed)
	require.ErrorIs(t, svc.ValidateOrderForRedemption(context.Background(), "o1"), ErrProductNotFree)

	empty := newService(t, `{"maxRedemptions":2}`, &fakeRedemptions{}, stubElvanto{}, free)
	require.ErrorIs(t, empty.ValidateOrderForRedemption(context.Background(), "o1"), ErrProductNotFree)
}

func TestService_SetSettings(t *testing.T) {
	ctx := context.Background()
	plugins := &stubPlugins{config: json.RawMessage(`{"maxRedemptions":2,"freeProductIds":["prd1"]}`)}
	products := fakeProducts{existing: map[string]bool{"prd1": true, "prd2": true}}
	svc := NewService(plugins, &fakeRedemptions{}, stubElvanto{}, fakeOrderLines{}, products)

	five := 5
	s, err := svc.SetSettings(ctx, nil, &five)
	require.NoError(t, err)
	require.Equal(t, 5, s.MaxRedemptions)
	require.Equal(t, []string{"prd1"}, s.FreeProductIDs, "free products preserved on a max-only update")

	ids := []string{"prd1", "prd2", "", "prd9"}
	s, err = svc.SetSettings(ctx, &ids, nil)
	require.NoError(t, err)
	require.Equal(t, 5, s.MaxRedemptions, "max preserved on a free-products-only update")
	require.Equal(t, []string{"prd1", "prd2"}, s.FreeProductIDs, "empty dropped, unknown product filtered at read")

	neg := -1
	_, err = svc.SetSettings(ctx, nil, &neg)
	require.Error(t, err, "negative max rejected")
}

func TestService_GetSettings_FiltersUnknownProducts(t *testing.T) {
	plugins := &stubPlugins{config: json.RawMessage(`{"maxRedemptions":3,"freeProductIds":["prd1","gone"]}`)}
	products := fakeProducts{existing: map[string]bool{"prd1": true}}
	svc := NewService(plugins, &fakeRedemptions{}, stubElvanto{}, fakeOrderLines{}, products)

	s, err := svc.GetSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, 3, s.MaxRedemptions)
	require.Equal(t, []string{"prd1"}, s.FreeProductIDs)
}

func TestService_RecordRedemption_EnforcesCap(t *testing.T) {
	ctx := context.Background()
	red := &fakeRedemptions{totals: map[string]int{"full": 2, "some": 1}}
	svc := newService(t, `{"maxRedemptions":2}`, red, stubElvanto{}, fakeOrderLines{})

	require.NoError(t, svc.RecordRedemption(ctx, "some", "S", "o1", 1))
	require.Equal(t, 1, red.created)

	require.Error(t, svc.RecordRedemption(ctx, "full", "F", "o2", 1), "over the daily cap")
	require.Equal(t, 1, red.created, "no row written when over cap")

	require.NoError(t, svc.RecordRedemption(ctx, "some", "S", "o3", 0))
	require.Equal(t, 1, red.created, "zero quantity is a no-op")
}
