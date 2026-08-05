package cmds

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestOutputJSON(t *testing.T) {
	data := map[string]string{"key": "value"}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputJSON(data)
	if err != nil {
		t.Fatalf("outputJSON failed: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}

	if result["key"] != "value" {
		t.Errorf("expected key=value, got %s", result["key"])
	}
}

func TestNewRootCmd(t *testing.T) {
	cmd := NewRootCmd()
	if cmd == nil {
		t.Fatal("NewRootCmd returned nil")
	}
	if cmd.Use != "bacen" {
		t.Errorf("expected Use 'bacen', got %s", cmd.Use)
	}
}

func TestNewVersionCmd(t *testing.T) {
	cmd := NewVersionCmd()
	if cmd == nil {
		t.Fatal("NewVersionCmd returned nil")
	}
	if cmd.Use != "version" {
		t.Errorf("expected Use 'version', got %s", cmd.Use)
	}
}

func TestNewConfigCmd(t *testing.T) {
	cmd := NewConfigCmd()
	if cmd == nil {
		t.Fatal("NewConfigCmd returned nil")
	}
	if cmd.Use != "config" {
		t.Errorf("expected Use 'config', got %s", cmd.Use)
	}
}

func TestNewStatsCmd(t *testing.T) {
	cmd := NewStatsCmd()
	if cmd == nil {
		t.Fatal("NewStatsCmd returned nil")
	}
	if cmd.Use != "stats" {
		t.Errorf("expected Use 'stats', got %s", cmd.Use)
	}
}

func TestNewNormasCmd(t *testing.T) {
	cmd := NewNormasCmd()
	if cmd == nil {
		t.Fatal("NewNormasCmd returned nil")
	}
	if cmd.Use != "normas" {
		t.Errorf("expected Use 'normas', got %s", cmd.Use)
	}
}

func TestNewScrapeCmd(t *testing.T) {
	cmd := NewScrapeCmd()
	if cmd == nil {
		t.Fatal("NewScrapeCmd returned nil")
	}
	if cmd.Use != "scrape" {
		t.Errorf("expected Use 'scrape', got %s", cmd.Use)
	}
}

func TestNewSchedulerCmd(t *testing.T) {
	cmd := NewSchedulerCmd()
	if cmd == nil {
		t.Fatal("NewSchedulerCmd returned nil")
	}
	if cmd.Use != "scheduler" {
		t.Errorf("expected Use 'scheduler', got %s", cmd.Use)
	}
}

func TestNewServeCmd(t *testing.T) {
	cmd := NewServeCmd()
	if cmd == nil {
		t.Fatal("NewServeCmd returned nil")
	}
	if cmd.Use != "serve" {
		t.Errorf("expected Use 'serve', got %s", cmd.Use)
	}
}