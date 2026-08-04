package mcp

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want map[string]any
	}{
		{"empty", json.RawMessage(`{}`), map[string]any{}},
		{"string value", json.RawMessage(`{"tipo":"Resolução"}`), map[string]any{"tipo": "Resolução"}},
		{"int value", json.RawMessage(`{"days":30}`), map[string]any{"days": float64(30)}},
		{"bool value", json.RawMessage(`{"recent":true}`), map[string]any{"recent": true}},
		{"invalid json", json.RawMessage(`not json`), map[string]any{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseArgs(tt.raw)
			for k, v := range tt.want {
				if gv, ok := got[k]; !ok || gv != v {
					t.Errorf("parseArgs() key %q: expected %v, got %v", k, v, gv)
				}
			}
		})
	}
}

func TestTextResult(t *testing.T) {
	result := textResult("hello world")
	if result == nil {
		t.Fatal("textResult returned nil")
	}
	if len(result.Content) != 1 {
		t.Errorf("expected 1 content item, got %d", len(result.Content))
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected *mcp.TextContent")
	}
	if textContent.Text != "hello world" {
		t.Errorf("expected 'hello world', got %s", textContent.Text)
	}
}

func TestNewServer(t *testing.T) {
	server, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	if server == nil {
		t.Fatal("NewServer returned nil")
	}
	if server.server == nil {
		t.Fatal("server.server is nil")
	}
	if server.storage == nil {
		t.Fatal("server.storage is nil")
	}
	if server.scraper == nil {
		t.Fatal("server.scraper is nil")
	}
	if server.scheduler == nil {
		t.Fatal("server.scheduler is nil")
	}
}