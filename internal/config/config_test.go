package config

import (
	"testing"
)

func TestInitWithDefaultConfig(t *testing.T) {
	tmpDir := t.TempDir()

	cfg, err := Init(tmpDir)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if cfg.App.Name != "lemefy-bacen" {
		t.Errorf("expected App.Name 'lemefy-bacen', got %s", cfg.App.Name)
	}
	if cfg.App.Port != 8080 {
		t.Errorf("expected App.Port 8080, got %d", cfg.App.Port)
	}
	if cfg.Database.Path != "data/normas.db" {
		t.Errorf("expected Database.Path 'data/normas.db', got %s", cfg.Database.Path)
	}
	if !cfg.Meilisearch.Enabled {
		t.Error("expected Meilisearch.Enabled to be true")
	}
	if cfg.Meilisearch.Host != "http://localhost:7700" {
		t.Errorf("expected Meilisearch.Host 'http://localhost:7700', got %s", cfg.Meilisearch.Host)
	}
	if cfg.Meilisearch.IndexPrefix != "bcb_" {
		t.Errorf("expected Meilisearch.IndexPrefix 'bcb_', got %s", cfg.Meilisearch.IndexPrefix)
	}
	if cfg.Scraper.BaseURL != "https://www.bcb.gov.br/normativos" {
		t.Errorf("expected Scraper.BaseURL 'https://www.bcb.gov.br/normativos', got %s", cfg.Scraper.BaseURL)
	}
	if cfg.Scheduler.Enabled != true {
		t.Error("expected Scheduler.Enabled to be true")
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("expected Logging.Level 'info', got %s", cfg.Logging.Level)
	}
}

func TestGetDefaultConfig(t *testing.T) {
	cfg := getDefaultConfig()
	if cfg == nil {
		t.Fatal("getDefaultConfig returned nil")
	}
	if cfg.App.Name != "lemefy-bacen" {
		t.Errorf("expected App.Name 'lemefy-bacen', got %s", cfg.App.Name)
	}
	if cfg.App.Port != 8080 {
		t.Errorf("expected App.Port 8080, got %d", cfg.App.Port)
	}
	if cfg.Database.Path != "data/normas.db" {
		t.Errorf("expected Database.Path 'data/normas.db', got %s", cfg.Database.Path)
	}
	if cfg.Scraper.BaseURL != "https://www.bcb.gov.br/normativos" {
		t.Errorf("expected Scraper.BaseURL 'https://www.bcb.gov.br/normativos', got %s", cfg.Scraper.BaseURL)
	}
	if cfg.Scheduler.UpdateCron != "0 2 * * *" {
		t.Errorf("expected Scheduler.UpdateCron '0 2 * * *', got %s", cfg.Scheduler.UpdateCron)
	}
	if cfg.Meilisearch.IndexPrefix != "bcb_" {
		t.Errorf("expected Meilisearch.IndexPrefix 'bcb_', got %s", cfg.Meilisearch.IndexPrefix)
	}
}

func TestGetLogger(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := Init(tmpDir)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	logger := GetLogger()
	if logger == nil {
		t.Fatal("GetLogger returned nil")
	}
}

func TestGetReturnsConfig(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := Init(tmpDir)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	cfg := Get()
	if cfg == nil {
		t.Fatal("Get returned nil")
	}
	if cfg.App.Name != "lemefy-bacen" {
		t.Errorf("expected App.Name 'lemefy-bacen', got %s", cfg.App.Name)
	}
}