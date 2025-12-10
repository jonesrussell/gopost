package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gopost/integration/internal/config"
	"github.com/gopost/integration/internal/logger"
)

type Client struct {
	url     string
	timeout time.Duration
	logger  logger.Logger
}

type CitiesResponse struct {
	Cities []City `json:"cities"`
	Count  int    `json:"count"`
}

type City struct {
	Name    string `json:"name"`
	Index   string `json:"index"`
	GroupID string `json:"group_id,omitempty"`
}

func NewClient(cfg *config.SourcesConfig, log logger.Logger) *Client {
	return &Client{
		url:     cfg.URL,
		timeout: cfg.Timeout,
		logger:  log,
	}
}

func (c *Client) GetCities(ctx context.Context) ([]config.CityConfig, error) {
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	url := fmt.Sprintf("%s/api/v1/cities", c.url)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	client := &http.Client{
		Timeout: c.timeout,
	}

	start := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(start)

	if err != nil {
		c.logger.Warn("Failed to fetch cities from sources service",
			logger.String("url", url),
			logger.Duration("duration", duration),
			logger.Error(err),
		)
		return nil, fmt.Errorf("fetch cities: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.logger.Warn("Sources service returned non-OK status",
			logger.String("url", url),
			logger.Int("status_code", resp.StatusCode),
			logger.Duration("duration", duration),
		)
		return nil, fmt.Errorf("sources service returned status %d", resp.StatusCode)
	}

	var citiesResp CitiesResponse
	if err := json.NewDecoder(resp.Body).Decode(&citiesResp); err != nil {
		c.logger.Error("Failed to decode cities response",
			logger.String("url", url),
			logger.Duration("duration", duration),
			logger.Error(err),
		)
		return nil, fmt.Errorf("decode response: %w", err)
	}

	c.logger.Info("Fetched cities from sources service",
		logger.String("url", url),
		logger.Int("city_count", citiesResp.Count),
		logger.Duration("duration", duration),
	)

	cities := make([]config.CityConfig, 0, len(citiesResp.Cities))
	for _, city := range citiesResp.Cities {
		cities = append(cities, config.CityConfig{
			Name:    city.Name,
			Index:   city.Index,
			GroupID: city.GroupID,
		})
	}

	return cities, nil
}

