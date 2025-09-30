package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Config holds daemon configuration
type Config struct {
	// Weenect API credentials
	Username string `json:"username"`
	Password string `json:"password"`

	// Database configuration
	DatabasePath string `json:"database_path"`

	// Rate limiting (requests per second)
	RateLimit float64 `json:"rate_limit"`

	// Backfill configuration
	BackfillStartDate string `json:"backfill_start_date"` // YYYY-MM-DD format

	// Sync schedule (cron format or "nightly")
	SyncSchedule string `json:"sync_schedule"`

	// Logging
	LogLevel string `json:"log_level"` // debug, info, warn, error

	// HTTP API server
	HTTPPort    int  `json:"http_port"`
	HTTPEnabled bool `json:"http_enabled"`
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		DatabasePath:      "./weenect.db",
		RateLimit:         4.0, // 4 requests per second
		BackfillStartDate: time.Now().AddDate(0, 0, -30).Format("2006-01-02"), // Last 30 days
		SyncSchedule:      "0 2 * * *", // 2am daily (cron format)
		LogLevel:          "info",
		HTTPPort:          8080,
		HTTPEnabled:       true,
	}
}

// getDefaultConfigPaths returns list of default config file paths to check
func getDefaultConfigPaths() []string {
	homeDir, _ := os.UserHomeDir()
	return []string{
		"./config.json",
		"./weenect-daemon.json",
		homeDir + "/.config/weenect/config.json",
		homeDir + "/.weenect/config.json",
	}
}

// loadConfig loads configuration from file, env vars, and defaults
func loadConfig(configPath string) (*Config, error) {
	cfg := DefaultConfig()

	// Determine which config file to use
	var configFile string
	if configPath != "" {
		// Explicit config path provided
		configFile = configPath
	} else {
		// Try default locations
		for _, path := range getDefaultConfigPaths() {
			if _, err := os.Stat(path); err == nil {
				configFile = path
				break
			}
		}
	}

	// Load from config file if found
	if configFile != "" {
		data, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file %s: %w", configFile, err)
		}

		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file %s: %w", configFile, err)
		}
	}

	// Environment variables override config file
	if val := os.Getenv("WEENECT_USERNAME"); val != "" {
		cfg.Username = val
	}
	if val := os.Getenv("WEENECT_PASSWORD"); val != "" {
		cfg.Password = val
	}
	if val := os.Getenv("WEENECT_DATABASE_PATH"); val != "" {
		cfg.DatabasePath = val
	}
	if val := os.Getenv("WEENECT_RATE_LIMIT"); val != "" {
		var rateLimit float64
		if _, err := fmt.Sscanf(val, "%f", &rateLimit); err == nil {
			cfg.RateLimit = rateLimit
		}
	}
	if val := os.Getenv("WEENECT_BACKFILL_START_DATE"); val != "" {
		cfg.BackfillStartDate = val
	}
	if val := os.Getenv("WEENECT_SYNC_SCHEDULE"); val != "" {
		cfg.SyncSchedule = val
	}
	if val := os.Getenv("WEENECT_LOG_LEVEL"); val != "" {
		cfg.LogLevel = val
	}
	if val := os.Getenv("WEENECT_HTTP_PORT"); val != "" {
		var port int
		if _, err := fmt.Sscanf(val, "%d", &port); err == nil {
			cfg.HTTPPort = port
		}
	}
	if val := os.Getenv("WEENECT_HTTP_ENABLED"); val != "" {
		cfg.HTTPEnabled = val == "true" || val == "1"
	}

	// Validate required fields
	if cfg.Username == "" {
		return nil, fmt.Errorf("username is required (set WEENECT_USERNAME or use config file)")
	}
	if cfg.Password == "" {
		return nil, fmt.Errorf("password is required (set WEENECT_PASSWORD or use config file)")
	}

	return cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Username == "" {
		return fmt.Errorf("username is required")
	}
	if c.Password == "" {
		return fmt.Errorf("password is required")
	}
	if c.DatabasePath == "" {
		return fmt.Errorf("database_path is required")
	}
	if c.RateLimit <= 0 {
		return fmt.Errorf("rate_limit must be positive")
	}

	// Validate backfill date format if set
	if c.BackfillStartDate != "" {
		if _, err := time.Parse("2006-01-02", c.BackfillStartDate); err != nil {
			return fmt.Errorf("invalid backfill_start_date format (use YYYY-MM-DD): %w", err)
		}
	}

	return nil
}