package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// Config represents the application configuration
type Config struct {
	App struct {
		Name    string `mapstructure:"name"`
		Version string `mapstructure:"version"`
		Env     string `mapstructure:"env"`
		Port    int    `mapstructure:"port"`
	}
	Database struct {
		Path string `mapstructure:"path"`
	}
	Scraper struct {
		BaseURL        string `mapstructure:"base_url"`
		Timeout        int    `mapstructure:"timeout"`
		MaxDepth       int    `mapstructure:"max_depth"`
		Concurrency    int    `mapstructure:"concurrency"`
		UserAgent      string `mapstructure:"user_agent"`
		RequestDelay   int    `mapstructure:"request_delay_ms"`
	}
	Scheduler struct {
		Enabled         bool   `mapstructure:"enabled"`
		UpdateCron      string `mapstructure:"update_cron"`
		CleanupCron     string `mapstructure:"cleanup_cron"`
		CleanupDays     int    `mapstructure:"cleanup_days"`
	}
	Logging struct {
		Level  string `mapstructure:"level"`
		Format string `mapstructure:"format"`
		File   string `mapstructure:"file"`
	}
}

var (
	config *Config
	logger *logrus.Logger
)

// Init initializes the configuration
func Init(configPath string) (*Config, error) {
	if config != nil {
		return config, nil
	}

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(configPath)
	viper.AddConfigPath(".")
	viper.AddConfigPath(filepath.Dir(configPath))

	// Set default values
	viper.SetDefault("app.name", "lemefy-bacen")
	viper.SetDefault("app.version", "1.0.0")
	viper.SetDefault("app.env", "development")
	viper.SetDefault("app.port", 8080)
	viper.SetDefault("database.path", "data/normas.db")
	viper.SetDefault("scraper.base_url", "https://www.bcb.gov.br/normativos")
	viper.SetDefault("scraper.timeout", 30)
	viper.SetDefault("scraper.max_depth", 3)
	viper.SetDefault("scraper.concurrency", 4)
	viper.SetDefault("scraper.user_agent", "Mozilla/5.0 (compatible; LemefyBacenScraper/1.0)")
	viper.SetDefault("scraper.request_delay_ms", 100)
	viper.SetDefault("scheduler.enabled", true)
	viper.SetDefault("scheduler.update_cron", "0 2 * * *") // Every day at 2 AM
	viper.SetDefault("scheduler.cleanup_cron", "0 3 * * 0") // Every Sunday at 3 AM
	viper.SetDefault("scheduler.cleanup_days", 365)
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "text")
	viper.SetDefault("logging.file", "logs/scraper.log")

	// Bind environment variables
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetEnvPrefix("LEMEFY")

	// Read configuration
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Initialize logger
	logger = initLogger(cfg.Logging)

	config = &cfg
	return config, nil
}

// Get returns the current configuration
func Get() *Config {
	if config == nil {
		// Try to initialize with default values
		_, err := Init(".")
		if err != nil {
			logger.Warn("Failed to initialize config, using defaults")
			return getDefaultConfig()
		}
	}
	return config
}

// GetLogger returns the logger instance
func GetLogger() *logrus.Logger {
	return logger
}

func initLogger(cfg struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	File   string `mapstructure:"file"`
}) *logrus.Logger {
	logger := logrus.New()

	// Set log level
	level, err := logrus.ParseLevel(cfg.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	// Set format
	if cfg.Format == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{})
	} else {
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			DisableColors:  false,
		})
	}

	// Set output
	if cfg.File != "" {
		// Ensure directory exists
		dir := filepath.Dir(cfg.File)
		if err := os.MkdirAll(dir, 0755); err != nil {
			logger.Warnf("Failed to create log directory: %v", err)
		} else {
			file, err := os.OpenFile(cfg.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				logger.Warnf("Failed to open log file: %v", err)
			} else {
				logger.SetOutput(file)
			}
		}
	}

	return logger
}

func getDefaultConfig() *Config {
	return &Config{
		App: struct {
			Name    string `mapstructure:"name"`
			Version string `mapstructure:"version"`
			Env     string `mapstructure:"env"`
			Port    int    `mapstructure:"port"`
		}{
			Name:    "lemefy-bacen",
			Version: "1.0.0",
			Env:     "development",
			Port:    8080,
		},
		Database: struct {
			Path string `mapstructure:"path"`
		}{
			Path: "data/normas.db",
		},
		Scraper: struct {
			BaseURL     string `mapstructure:"base_url"`
			Timeout     int    `mapstructure:"timeout"`
			MaxDepth    int    `mapstructure:"max_depth"`
			Concurrency int    `mapstructure:"concurrency"`
			UserAgent   string `mapstructure:"user_agent"`
			RequestDelay int   `mapstructure:"request_delay_ms"`
		}{
			BaseURL:      "https://www.bcb.gov.br/normativos",
			Timeout:      30,
			MaxDepth:     3,
			Concurrency:  4,
			UserAgent:    "Mozilla/5.0 (compatible; LemefyBacenScraper/1.0)",
			RequestDelay: 100,
		},
		Scheduler: struct {
			Enabled     bool   `mapstructure:"enabled"`
			UpdateCron  string `mapstructure:"update_cron"`
			CleanupCron string `mapstructure:"cleanup_cron"`
			CleanupDays int    `mapstructure:"cleanup_days"`
		}{
			Enabled:     true,
			UpdateCron:  "0 2 * * *",
			CleanupCron: "0 3 * * 0",
			CleanupDays: 365,
		},
		Logging: struct {
			Level  string `mapstructure:"level"`
			Format string `mapstructure:"format"`
			File   string `mapstructure:"file"`
		}{
			Level:  "info",
			Format: "text",
			File:   "logs/scraper.log",
		},
	}
}

// GetUpdateInterval returns the time until next update
func (c *Config) GetUpdateInterval() time.Duration {
	// Parse cron expression to get next execution time
	// For simplicity, return 24 hours if we can't parse
	return 24 * time.Hour
}
