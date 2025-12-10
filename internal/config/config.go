package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Debug         bool                `yaml:"debug"` // Application debug mode (controls log level and format)
	Elasticsearch ElasticsearchConfig `yaml:"elasticsearch"`
	Drupal        DrupalConfig        `yaml:"drupal"`
	Redis         RedisConfig         `yaml:"redis"`
	Service       ServiceConfig       `yaml:"service"`
	Cities        []CityConfig        `yaml:"cities"`
	Sources       SourcesConfig       `yaml:"sources"` // Optional: Sources service configuration
}

type ElasticsearchConfig struct {
	URL      string `yaml:"url"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type DrupalConfig struct {
	URL           string `yaml:"url"`
	Username      string `yaml:"username"`        // Username for REST API Authentication
	Token         string `yaml:"token"`           // API key/token for authentication
	AuthMethod    string `yaml:"auth_method"`     // AUTH-METHOD header value (application ID)
	SkipTLSVerify bool   `yaml:"skip_tls_verify"` // Skip TLS certificate verification (development only)
}

type RedisConfig struct {
	URL      string `yaml:"url"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type ServiceConfig struct {
	CheckInterval time.Duration `yaml:"check_interval"`
	RateLimitRPS  int           `yaml:"rate_limit_rps"`
	LookbackHours int           `yaml:"lookback_hours"`
	CrimeKeywords []string      `yaml:"crime_keywords"`
	ContentType   string        `yaml:"content_type"`
	GroupType     string        `yaml:"group_type"`
	DedupTTL      time.Duration `yaml:"dedup_ttl"` // Default: 8760h (1 year)
}

type CityConfig struct {
	Name    string `yaml:"name"`
	Index   string `yaml:"index"`
	GroupID string `yaml:"group_id"`
}

type SourcesConfig struct {
	URL     string        `yaml:"url"`      // Sources service API URL (e.g., "http://localhost:8080")
	Timeout time.Duration `yaml:"timeout"`  // Request timeout (default: 5s)
	Enabled bool          `yaml:"enabled"`  // Enable fetching cities from sources service
}

// Validate checks if the configuration is valid and returns an error if not.
func (c *Config) Validate() error {
	if c.Elasticsearch.URL == "" {
		return errors.New("elasticsearch.url is required")
	}
	if c.Drupal.URL == "" {
		return errors.New("drupal.url is required")
	}
	if c.Drupal.Token == "" {
		return errors.New("drupal.token is required")
	}
	if c.Redis.URL == "" {
		return errors.New("redis.url is required")
	}
	if c.Service.RateLimitRPS <= 0 {
		return fmt.Errorf("service.rate_limit_rps must be positive, got %d", c.Service.RateLimitRPS)
	}
	if c.Service.CheckInterval <= 0 {
		return fmt.Errorf("service.check_interval must be positive, got %v", c.Service.CheckInterval)
	}
	if c.Service.DedupTTL < 0 {
		return fmt.Errorf("service.dedup_ttl must be non-negative, got %v", c.Service.DedupTTL)
	}
	// Cities are required either from config or sources service
	if !c.Sources.Enabled && len(c.Cities) == 0 {
		return errors.New("at least one city must be configured or sources service must be enabled")
	}
	if c.Sources.Enabled && c.Sources.URL == "" {
		return errors.New("sources.url is required when sources.enabled is true")
	}
	for i, city := range c.Cities {
		if city.Name == "" {
			return fmt.Errorf("cities[%d].name is required", i)
		}
		// group_id is optional - articles can be posted without a group
	}
	return nil
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Set defaults
	if cfg.Service.CheckInterval == 0 {
		cfg.Service.CheckInterval = 5 * time.Minute
	}
	if cfg.Service.RateLimitRPS == 0 {
		cfg.Service.RateLimitRPS = 10
	}
	// LookbackHours: 0 means no date filter, search all articles
	// If not specified, default to 24 hours for backward compatibility
	// We use -1 as a sentinel to detect if it was explicitly set
	// For now, we'll allow 0 to mean "no filter" and only set default if truly unset
	// Note: YAML parsing makes it hard to distinguish unset from 0, so we rely on
	// the service logic to handle 0 as "no date filter"
	if len(cfg.Service.CrimeKeywords) == 0 {
		cfg.Service.CrimeKeywords = []string{
			"police", "arrest", "charged", "court",
			"murder", "assault", "robbery", "theft",
			"crime", "criminal", "suspect", "victim",
			"investigation", "warrant", "sentence",
		}
	}
	if cfg.Service.ContentType == "" {
		cfg.Service.ContentType = "node--article"
	}
	if cfg.Service.GroupType == "" {
		cfg.Service.GroupType = "group--crime_news"
	}
	const hoursPerYear = 8760
	if cfg.Service.DedupTTL == 0 {
		cfg.Service.DedupTTL = hoursPerYear * time.Hour // 1 year default
	}
	if cfg.Sources.Timeout == 0 {
		cfg.Sources.Timeout = 5 * time.Second
	}

	// Override with environment variables if present
	if esURL := os.Getenv("ES_URL"); esURL != "" {
		cfg.Elasticsearch.URL = esURL
	}
	if drupalURL := os.Getenv("DRUPAL_URL"); drupalURL != "" {
		cfg.Drupal.URL = drupalURL
	}
	if drupalUsername := os.Getenv("DRUPAL_USERNAME"); drupalUsername != "" {
		cfg.Drupal.Username = drupalUsername
	}
	if drupalToken := os.Getenv("DRUPAL_TOKEN"); drupalToken != "" {
		cfg.Drupal.Token = drupalToken
	}
	if drupalAuthMethod := os.Getenv("DRUPAL_AUTH_METHOD"); drupalAuthMethod != "" {
		cfg.Drupal.AuthMethod = drupalAuthMethod
	}
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		cfg.Redis.URL = redisURL
	}
	if sourcesURL := os.Getenv("SOURCES_URL"); sourcesURL != "" {
		cfg.Sources.URL = sourcesURL
	}
	if sourcesEnabled := os.Getenv("SOURCES_ENABLED"); sourcesEnabled != "" {
		cfg.Sources.Enabled = parseBool(sourcesEnabled)
	}
	// Parse APP_DEBUG environment variable
	if appDebug := os.Getenv("APP_DEBUG"); appDebug != "" {
		cfg.Debug = parseBool(appDebug)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// LoadWithSources loads configuration and optionally fetches cities from sources service.
// If sources service is enabled and cities are fetched successfully, they override the config file cities.
func LoadWithSources(path string, sourcesClient interface{ GetCities(context.Context) ([]CityConfig, error) }) (*Config, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}

	// If sources service is enabled, try to fetch cities
	if cfg.Sources.Enabled && sourcesClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Sources.Timeout)
		defer cancel()

		cities, err := sourcesClient.GetCities(ctx)
		if err != nil {
			// Log warning but don't fail - fallback to config file cities
			// In production, you might want to use a logger here
			_ = err
			return cfg, nil
		}

		if len(cities) > 0 {
			cfg.Cities = cities
		}
	}

	return cfg, nil
}

// parseBool parses a string value as a boolean.
// Returns true for "true", "1", "yes" (case-insensitive), false otherwise.
// This function handles common boolean string representations.
func parseBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "true" || s == "1" || s == "yes"
}
