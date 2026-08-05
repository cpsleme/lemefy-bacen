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
		// Resoluções carry an "esfera" suffix in the API; all must classify as
		// Resolução.
		{"Resolução", models.TipoResolucao},
		{"Resolução CMN", models.TipoResolucao},
		{"Resolução BCB", models.TipoResolucao},
		{"Resolução Conjunta", models.TipoResolucao},
		{"Resolução Coseg", models.TipoResolucao},
		{"Circular", models.TipoCircular},
		// The BCB API returns the short tipo as "Instrução Normativa BCB".
		{"Instrução", models.TipoInstrucao},
		{"Instrução Normativa BCB", models.TipoInstrucao},
		{"Comunicado", models.TipoComunicado},
		// The BCB API spells "Carta-Circular" as "Carta Circular" (space), while the
		// model uses a hyphen; both spellings must be recognized.
		{"Carta Circular", models.TipoCartaCircular},
		{"Carta-Circular", models.TipoCartaCircular},
		{"Ato de Diretor", models.TipoOutros},
		{"Ato do Presidente", models.TipoOutros},
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
		{models.TipoResolucao, "Resolução*"},
		{models.TipoCircular, "Circular"},
		{models.TipoInstrucao, "Instrução*"},
		{models.TipoComunicado, "Comunicado"},
		{models.TipoCartaCircular, "Carta Circular"},
		{models.TipoOutros, "Outros"},
	}
	for _, tc := range cases {
		got := ApiTipoFor(tc.tipo)
		if got != tc.want {
			t.Errorf("ApiTipoFor(%q) = %q, want %q", tc.tipo, got, tc.want)
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
		got := ParseNumero(tc.in)
		if got != tc.want {
			t.Errorf("ParseNumero(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
