package scraper

import (
	"testing"

	"github.com/lemefy/lemefy-bacen/internal/models"
)

func TestNormalizeTipo(t *testing.T) {
	cases := []struct {
		input string
		want  models.TipoNorma
	}{
		{"Resolução", models.TipoResolucao},
		{"Circular", models.TipoCircular},
		{"Instrução", models.TipoInstrucao},
		{"Comunicado", models.TipoComunicado},
		// The BCB API spells this with a space, while the domain type uses a hyphen.
		{"Carta Circular", models.TipoCartaCircular},
		{"Carta-Circular", models.TipoCartaCircular},
		{"Ato de Diretor", models.TipoOutros},
		{"Outros", models.TipoOutros},
		{"", models.TipoOutros},
	}
	for _, tc := range cases {
		got := normalizeTipo(tc.input)
		if got != tc.want {
			t.Errorf("normalizeTipo(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestAPITipoFor(t *testing.T) {
	cases := []struct {
		tipo models.TipoNorma
		want string
	}{
		{models.TipoResolucao, "Resolução"},
		{models.TipoCircular, "Circular"},
		{models.TipoInstrucao, "Instrução"},
		{models.TipoComunicado, "Comunicado"},
		{models.TipoCartaCircular, "Carta Circular"},
		{models.TipoOutros, "Outros"},
	}
	for _, tc := range cases {
		got := apiTipoFor(tc.tipo)
		if got != tc.want {
			t.Errorf("apiTipoFor(%q) = %q, want %q", tc.tipo, got, tc.want)
		}
	}
}

func TestFormatBrazilianNumero(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"45682", "45.682"},
		{"4837", "4.837"},
		{"704", "704"},
		{"1386", "1.386"},
		{"", ""},
	}
	for _, tc := range cases {
		got := formatBrazilianNumero(tc.in)
		if got != tc.want {
			t.Errorf("formatBrazilianNumero(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseNumero(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"45682.0000000000", "45682"},
		{"4837.0000000000", "4837"},
		{"45682", "45682"},
		{"", ""},
	}
	for _, tc := range cases {
		got := parseNumero(tc.in)
		if got != tc.want {
			t.Errorf("parseNumero(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
