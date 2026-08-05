package cmds

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/lemefy/lemefy-bacen/internal/models"
	"github.com/lemefy/lemefy-bacen/internal/scraper"
	"github.com/spf13/cobra"
)

type validateCmdFlags struct {
	id     int
	url    string
	tipo   string
	numero string
	all    bool
	limit  int
}

func NewValidateCmd() *cobra.Command {
	flags := &validateCmdFlags{}

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate document consistency across BCB, SQLite, and Meilisearch",
		Long:  "Fetch a document from BCB API, compare it with SQLite and Meilisearch storage",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := InitApp(configPath)
			if err != nil {
				return err
			}
			defer app.Close()

			if flags.all {
				return runValidateAll(app, flags.limit)
			}

			if flags.url != "" {
				return runValidateByURL(app, flags.url)
			}

			if flags.tipo != "" && flags.numero != "" {
				return runValidateByTipoNumero(app, flags.tipo, flags.numero)
			}

			if flags.id > 0 {
				return runValidateByID(app, flags.id)
			}

			return fmt.Errorf("provide --id, --url, --tipo+--numero, or --all")
		},
	}

	cmd.Flags().IntVarP(&flags.id, "id", "i", 0, "SQLite ID of the document to validate")
	cmd.Flags().StringVarP(&flags.url, "url", "u", "", "URL of the document to validate")
	cmd.Flags().StringVarP(&flags.tipo, "tipo", "t", "", "Tipo of the document to validate")
	cmd.Flags().StringVarP(&flags.numero, "numero", "n", "", "Numero of the document to validate")
	cmd.Flags().BoolVarP(&flags.all, "all", "a", false, "Validate all documents in SQLite")
	cmd.Flags().IntVarP(&flags.limit, "limit", "l", 10, "Limit for --all validation")

	return cmd
}

func runValidateByID(app *App, id int) error {
	if id <= 0 {
		return fmt.Errorf("invalid id: %d", id)
	}

	fmt.Printf("Validating document with SQLite ID: %d\n", id)

	sqliteNorma, err := app.Storage.GetNormaByID(int64(id))
	if err != nil {
		return fmt.Errorf("failed to get norma from SQLite: %w", err)
	}
	if sqliteNorma == nil {
		return fmt.Errorf("norma with ID %d not found in SQLite", id)
	}

	return compareAndPrint(app, sqliteNorma)
}

func runValidateByURL(app *App, docURL string) error {
	fmt.Printf("Validating document with URL: %s\n", docURL)

	sqliteNorma, err := app.Storage.GetNormaByURL(docURL)
	if err != nil {
		return fmt.Errorf("failed to get norma from SQLite: %w", err)
	}
	if sqliteNorma == nil {
		return fmt.Errorf("norma with URL %q not found in SQLite", docURL)
	}

	return compareAndPrint(app, sqliteNorma)
}

func runValidateByTipoNumero(app *App, tipo, numero string) error {
	fmt.Printf("Validating document with tipo=%s numero=%s\n", tipo, numero)

	search := &models.NormaSearch{
		Tipo:     (*models.TipoNorma)(&tipo),
		Numero:   &numero,
		Page:     1,
		PageSize: 1,
	}

	normas, _, err := app.Storage.ListNormas(search)
	if err != nil {
		return fmt.Errorf("failed to list normas: %w", err)
	}
	if len(normas) == 0 {
		return fmt.Errorf("norma with tipo=%q numero=%q not found in SQLite", tipo, numero)
	}

	return compareAndPrint(app, &normas[0])
}

func runValidateAll(app *App, limit int) error {
	fmt.Printf("Validating up to %d documents in SQLite...\n", limit)

	normas, _, err := app.Storage.ListNormas(&models.NormaSearch{
		Page:     1,
		PageSize: limit,
	})
	if err != nil {
		return fmt.Errorf("failed to list normas: %w", err)
	}

	if len(normas) == 0 {
		return fmt.Errorf("no normas found in SQLite")
	}

	mismatchCount := 0
	for i, n := range normas {
		fmt.Printf("\n[%d/%d] ID=%d %s %s\n", i+1, len(normas), n.ID, n.Tipo, n.Numero)
		if err := compareAndPrint(app, &n); err != nil {
			fmt.Printf("  ERROR: %v\n", err)
			mismatchCount++
		}
	}

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Validated: %d\n", len(normas))
	fmt.Printf("Mismatches: %d\n", mismatchCount)
	if mismatchCount == 0 {
		fmt.Println("SUCCESS: All validated documents are identical across BCB, SQLite, and Meilisearch")
	} else {
		fmt.Printf("FAILED: %d document(s) have differences\n", mismatchCount)
	}
	return nil
}

func compareAndPrint(app *App, sqliteNorma *models.Norma) error {
	meiliNorma, err := fetchFromMeilisearch(app, sqliteNorma)
	if err != nil {
		return fmt.Errorf("failed to get norma from Meilisearch: %w", err)
	}
	if meiliNorma == nil {
		return fmt.Errorf("norma with ID %d not found in Meilisearch", sqliteNorma.ID)
	}

	bcbNorma, err := fetchFromBCB(app, sqliteNorma)
	if err != nil {
		return fmt.Errorf("failed to get norma from BCB: %w", err)
	}
	if bcbNorma == nil {
		return fmt.Errorf("norma with ID %d not found in BCB API", sqliteNorma.ID)
	}

	fmt.Println("\n=== Comparison Results ===")

	diffs := compareNormas("BCB vs SQLite", *bcbNorma, *sqliteNorma)
	diffs = append(diffs, compareNormas("BCB vs Meilisearch", *bcbNorma, *meiliNorma)...)
	diffs = append(diffs, compareNormas("SQLite vs Meilisearch", *sqliteNorma, *meiliNorma)...)

	if len(diffs) == 0 {
		fmt.Println("SUCCESS: All documents are identical across BCB, SQLite, and Meilisearch")
		return nil
	}

	fmt.Printf("\nFound %d difference(s):\n", len(diffs))
	for _, d := range diffs {
		fmt.Printf("  - %s\n", d)
	}
	return fmt.Errorf("validation failed with %d difference(s)", len(diffs))
}

func fetchFromMeilisearch(app *App, sqliteNorma *models.Norma) (*models.Norma, error) {
	if app.Meili == nil || !app.Meili.Available() {
		return nil, fmt.Errorf("Meilisearch not available")
	}

	uid := app.Meili.IndexFor(sqliteNorma.Tipo)
	apiKey := app.Config.Meilisearch.APIKey

	baseURL := app.Config.Meilisearch.Host
	if strings.HasSuffix(baseURL, "/") {
		baseURL = baseURL[:len(baseURL)-1]
	}

	req, err := http.NewRequest(http.MethodGet, baseURL+"/indexes/"+uid+"/documents/"+fmt.Sprint(sqliteNorma.ID), nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Meilisearch returned status %d", resp.StatusCode)
	}

	var doc models.Norma
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func fetchFromBCB(app *App, sqliteNorma *models.Norma) (*models.Norma, error) {
	cfg := app.Config
	baseURL := cfg.Scraper.BaseURL
	if baseURL == "" {
		baseURL = "https://www.bcb.gov.br/normativos"
	}

	apiBase := "https://www.bcb.gov.br/api/search/app/normativos/buscanormativos"
	parsedAPI, err := url.Parse(apiBase)
	if err != nil {
		return nil, err
	}

	q := parsedAPI.Query()
	queryText := fmt.Sprintf(`ContentType:normativo AND contentSource:normativos AND TipodoNormativoOWSCHCS="%s"`,
		scraper.EscapeTipo(scraper.ApiTipoFor(sqliteNorma.Tipo)))
	q.Set("querytext", queryText)
	q.Set("rowlimit", "1000")
	q.Set("startrow", "0")
	q.Set("sortlist", "Data1OWSDATE:descending")
	parsedAPI.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, parsedAPI.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", cfg.Scraper.UserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9")

	client := &http.Client{Timeout: time.Duration(cfg.Scraper.Timeout) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("BCB API returned status %d", resp.StatusCode)
	}

	var apiResp struct {
		TotalRows int        `json:"TotalRows"`
		RowCount  int        `json:"RowCount"`
		Rows      []scraper.ApiRow `json:"Rows"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode BCB API response: %w", err)
	}

	for _, row := range apiResp.Rows {
		numero := scraper.ParseNumero(row.NumeroOWSNMBR)
		if numero == sqliteNorma.Numero {
			s := scraper.NewScraper(app.Config, nil)
			norma, ok := s.MapRowToNorma(row)
			if !ok {
				return nil, fmt.Errorf("failed to map BCB row to norma")
			}
			norma.Texto = s.FetchText(&norma)
			return &norma, nil
		}
	}

	return nil, nil
}

func compareNormas(label string, a, b models.Norma) []string {
	var diffs []string

	fields := []struct {
		name string
		av   string
		bv   string
	}{
		{"Numero", a.Numero, b.Numero},
		{"Tipo", string(a.Tipo), string(b.Tipo)},
		{"Titulo", a.Titulo, b.Titulo},
		{"DataPublicacao", scraper.FormatDateTime(a.DataPublicacao), scraper.FormatDateTime(b.DataPublicacao)},
		{"DataVigencia", scraper.FormatDateTime(a.DataVigencia), scraper.FormatDateTime(b.DataVigencia)},
		{"URL", normalizeURL(a.URL), normalizeURL(b.URL)},
		{"TextoURL", normalizeURL(a.TextoURL), normalizeURL(b.TextoURL)},
		{"Situacao", a.Situacao, b.Situacao},
		{"Assunto", a.Assunto, b.Assunto},
		{"Sumario", a.Sumario, b.Sumario},
		{"Texto", a.Texto, b.Texto},
		{"ArquivoPDF", a.ArquivoPDF, b.ArquivoPDF},
		{"Documentos", a.Documentos, b.Documentos},
		{"DOU", a.DOU, b.DOU},
		{"NormasVinculadas", a.NormasVinculadas, b.NormasVinculadas},
		{"Referencias", a.Referencias, b.Referencias},
		{"Atualizacoes", a.Atualizacoes, b.Atualizacoes},
		{"DataAssinatura", a.DataAssinatura, b.DataAssinatura},
		{"Voto", a.Voto, b.Voto},
		{"VersaoNormativo", a.VersaoNormativo, b.VersaoNormativo},
	}

	for _, f := range fields {
		if f.av != f.bv {
			diffs = append(diffs, fmt.Sprintf("[%s] %s: %q != %q", label, f.name, truncate(f.av, 120), truncate(f.bv, 120)))
		}
	}

	sort.Strings(diffs)
	return diffs
}

func normalizeURL(u string) string {
	u = strings.ReplaceAll(u, "+", "%20")
	return u
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
