package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Elasticsearch ElasticsearchConfig `yaml:"elasticsearch"`
	Drupal        DrupalConfig        `yaml:"drupal"`
	Redis         RedisConfig         `yaml:"redis"`
	Service       ServiceConfig       `yaml:"service"`
	Cities        []CityConfig        `yaml:"cities"`
}

type ElasticsearchConfig struct {
	URL      string `yaml:"url"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type DrupalConfig struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
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
}

type CityConfig struct {
	Name    string `yaml:"name"`
	Index   string `yaml:"index"`
	GroupID string `yaml:"group_id"`
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
	if cfg.Service.LookbackHours == 0 {
		cfg.Service.LookbackHours = 24
	}
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

	// Override with environment variables if present
	if esURL := os.Getenv("ES_URL"); esURL != "" {
		cfg.Elasticsearch.URL = esURL
	}
	if drupalURL := os.Getenv("DRUPAL_URL"); drupalURL != "" {
		cfg.Drupal.URL = drupalURL
	}
	if drupalToken := os.Getenv("DRUPAL_TOKEN"); drupalToken != "" {
		cfg.Drupal.Token = drupalToken
	}
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		cfg.Redis.URL = redisURL
	}

	return &cfg, nil
}
