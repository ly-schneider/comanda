package club100

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type ElvantoSearcher interface {
	SearchPeople(ctx context.Context, apiKey, groupID string) ([]ElvantoPerson, error)
}

type ElvantoClient struct {
	http   *http.Client
	logger *zap.Logger
}

type ElvantoPerson struct {
	ID        string `json:"id"`
	FirstName string `json:"firstname"`
	LastName  string `json:"lastname"`
}

func NewElvantoClient(logger *zap.Logger) *ElvantoClient {
	return &ElvantoClient{
		http:   &http.Client{Timeout: 30 * time.Second},
		logger: logger,
	}
}

func (c *ElvantoClient) SearchPeople(ctx context.Context, apiKey, groupID string) ([]ElvantoPerson, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("elvanto not configured")
	}

	reqBody := map[string]any{
		"page":      1,
		"page_size": 100,
		"search": map[string]any{
			"groups": groupID,
		},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.elvanto.com/v1/people/search.json", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth(apiKey, "")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		c.logger.Error("elvanto request failed", zap.Error(err))
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		c.logger.Error("elvanto API error", zap.Int("status", resp.StatusCode), zap.String("body", string(body)))
		return nil, fmt.Errorf("elvanto API returned %d", resp.StatusCode)
	}

	var result elvantoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		c.logger.Error("elvanto decode error", zap.Error(err))
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if result.Status != "ok" {
		c.logger.Error("elvanto status not ok", zap.String("status", result.Status))
		return nil, fmt.Errorf("elvanto error: %s", result.Status)
	}

	return result.People.Person, nil
}

type elvantoResponse struct {
	Status string `json:"status"`
	People struct {
		Person []ElvantoPerson `json:"person"`
	} `json:"people"`
}
