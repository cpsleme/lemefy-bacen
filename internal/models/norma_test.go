package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/lemefy/lemefy-bacen/pkg/utils"
)

func TestTipoNormaValues(t *testing.T) {
	tests := []struct {
		name string
		tipo TipoNorma
	}{
		{"Resolucao", TipoResolucao},
		{"Circular", TipoCircular},
		{"Instrucao", TipoInstrucao},
		{"Comunicado", TipoComunicado},
		{"CartaCircular", TipoCartaCircular},
		{"Outros", TipoOutros},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.tipo == "" {
				t.Errorf("TipoNorma %s should not be empty", tt.name)
			}
		})
	}
}

func TestNormaDefaults(t *testing.T) {
	norma := &Norma{
		Numero: "123",
		Tipo:   TipoResolucao,
		Titulo: "Test Norm",
	}

	if norma.Numero != "123" {
		t.Errorf("expected Numero '123', got %s", norma.Numero)
	}
	if norma.Tipo != TipoResolucao {
		t.Errorf("expected Tipo Resolucao, got %s", norma.Tipo)
	}
	if norma.Situacao != "" {
		t.Errorf("expected empty Situacao, got %s", norma.Situacao)
	}
}

func TestNormaSearchDefaults(t *testing.T) {
	search := &NormaSearch{
		Page:     1,
		PageSize: 50,
	}

	if search.Page != 1 {
		t.Errorf("expected Page 1, got %d", search.Page)
	}
	if search.PageSize != 50 {
		t.Errorf("expected PageSize 50, got %d", search.PageSize)
	}
	if search.Tipo != nil {
		t.Error("expected Tipo to be nil")
	}
	if search.Numero != nil {
		t.Error("expected Numero to be nil")
	}
}

func TestNormaResponsePagination(t *testing.T) {
	resp := &NormaResponse{
		Total:     100,
		Page:      2,
		PageSize:  10,
		TotalPages: 10,
	}

	if resp.Total != 100 {
		t.Errorf("expected Total 100, got %d", resp.Total)
	}
	if resp.Page != 2 {
		t.Errorf("expected Page 2, got %d", resp.Page)
	}
	if resp.TotalPages != 10 {
		t.Errorf("expected TotalPages 10, got %d", resp.TotalPages)
	}
}

func TestStatsDefaults(t *testing.T) {
	stats := &Stats{}

	if stats.TotalNormas != 0 {
		t.Errorf("expected TotalNormas 0, got %d", stats.TotalNormas)
	}
	if stats.NormasVigentes != 0 {
		t.Errorf("expected NormasVigentes 0, got %d", stats.NormasVigentes)
	}
	if stats.NormasRevogadas != 0 {
		t.Errorf("expected NormasRevogadas 0, got %d", stats.NormasRevogadas)
	}
	if stats.Tipos != nil {
		t.Error("expected Tipos to be nil")
	}
}

func TestParseDateFormats(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"ISO format", "2024-01-15", false},
		{"Brazilian format", "15/01/2024", false},
		{"With time", "2024-01-15 10:30:00", false},
		{"Empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := utils.ParseDate(tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for input %q, got nil", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error for input %q: %v", tt.input, err)
			}
		})
	}
}

func TestCleanText(t *testing.T) {
	tests := []struct {
		name string
		input string
		want string
	}{
		{"multiple spaces", "hello    world", "hello world"},
		{"tabs", "hello\tworld", "hello world"},
		{"newlines", "hello\nworld", "hello world"},
		{"leading/trailing", "  hello  ", "hello"},
		{"non-breaking space", "hello\u00a0world", "hello world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := utils.CleanText(tt.input)
			if got != tt.want {
				t.Errorf("CleanText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name string
		input string
		want string
	}{
		{"normal", "file.txt", "file.txt"},
		{"with slash", "dir/file.txt", "dir_file.txt"},
		{"with colon", "file:name.txt", "file-name.txt"},
		{"with asterisk", "file*.txt", "file.txt"},
		{"with question", "file?.txt", "file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := utils.SanitizeFilename(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormaJSON(t *testing.T) {
	norma := Norma{
		ID:     1,
		Numero: "123",
		Tipo:   TipoResolucao,
		Titulo: "Test",
	}

	data, err := json.Marshal(norma)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded Norma
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.ID != norma.ID {
		t.Errorf("expected ID %d, got %d", norma.ID, decoded.ID)
	}
	if decoded.Numero != norma.Numero {
		t.Errorf("expected Numero %s, got %s", norma.Numero, decoded.Numero)
	}
}

func TestNormaSearchWithDateRange(t *testing.T) {
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

	search := &NormaSearch{
		DataDe:  &from,
		DataAte: &to,
	}

	if search.DataDe == nil || search.DataAte == nil {
		t.Fatal("Date range should not be nil")
	}

	if !search.DataDe.Equal(from) {
		t.Errorf("expected DataDe %v, got %v", from, *search.DataDe)
	}
	if !search.DataAte.Equal(to) {
		t.Errorf("expected DataAte %v, got %v", to, *search.DataAte)
	}
}