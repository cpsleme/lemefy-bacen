package parsers

import (
	"strings"

	"github.com/lemefy/lemefy-bacen/internal/models"
)

// ParseDocumentosPDF splits the BCB Documentos string into structured items.
// Input format: "file.pdf;123#;file2.pdf;456#;"
func ParseDocumentosPDF(raw string) []models.DocumentoPDF {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ";")
	var out []models.DocumentoPDF
	for i := 0; i < len(parts); i += 2 {
		nome := strings.TrimSpace(parts[i])
		if nome == "" {
			continue
		}
		id := ""
		if i+1 < len(parts) {
			id = strings.TrimSpace(parts[i+1])
		}
		out = append(out, models.DocumentoPDF{Nome: nome, ID: id})
	}
	return out
}

// ParseNormasVinculadas splits the BCB NormasVinculadas string into structured items.
// Input format: "TIPO;@NUMERO;@ANO;#TIPO2;@NUMERO2;@ANO2;#"
func ParseNormasVinculadas(raw string) []models.NormaVinculada {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "#")
	var out []models.NormaVinculada
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		fields := strings.Split(p, ";")
		tipo := strings.TrimSpace(fields[0])
		numero := ""
		ano := ""
		for _, f := range fields[1:] {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			if strings.HasPrefix(f, "@") {
				if numero == "" {
					numero = strings.TrimPrefix(f, "@")
				} else if ano == "" {
					ano = strings.TrimPrefix(f, "@")
				}
			}
		}
		out = append(out, models.NormaVinculada{Tipo: tipo, Numero: numero, Ano: ano})
	}
	return out
}

// ParseReferencias splits the BCB Referencias string into structured items.
// Input format: "link|texto da ref.;#outra ref.;#"
func ParseReferencias(raw string) []models.Referencia {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "#")
	var out []models.Referencia
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		ref := models.Referencia{Texto: p}
		if idx := strings.Index(p, "|"); idx >= 0 {
			ref.URL = strings.TrimSpace(p[:idx])
			ref.Texto = strings.TrimSpace(p[idx+1:])
		}
		out = append(out, ref)
	}
	return out
}
