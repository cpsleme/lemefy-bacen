package scraper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lemefy/lemefy-bacen/internal/config"
	"github.com/lemefy/lemefy-bacen/internal/meilisearch"
	"github.com/lemefy/lemefy-bacen/internal/models"
	"github.com/lemefy/lemefy-bacen/internal/storage"
	"github.com/sirupsen/logrus"
)

const (
	// BCB Search API (SharePoint/search) used by the bcb.gov.br norms SPA.
	bcbAPIBase = "https://www.bcb.gov.br/api/search/app/normativos/buscanormativos"
	// Web base used to build the human-readable norma URL (matches existing records).
	bcbNormativoBase = "https://www.bcb.gov.br/estabilidadefinanceira/exibenormativo"

	// BCB API page size. The endpoint caps rowlimit and silently trims larger values.
	defaultPageSize = 500
	defaultMaxPages = 10
)

// apiResponse models the JSON document returned by the BCB search API.
type apiResponse struct {
	TotalRows int      `json:"TotalRows"`
	RowCount  int      `json:"RowCount"`
	Rows      []ApiRow `json:"Rows"`
}

// ApiRow models a single search result row. Only the fields consumed by the
// scraper are mapped; the API returns many others (e.g. listItemId) with
// inconsistent typing, which Go's strict decoder rejects.
type ApiRow struct {
	Title                   string `json:"title"`
	TipodoNormativoOWSCHCS  string `json:"TipodoNormativoOWSCHCS"`
	NumeroOWSNMBR           string `json:"NumeroOWSNMBR"`
	RefinableString03       string `json:"RefinableString03"`
	AssuntoNormativoOWSMTXT string `json:"AssuntoNormativoOWSMTXT"`
	HitHighlightedSummary   string `json:"HitHighlightedSummary"`
	RevogadoOWSBOOL         string `json:"RevogadoOWSBOOL"`
	CanceladoOWSBOOL        string `json:"CanceladoOWSBOOL"`
	Data                    string `json:"data"`
}

// Scraper collects Banco Central norms via the BCB Search API and mirrors them
// into the local SQLite store and (when configured) Meilisearch.
type Scraper struct {
	config     *config.Config
	storage    *storage.Database
	meili      *meilisearch.Client
	logger     *logrus.Logger
	httpClient *http.Client
	maxPages   int
	pageSize   int

	wg   sync.WaitGroup
	mu   sync.Mutex
	stats ScraperStats
}

// ScraperStats represents scraper statistics
type ScraperStats struct {
	NormasFound   int
	NormasAdded   int
	NormasUpdated int
	Errors        int
	StartTime     time.Time
	EndTime       time.Time
}

// NewScraper creates a new scraper instance backed by the BCB Search API.
func NewScraper(cfg *config.Config, db *storage.Database) *Scraper {
	logger := config.GetLogger()

	pageSize := cfg.Scraper.PageSize
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	maxPages := cfg.Scraper.MaxPages
	if maxPages <= 0 {
		maxPages = defaultMaxPages
	}

	return &Scraper{
		config:     cfg,
		storage:    db,
		logger:     logger,
		httpClient: &http.Client{Timeout: time.Duration(cfg.Scraper.Timeout) * time.Second},
		maxPages:   maxPages,
		pageSize:   pageSize,
		stats: ScraperStats{
			StartTime: time.Now(),
		},
	}
}

// SetSearchClient attaches a Meilisearch client so scraped normas are mirrored
// to the local search index. Safe to call with nil to disable syncing.
func (s *Scraper) SetSearchClient(c *meilisearch.Client) {
	s.meili = c
}

// baseQuery returns the querytext selecting every normativo content type.
func (s *Scraper) baseQuery() string {
	return "ContentType:normativo AND contentSource:normativos"
}

// Run starts the scraping process, collecting the most recent norms from the
// BCB Search API (bounded by MaxPages * PageSize) and persisting them.
func (s *Scraper) Run() error {
	s.logger.Info("Starting scraping process...")

	normas, err := s.fetchNormas(s.baseQuery(), "", s.maxPages, 0)
	if err != nil {
		return fmt.Errorf("failed to fetch normas from BCB API: %w", err)
	}

	s.mu.Lock()
	s.stats.NormasFound = len(normas)
	s.mu.Unlock()

	s.processNormas(normas)

	s.stats.EndTime = time.Now()
	duration := s.stats.EndTime.Sub(s.stats.StartTime).Milliseconds()

	s.logger.WithFields(logrus.Fields{
		"found":    s.stats.NormasFound,
		"added":    s.stats.NormasAdded,
		"updated":  s.stats.NormasUpdated,
		"errors":   s.stats.Errors,
		"duration_ms": duration,
	}).Info("Scraping completed")

	status := "completed"
	if s.stats.Errors > 0 {
		status = "completed_with_errors"
	}

	err = s.storage.SaveScrapeHistory(
		s.stats.NormasFound,
		s.stats.NormasAdded,
		s.stats.NormasUpdated,
		int(duration),
		status,
		"",
	)

	return err
}

// ScrapeByTipo scrapes norms filtered by a single norm type.
func (s *Scraper) ScrapeByTipo(tipo models.TipoNorma) error {
	return s.ScrapeByTipoWithLimit(tipo, 0)
}

// ScrapeByTipoWithLimit scrapes norms filtered by a single norm type with a limit.
func (s *Scraper) ScrapeByTipoWithLimit(tipo models.TipoNorma, limit int) error {
	query := fmt.Sprintf(`%s AND TipodoNormativoOWSCHCS="%s"`, s.baseQuery(), EscapeTipo(ApiTipoFor(tipo)))

	s.resetStats()

	normas, err := s.fetchNormas(query, "", s.maxPages, limit)
	if err != nil {
		return fmt.Errorf("failed to scrape by tipo: %w", err)
	}

	s.mu.Lock()
	s.stats.NormasFound = len(normas)
	s.mu.Unlock()

	s.processNormas(normas)

	s.stats.EndTime = time.Now()
	return s.saveHistory()
}

// ScrapeRecent scrapes recent norms published within the last `days` days.
func (s *Scraper) ScrapeRecent(days int) error {
	if days <= 0 {
		days = 30
	}
	start := time.Now().UTC().AddDate(0, 0, -days)
	refinement := fmt.Sprintf(`Data:range(datetime(%s),datetime(%s))`,
		start.Format("2006-01-02T15:04:05"), time.Now().UTC().Format("2006-01-02T15:04:05"))

	s.resetStats()

	normas, err := s.fetchNormas(s.baseQuery(), refinement, s.maxPages, 0)
	if err != nil {
		return fmt.Errorf("failed to scrape recent: %w", err)
	}

	s.mu.Lock()
	s.stats.NormasFound = len(normas)
	s.mu.Unlock()

	s.processNormas(normas)

	s.stats.EndTime = time.Now()
	return s.saveHistory()
}

// UpdateAllNormas performs a full update of all normas.
func (s *Scraper) UpdateAllNormas() error {
	return s.Run()
}

// fetchNormas pages through the BCB Search API, translating each row into a
// models.Norma. It stops early when the result set is exhausted, MaxPages is
// reached, or limit is reached (0 means no limit).
func (s *Scraper) fetchNormas(querytext, refinement string, maxPages int, limit int) ([]models.Norma, error) {
	if refinement != "" {
		querytext = fmt.Sprintf("%s AND %s", querytext, refinement)
	}

	var all []models.Norma
	start := 0
	for page := 0; page < maxPages; page++ {
		resp, err := s.fetchPage(querytext, start)
		if err != nil {
			return all, fmt.Errorf("page %d: %w", page, err)
		}
		if len(resp.Rows) == 0 {
			break
		}
		for _, row := range resp.Rows {
			if n, ok := s.MapRowToNorma(row); ok {
				all = append(all, n)
				if limit > 0 && len(all) >= limit {
					return all, nil
				}
			}
		}
		s.logger.WithField("url", bcbAPIBase).Debugf("Fetched page %d: %d rows", page, resp.RowCount)

		start += resp.RowCount
		if resp.RowCount < s.pageSize || start >= resp.TotalRows {
			break
		}
	}
	return all, nil
}

// fetchPage requests a single page from the BCB Search API.
func (s *Scraper) fetchPage(querytext string, startRow int) (apiResponse, error) {
	u, err := url.Parse(bcbAPIBase)
	if err != nil {
		return apiResponse{}, err
	}
	q := u.Query()
	q.Set("querytext", querytext)
	q.Set("rowlimit", strconv.Itoa(s.pageSize))
	q.Set("startrow", strconv.Itoa(startRow))
	q.Set("sortlist", "Data1OWSDATE:descending")
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return apiResponse{}, err
	}
	req.Header.Set("User-Agent", s.config.Scraper.UserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return apiResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return apiResponse{}, fmt.Errorf("BCB API returned status %d", resp.StatusCode)
	}

	var out apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return apiResponse{}, fmt.Errorf("failed to decode BCB API response: %w", err)
	}
	return out, nil
}

// mapRowToNorma converts a raw BCB API row into a models.Norma, matching the
// URL scheme and field formatting used by existing stored records so that
// re-scraping upserts in place rather than creating duplicates.
func (s *Scraper) MapRowToNorma(r ApiRow) (models.Norma, bool) {
	tipoParam := strings.TrimSpace(strings.TrimPrefix(r.RefinableString03, "string;#"))
	if tipoParam == "" {
		tipoParam = strings.TrimSpace(r.TipodoNormativoOWSCHCS)
	}
	if tipoParam == "" {
		return models.Norma{}, false
	}

	numero := ParseNumero(r.NumeroOWSNMBR)
	if numero == "" {
		return models.Norma{}, false
	}

	titulo := fmt.Sprintf("%s N° %s", tipoParam, formatBrazilianNumero(numero))

	tipo := normalizeTipo(r.TipodoNormativoOWSCHCS)

	docURL := fmt.Sprintf("%s?tipo=%s&numero=%s",
		bcbNormativoBase, url.QueryEscape(tipoParam), numero)

	dataPub := strings.TrimSpace(r.Data)
	if dataPub == "" {
		dataPub = time.Now().UTC().Format(time.RFC3339)
	}
	dataPub = FormatDateTime(dataPub)

	situacao := "Vigente"
	if strings.Contains(r.RevogadoOWSBOOL, "1") {
		situacao = "Revogado"
	} else if strings.Contains(r.CanceladoOWSBOOL, "1") {
		situacao = "Cancelado"
	}

	assunto := s.cleanText(r.AssuntoNormativoOWSMTXT)
	sumario := s.cleanText(r.HitHighlightedSummary)
	if len(assunto) > len(sumario) {
		sumario = assunto
	}

	return models.Norma{
		Numero:         numero,
		Tipo:           tipo,
		Titulo:         titulo,
		DataPublicacao: dataPub,
		DataVigencia:   dataPub,
		URL:            docURL,
		TextoURL:       docURL,
		Situacao:       situacao,
		Assunto:        assunto,
		Sumario:        sumario,
		ArquivoPDF:     "",
		Texto:          "",
	}, true
}

// contentTipoFor returns the BCB content API tipo value for a given stored tipo.
func contentTipoFor(tipo models.TipoNorma) string {
	switch tipo {
	case models.TipoInstrucao:
		return "Instrução Normativa BCB"
	case models.TipoComunicado:
		return "Comunicado"
	case models.TipoCartaCircular:
		return "Carta Circular"
	case models.TipoCircular:
		return "Circular"
	case models.TipoResolucao:
		return "Resolução"
	default:
		return string(tipo)
	}
}

// contentEndpointFor returns the BCB content API endpoint for a given tipo.
func contentEndpointFor(tipo models.TipoNorma) string {
	switch tipo {
	case models.TipoComunicado:
		return "https://www.bcb.gov.br/api/conteudo/app/normativos/exibeoutrasnormas"
	default:
		return "https://www.bcb.gov.br/api/conteudo/app/normativos/exibenormativo"
	}
}

// FetchText fetches the full text of a norma from the BCB content API.
func (s *Scraper) FetchText(norma *models.Norma) string {
	if norma == nil {
		return ""
	}

	endpoint := contentEndpointFor(norma.Tipo)
	u, err := url.Parse(endpoint)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to parse content API endpoint")
		return ""
	}

	q := u.Query()
	q.Set("p1", contentTipoFor(norma.Tipo))
	q.Set("p2", norma.Numero)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to create content API request")
		return ""
	}
	req.Header.Set("User-Agent", s.config.Scraper.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to fetch norma text from BCB")
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		s.logger.Warnf("BCB content API returned status %d for %s %s", resp.StatusCode, norma.Tipo, norma.Numero)
		return ""
	}

	var contentResp struct {
		Conteudo []map[string]interface{} `json:"conteudo"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&contentResp); err != nil {
		s.logger.WithError(err).Warn("Failed to decode BCB content API response")
		return ""
	}

	if len(contentResp.Conteudo) == 0 {
		return ""
	}

	item := contentResp.Conteudo[0]
	
	if textoVal, ok := item["Texto"].(string); ok {
		texto := textoVal
		texto = strings.ReplaceAll(texto, "\r\n", "\n")
		texto = strings.ReplaceAll(texto, "\r", "\n")
		texto = s.cleanText(texto)
		norma.Texto = texto
	}
	
	if docVal, ok := item["Documentos"].(string); ok && docVal != "" {
		parts := strings.Split(docVal, ";")
		if len(parts) >= 1 {
			norma.ArquivoPDF = strings.TrimSpace(parts[0])
		}
		norma.Documentos = s.cleanText(docVal)
	}
	
	if douVal, ok := item["DOU"].(string); ok {
		norma.DOU = s.cleanText(douVal)
	}
	
	if assuntoVal, ok := item["Assunto"].(string); ok {
		norma.Assunto = s.cleanText(assuntoVal)
	}
	
	if normasVal, ok := item["NormasVinculadas"].(string); ok {
		norma.NormasVinculadas = s.cleanText(normasVal)
	}
	
	if refVal, ok := item["Referencias"].(string); ok {
		norma.Referencias = s.cleanText(refVal)
	}
	
	if atuVal, ok := item["Atualizacoes"].(string); ok {
		norma.Atualizacoes = s.cleanText(atuVal)
	}
	
	if dataAssVal, ok := item["DataAssinatura"].(string); ok {
		norma.DataAssinatura = s.cleanText(dataAssVal)
	}
	
	if votoVal, ok := item["Voto"].(string); ok {
		norma.Voto = s.cleanText(votoVal)
	}
	
	if versaoVal, ok := item["VersaoNormativo"].(string); ok {
		norma.VersaoNormativo = s.cleanText(versaoVal)
	}
	
	return norma.Texto
}

// processNormas persists a batch of scraped normas concurrently, mirroring each
// into Meilisearch (best-effort) as it is stored.
func (s *Scraper) processNormas(normas []models.Norma) {
	const workers = 8
	jobs := make(chan models.Norma, len(normas))
	for w := 0; w < workers; w++ {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			for n := range jobs {
				s.processNorma(n)
			}
		}()
	}
	for _, n := range normas {
		jobs <- n
	}
	close(jobs)
	s.wg.Wait()
}

// processNorma stores a single norma and updates it in Meilisearch.
func (s *Scraper) processNorma(n models.Norma) {
	exists, err := s.storage.CheckNormaExists(n.URL)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to check if norma exists")
		s.mu.Lock()
		s.stats.Errors++
		s.mu.Unlock()
		return
	}

	if err := s.storage.SaveNorma(&n); err != nil {
		s.logger.WithError(err).Warn("Failed to save norma")
		s.mu.Lock()
		s.stats.Errors++
		s.mu.Unlock()
		return
	}

	if n.Texto == "" || n.Documentos != "" || n.DOU != "" || n.NormasVinculadas != "" || n.Referencias != "" || n.Atualizacoes != "" {
		n.Texto = s.FetchText(&n)
		if n.Texto != "" || n.Documentos != "" || n.DOU != "" || n.NormasVinculadas != "" || n.Referencias != "" || n.Atualizacoes != "" {
			if err := s.storage.SaveNorma(&n); err != nil {
				s.logger.WithError(err).Warn("Failed to save norma text/fields")
			}
		}
	}

	s.meili.IndexNorma(&n)

	s.mu.Lock()
	if exists {
		s.stats.NormasUpdated++
	} else {
		s.stats.NormasAdded++
	}
	s.mu.Unlock()
}

// resetStats restores the scraper statistics for a fresh run.
func (s *Scraper) resetStats() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats = ScraperStats{StartTime: time.Now()}
}

// saveHistory persists the latest scrape statistics.
func (s *Scraper) saveHistory() error {
	s.stats.EndTime = time.Now()
	duration := s.stats.EndTime.Sub(s.stats.StartTime).Milliseconds()

	status := "completed"
	if s.stats.Errors > 0 {
		status = "completed_with_errors"
	}
	return s.storage.SaveScrapeHistory(
		s.stats.NormasFound,
		s.stats.NormasAdded,
		s.stats.NormasUpdated,
		int(duration),
		status,
		"",
	)
}

// GetStats returns the scraper statistics
func (s *Scraper) GetStats() ScraperStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

// normalizeTipo maps the BCB short tipo text to a TipoNorma constant. The BCB
// API returns tipo labels inconsistently: Resoluções carry an "esfera" suffix
// (e.g. "Resolução CMN", "Resolução BCB", "Resolução Conjunta", "Resolução Coseg")
// and Instruções are returned as "Instrução Normativa BCB"; Carta-Circular is
// spelled with a space ("Carta Circular"). Prefix matching classifies all of
// these variants regardless of the suffix.
func normalizeTipo(t string) models.TipoNorma {
	t = strings.TrimSpace(t)
	switch {
	case strings.HasPrefix(t, "Resolução"):
		return models.TipoResolucao
	case t == "Circular":
		return models.TipoCircular
	case t == "Comunicado":
		return models.TipoComunicado
	case strings.HasPrefix(t, "Instrução"):
		return models.TipoInstrucao
	case strings.HasPrefix(t, "Carta"):
		return models.TipoCartaCircular
	default:
		return models.TipoOutros
	}
}

// ApiTipoFor returns the tipo token used to filter the BCB Search API by
// TipodoNormativoOWSCHCS for a given TipoNorma. Resoluções and Instruções have
// "esfera"/suffix variants, so a wildcard prefix is used to match every
// variant (e.g. both "Resolução CMN" and "Resolução BCB"). Carta-Circular is
// indexed by the API as "Carta Circular" (space, not hyphen).
func ApiTipoFor(tipo models.TipoNorma) string {
	switch tipo {
	case models.TipoResolucao:
		return "Resolução*"
	case models.TipoInstrucao:
		return "Instrução*"
	case models.TipoCartaCircular:
		return "Carta Circular"
	case models.TipoComunicado:
		return "Comunicado"
	case models.TipoCircular:
		return "Circular"
	default:
		return string(tipo)
	}
}


// ParseNumero extracts the integer string from a NumeroOWSNMBR value such as
// "45682.0000000000".
func ParseNumero(v string) string {
	v = strings.TrimSpace(v)
	parts := strings.SplitN(v, ".", 2)
	return strings.TrimSpace(parts[0])
}

// formatBrazilianNumero formats an integer string with a "." thousands
// separator, e.g. "45682" -> "45.682".
func formatBrazilianNumero(n string) string {
	n = strings.TrimSpace(n)
	if n == "" {
		return n
	}
	if len(n) <= 3 {
		return n
	}
	digits := len(n)
	var b strings.Builder
	for i, c := range n {
		if i > 0 && (digits-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// EscapeTipo escapes a tipo value for safe use inside a SharePoint querytext
// literal (double-quoted). Embedded double quotes are doubled.
func EscapeTipo(t string) string {
	return strings.ReplaceAll(strings.TrimSpace(t), `"`, `\"`)
}

// FormatDateTime normalizes a datetime string to a readable local-time format
// without timezone info, e.g. "2026-08-04T21:50:51Z" -> "2026-08-04 21:50:51".
func FormatDateTime(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}
	v = strings.Replace(v, "T", " ", 1)
	v = strings.TrimSuffix(v, "Z")
	v = strings.TrimSpace(v)
	if len(v) > 19 {
		v = v[:19]
	}
	return v
}

var (
	htmlTagRe      = regexp.MustCompile(`(?is)<[^>]*>`)
	controlCharsRe = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
)

var htmlEntities = map[string]string{
	"&nbsp;": " ",
	"&lt;": "<",
	"&gt;": ">",
	"&amp;": "&",
	"&quot;": "\"",
	"&#58;": ":",
	"&#160;": " ",
	"&#123;": "{",
	"&#125;": "}",
	"&#201;": "É",
	"&#231;": "ç",
	"&ldquo;": "\"",
	"&rdquo;": "\"",
	"&lsquo;": "'",
	"&rsquo;": "'",
	"&mdash;": "—",
	"&ndash;": "–",
	"&copy;": "©",
	"&reg;": "®",
}

// decodeHTMLEntities replaces known HTML entities with their Unicode equivalents.
func decodeHTMLEntities(s string) string {
	for ent, ch := range htmlEntities {
		s = strings.ReplaceAll(s, ent, ch)
	}
	return s
}

// sanitizeText removes HTML tags, decodes HTML entities, removes control
// characters, and normalizes whitespace for BCB normative text fields.
func sanitizeText(s string) string {
	s = strings.ReplaceAll(s, "<ddd/>", " ")
	s = htmlTagRe.ReplaceAllString(s, " ")
	s = decodeHTMLEntities(s)
	s = controlCharsRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.ReplaceAll(s, "\u200b", "")
	s = strings.ReplaceAll(s, "\u2009", " ")
	return s
}

// cleanText cleans and normalizes text
func (s *Scraper) cleanText(text string) string {
	text = sanitizeText(text)
	return text
}
