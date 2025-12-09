package config

import (
	"os"
	"testing"
)

func TestParseBool(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"true lowercase", "true", true},
		{"true uppercase", "TRUE", true},
		{"true mixed case", "True", true},
		{"one", "1", true},
		{"yes lowercase", "yes", true},
		{"yes uppercase", "YES", true},
		{"yes mixed case", "Yes", true},
		{"false lowercase", "false", false},
		{"false uppercase", "FALSE", false},
		{"zero", "0", false},
		{"no lowercase", "no", false},
		{"no uppercase", "NO", false},
		{"empty string", "", false},
		{"whitespace true", "  true  ", true},
		{"whitespace false", "  false  ", false},
		{"invalid value", "maybe", false},
		{"invalid value 2", "2", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseBool(tt.input)
			if result != tt.expected {
				t.Errorf("parseBool(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestConfigDebugFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{"true from env", "true", true},
		{"1 from env", "1", true},
		{"yes from env", "yes", true},
		{"false from env", "false", false},
		{"0 from env", "0", false},
		{"no env var", "", false}, // Should default to false
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("APP_DEBUG", tt.envValue)
			}

			// Create a minimal config for testing
			cfg := &Config{}
			if appDebug := os.Getenv("APP_DEBUG"); appDebug != "" {
				cfg.Debug = parseBool(appDebug)
			}

			if cfg.Debug != tt.expected {
				t.Errorf("Config.Debug = %v, want %v (APP_DEBUG=%q)", cfg.Debug, tt.expected, tt.envValue)
			}
		})
	}
}
