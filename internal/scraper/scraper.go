package scraper

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gocolly/colly/v2"
	"github.com/lemefy/lemefy-bacen/internal/config"
	"github.com/lemefy/lemefy-bacen/internal/models"
	"github.com/lemefy/lemefy-bacen/internal/storage"
	"github.com/sirupsen/logrus"
)

// Scraper represents the web scraper for Banco Central norms
type Scraper struct {
	collector  *colly.Collector
	config     *config.Config
	storage    *storage.Database
	logger     *logrus.Logger
	baseURL    string
	normasChan chan models.Norma
	wg         sync.WaitGroup
	mu         sync.Mutex
	stats      ScraperStats
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

// NewScraper creates a new scraper instance
func NewScraper(cfg *config.Config, db *storage.Database) *Scraper {
	logger := config.GetLogger()

	// Create collector
	collector := colly.NewCollector(
		colly.AllowedDomains("www.bcb.gov.br", "bcb.gov.br"),
		colly.MaxDepth(cfg.Scraper.MaxDepth),
		colly.Async(true),
	)

	// Set client with timeout
	client := &http.Client{
		Timeout: time.Duration(cfg.Scraper.Timeout) * time.Second,
	}
	collector.SetClient(client)

	// Set request delay
	if cfg.Scraper.RequestDelay > 0 {
		collector.Limit(&colly.LimitRule{
			DomainGlob:  "*bcb.gov.br*",
			Delay:      time.Duration(cfg.Scraper.RequestDelay) * time.Millisecond,
			Parallelism: cfg.Scraper.Concurrency,
		})
	}

	return &Scraper{
		collector:  collector,
		config:     cfg,
		storage:    db,
		logger:     logger,
		baseURL:    cfg.Scraper.BaseURL,
		normasChan: make(chan models.Norma, 1000),
		stats: ScraperStats{
			StartTime: time.Now(),
		},
	}
}

// Run starts the scraping process
func (s *Scraper) Run() error {
	defer close(s.normasChan)

	s.logger.Info("Starting scraping process...")

	// Setup callbacks
	s.setupCallbacks()

	// Start scraping from the main norms page
	err := s.collector.Visit(s.baseURL)
	if err != nil {
		return fmt.Errorf("failed to start scraping: %w", err)
	}

	// Wait for all requests to complete
	s.collector.Wait()

	// Process collected normas
	s.processNormas()

	s.stats.EndTime = time.Now()
	duration := s.stats.EndTime.Sub(s.stats.StartTime).Milliseconds()

	s.logger.WithFields(logrus.Fields{
		"found":   s.stats.NormasFound,
		"added":   s.stats.NormasAdded,
		"updated": s.stats.NormasUpdated,
		"errors":  s.stats.Errors,
		"duration_ms": duration,
	}).Info("Scraping completed")

	// Save scrape history
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

// setupCallbacks sets up the collector callbacks
func (s *Scraper) setupCallbacks() {
	// OnHTML callbacks for different norm types
	s.collector.OnHTML("div.normativo-item, li.normativo-item, article.normativo-item, div.item-norma", func(e *colly.HTMLElement) {
		norma, err := s.parseNormaItem(e)
		if err != nil {
			s.logger.WithError(err).Warn("Failed to parse norma item")
			s.mu.Lock()
			s.stats.Errors++
			s.mu.Unlock()
			return
		}

		if norma != nil {
			s.normasChan <- *norma
			s.mu.Lock()
			s.stats.NormasFound++
			s.mu.Unlock()
		}
	})

	// Try alternative selectors for the BCB site
	s.collector.OnHTML("table.tabela-normas tbody tr", func(e *colly.HTMLElement) {
		norma, err := s.parseNormaFromTableRow(e)
		if err != nil {
			s.logger.WithError(err).Warn("Failed to parse norma from table row")
			s.mu.Lock()
			s.stats.Errors++
			s.mu.Unlock()
			return
		}

		if norma != nil {
			s.normasChan <- *norma
			s.mu.Lock()
			s.stats.NormasFound++
			s.mu.Unlock()
		}
	})

	// OnRequest callback
	s.collector.OnRequest(func(r *colly.Request) {
		// Set headers for each request
		r.Headers.Set("User-Agent", s.config.Scraper.UserAgent)
		r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
		r.Headers.Set("Accept-Language", "pt-BR,pt;q=0.5")
		
		s.logger.WithField("url", r.URL.String()).Debug("Visiting URL")
	})

	// OnError callback
	s.collector.OnError(func(r *colly.Response, err error) {
		s.logger.WithFields(logrus.Fields{
			"url":   r.Request.URL.String(),
			"error": err.Error(),
		}).Error("Request failed")
		s.mu.Lock()
		s.stats.Errors++
		s.mu.Unlock()
	})

	// OnResponse callback
	s.collector.OnResponse(func(r *colly.Response) {
		s.logger.WithFields(logrus.Fields{
			"url":      r.Request.URL.String(),
			"status":   r.StatusCode,
			"size":     len(r.Body),
		}).Debug("Received response")
	})
}

// processNormas processes the collected normas
func (s *Scraper) processNormas() {
	for norma := range s.normasChan {
		s.wg.Add(1)
		go func(n models.Norma) {
			defer s.wg.Done()

			// Check if norma already exists
			exists, err := s.storage.CheckNormaExists(n.URL)
			if err != nil {
				s.logger.WithError(err).Warn("Failed to check if norma exists")
				return
			}

			if exists {
				// Update existing norma
				err = s.storage.SaveNorma(&n)
				if err != nil {
					s.logger.WithError(err).Warn("Failed to update norma")
					return
				}
				s.mu.Lock()
				s.stats.NormasUpdated++
				s.mu.Unlock()
			} else {
				// Insert new norma
				err = s.storage.SaveNorma(&n)
				if err != nil {
					s.logger.WithError(err).Warn("Failed to save norma")
					return
				}
				s.mu.Lock()
				s.stats.NormasAdded++
				s.mu.Unlock()
			}
		}(norma)
	}

	// Wait for all goroutines to complete
	s.wg.Wait()
}

// parseNormaItem parses a norma from a div/li/article element
func (s *Scraper) parseNormaItem(e *colly.HTMLElement) (*models.Norma, error) {
	norma := &models.Norma{}

	// Extract URL
	link := e.ChildAttr("a", "href")
	if link == "" {
		link = e.Attr("href")
	}

	if link == "" {
		return nil, fmt.Errorf("no URL found")
	}

	// Make URL absolute
	url, err := s.makeAbsoluteURL(link)
	if err != nil {
		return nil, fmt.Errorf("failed to make absolute URL: %w", err)
	}
	norma.URL = url.String()

	// Extract tipo and numero
	tipoText := strings.TrimSpace(e.ChildText(".tipo-norma, .norma-tipo, .type"))
	numeroText := strings.TrimSpace(e.ChildText(".numero-norma, .norma-numero, .number"))

	if tipoText == "" {
		// Try to extract from the link text or other elements
		tipoText = strings.TrimSpace(e.ChildText("span.tipo, .label-tipo"))
	}

	if numeroText == "" {
		numeroText = strings.TrimSpace(e.ChildText("span.numero, .label-numero"))
	}

	// If still empty, try to parse from the title
	if tipoText == "" || numeroText == "" {
		title := strings.TrimSpace(e.ChildText("h2, h3, h4, .title, .titulo"))
		if title != "" {
			// Try to extract tipo and numero from title
			parts := strings.Split(title, " ")
			if len(parts) >= 2 {
				if tipoText == "" {
					tipoText = parts[0]
				}
				if numeroText == "" {
					numeroText = parts[1]
				}
			}
		}
	}

	norma.Tipo = models.TipoNorma(tipoText)
	norma.Numero = numeroText

	// Extract titulo
	titulo := strings.TrimSpace(e.ChildText("h2, h3, h4, .title, .titulo, a"))
	if titulo == "" {
		titulo = strings.TrimSpace(e.Text)
	}
	norma.Titulo = s.cleanText(titulo)

	// Extract data de publicacao
	dataPubText := strings.TrimSpace(e.ChildText(".data-publicacao, .publicacao, .date, time"))
	if dataPubText == "" {
		// Try other selectors
		dataPubText = strings.TrimSpace(e.ChildText("span.data, .norma-data"))
	}

	dataPub, err := s.parseDate(dataPubText)
	if err != nil {
		// Use current time as fallback
		dataPub = time.Now().UTC()
	}
	norma.DataPublicacao = dataPub.UTC().Format(time.RFC3339)

	// Extract data de vigencia (usually same as publicacao if not specified)
	dataVigText := strings.TrimSpace(e.ChildText(".data-vigencia, .vigencia"))
	dataVig, err := s.parseDate(dataVigText)
	if err != nil {
		// Use publicacao date as fallback
		dataVig = dataPub
	}
	norma.DataVigencia = dataVig.UTC().Format(time.RFC3339)

	// Extract situacao
	situacao := strings.TrimSpace(e.ChildText(".situacao, .status, .norma-situacao"))
	if situacao == "" {
		situacao = "Vigente" // Default to Vigente
	}
	norma.Situacao = situacao

	// Extract assunto
	assunto := strings.TrimSpace(e.ChildText(".assunto, .subject, .norma-assunto"))
	if assunto != "" {
		norma.Assunto = s.cleanText(assunto)
	}

	// Extract sumario
	sumario := strings.TrimSpace(e.ChildText(".sumario, .summary, .norma-sumario, p"))
	if sumario != "" {
		norma.Sumario = s.cleanText(sumario)
	}

	// Extract PDF link
	pdfLink := e.ChildAttr("a[href*=.pdf], a.pdf", "href")
	if pdfLink != "" {
		pdfURL, err := s.makeAbsoluteURL(pdfLink)
		if err == nil {
			norma.ArquivoPDF = pdfURL.String()
		}
	}

	return norma, nil
}

// parseNormaFromTableRow parses a norma from a table row
func (s *Scraper) parseNormaFromTableRow(e *colly.HTMLElement) (*models.Norma, error) {
	norma := &models.Norma{}

	// Extract URL from link in the row
	link := e.ChildAttr("a", "href")
	if link == "" {
		return nil, fmt.Errorf("no URL found in table row")
	}

	url, err := s.makeAbsoluteURL(link)
	if err != nil {
		return nil, fmt.Errorf("failed to make absolute URL: %w", err)
	}
	norma.URL = url.String()

	// Extract tipo and numero from the first column
	col1 := strings.TrimSpace(e.ChildText("td:nth-child(1)"))
	parts := strings.Split(col1, " ")
	if len(parts) >= 2 {
		norma.Tipo = models.TipoNorma(strings.Join(parts[:len(parts)-1], " "))
		norma.Numero = parts[len(parts)-1]
	} else {
		norma.Tipo = models.TipoNorma(col1)
	}

	// Extract titulo from the second column
	titulo := strings.TrimSpace(e.ChildText("td:nth-child(2)"))
	if titulo == "" {
		titulo = strings.TrimSpace(e.ChildText("td:nth-child(2) a"))
	}
	norma.Titulo = s.cleanText(titulo)

	// Extract data de publicacao from the third column
	dataPubText := strings.TrimSpace(e.ChildText("td:nth-child(3)"))
	dataPub, err := s.parseDate(dataPubText)
	if err != nil {
		dataPub = time.Now().UTC()
	}
	norma.DataPublicacao = dataPub.UTC().Format(time.RFC3339)

	// Extract situacao from the fourth column
	situacao := strings.TrimSpace(e.ChildText("td:nth-child(4)"))
	if situacao == "" {
		situacao = "Vigente"
	}
	norma.Situacao = situacao

	// Data de vigencia (same as publicacao if not specified)
	norma.DataVigencia = dataPub.UTC().Format(time.RFC3339)

	// Try to extract PDF from the row
	pdfLink := e.ChildAttr("a[href*=.pdf]", "href")
	if pdfLink != "" {
		pdfURL, err := s.makeAbsoluteURL(pdfLink)
		if err == nil {
			norma.ArquivoPDF = pdfURL.String()
		}
	}

	return norma, nil
}

// makeAbsoluteURL converts a relative URL to absolute
func (s *Scraper) makeAbsoluteURL(link string) (*url.URL, error) {
	base, err := url.Parse(s.baseURL)
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	return base.ResolveReference(u), nil
}

// parseDate parses a date string in various formats
func (s *Scraper) parseDate(dateStr string) (time.Time, error) {
	if dateStr == "" {
		return time.Now().UTC(), fmt.Errorf("empty date string")
	}

	// Clean the date string
	dateStr = strings.TrimSpace(dateStr)

	// Common date formats on BCB site
	formats := []string{
		"02/01/2006",
		"02-01-2006",
		"2006-01-02",
		"02/01/2006 15:04:05",
		"2006-01-02 15:04:05",
		"02 Jan 2006",
		"Jan 02, 2006",
		"02 de janeiro de 2006",
	}

	for _, format := range formats {
		t, err := time.Parse(format, dateStr)
		if err == nil {
			return t.UTC(), nil
		}
	}

	// Try to parse with regex
	re := regexp.MustCompile(`(\d{2})[/-](\d{2})[/-](\d{4})`)
	matches := re.FindStringSubmatch(dateStr)
	if len(matches) == 4 {
		return time.Parse("2006-01-02", fmt.Sprintf("%s-%s-%s", matches[3], matches[2], matches[1]))
	}

	return time.Now().UTC(), fmt.Errorf("failed to parse date: %s", dateStr)
}

// cleanText cleans and normalizes text
func (s *Scraper) cleanText(text string) string {
	// Remove multiple spaces
	text = strings.Join(strings.Fields(text), " ")
	// Trim whitespace
	text = strings.TrimSpace(text)
	// Replace special quotes
	text = strings.ReplaceAll(text, "\u00a0", " ")
	text = strings.ReplaceAll(text, "\u200b", "")
	text = strings.ReplaceAll(text, "\u2009", " ")

	return text
}

// GetStats returns the scraper statistics
func (s *Scraper) GetStats() ScraperStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

// ScrapeNormaDetails scrapes detailed information from a norma page
func (s *Scraper) ScrapeNormaDetails(url string) (*models.Norma, error) {
	// Create a new collector for detailed scraping
	detailCollector := colly.NewCollector()
	detailCollector.SetClient(&http.Client{
		Timeout: 30 * time.Second,
	})
	
	// Set headers for detail collector
	detailCollector.OnRequest(func(r *colly.Request) {
		r.Headers.Set("User-Agent", s.config.Scraper.UserAgent)
		r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	})

	var norma models.Norma
	var errError error

	detailCollector.OnHTML("html", func(e *colly.HTMLElement) {
		// Try to extract tipo and numero
		tipo := strings.TrimSpace(e.ChildText("h1, h2, .title, .titulo"))
		
		// Extract from meta or specific selectors
		numero := e.ChildText(".norma-numero, .numero, .norma-number")
		
		titulo := strings.TrimSpace(e.ChildText("h1, h2, .title, .titulo"))
		
		// Extract dates
		dataPubText := strings.TrimSpace(e.ChildText(".data-publicacao, .publicacao, time, .date"))
		dataVigText := strings.TrimSpace(e.ChildText(".data-vigencia, .vigencia"))
		
		// Extract subject and summary
		assunto := strings.TrimSpace(e.ChildText(".assunto, .subject, .norma-assunto"))
		sumario := strings.TrimSpace(e.ChildText(".sumario, .summary, . norm a-sumario"))
		
		// Extract PDF
		pdfLink := e.ChildAttr("a[href*=.pdf]", "href")
		
		situacao := strings.TrimSpace(e.ChildText(".situacao, .status, .norma-situacao"))
		if situacao == "" {
			situacao = "Vigente"
		}

		// Parse dates
		dataPub, err := s.parseDate(dataPubText)
		if err != nil {
			dataPub = time.Now().UTC()
		}

		dataVig, err := s.parseDate(dataVigText)
		if err != nil {
			dataVig = dataPub
		}

		norma = models.Norma{
			URL:          url,
			Tipo:         models.TipoNorma(tipo),
			Numero:       numero,
			Titulo:       s.cleanText(titulo),
			DataPublicacao: dataPub.UTC().Format(time.RFC3339),
			DataVigencia:  dataVig.UTC().Format(time.RFC3339),
			Situacao:     situacao,
			Assunto:      s.cleanText(assunto),
			Sumario:      s.cleanText(sumario),
			ArquivoPDF:   pdfLink,
		}
	})

	detailCollector.OnError(func(r *colly.Response, err error) {
		errError = fmt.Errorf("failed to scrape norma details: %w", err)
	})

	err := detailCollector.Visit(url)
	if err != nil {
		return nil, fmt.Errorf("failed to visit norma URL: %w", err)
	}

	detailCollector.Wait()

	if errError != nil {
		return nil, errError
	}

	return &norma, nil
}

// UpdateAllNormas performs a full update of all normas
func (s *Scraper) UpdateAllNormas() error {
	// This method would implement a full re-scrape of all norms
	// For now, we'll just run the regular scrape
	return s.Run()
}

// ScrapeByTipo scrapes norms by type
func (s *Scraper) ScrapeByTipo(tipo models.TipoNorma) error {
	// Build URL for specific type
	typeURL := fmt.Sprintf("%s/buscanormas?tipo=%s", s.baseURL, url.QueryEscape(string(tipo)))

	// Reset stats
	s.stats = ScraperStats{
		StartTime: time.Now(),
	}

	// Clear the channel
	close(s.normasChan)
	s.normasChan = make(chan models.Norma, 1000)

	// Visit the type-specific page
	err := s.collector.Visit(typeURL)
	if err != nil {
		return fmt.Errorf("failed to scrape by tipo: %w", err)
	}

	s.collector.Wait()
	s.processNormas()

	return nil
}

// ScrapeRecent scrapes recent norms (last N days)
func (s *Scraper) ScrapeRecent(days int) error {
	// Build URL for recent norms
	recentURL := fmt.Sprintf("%s/buscanormas?dataDe=%s", s.baseURL, 
		url.QueryEscape(time.Now().AddDate(0, 0, -days).Format("02/01/2006")))

	// Reset stats
	s.stats = ScraperStats{
		StartTime: time.Now(),
	}

	// Clear the channel
	close(s.normasChan)
	s.normasChan = make(chan models.Norma, 1000)

	// Visit the recent page
	err := s.collector.Visit(recentURL)
	if err != nil {
		return fmt.Errorf("failed to scrape recent: %w", err)
	}

	s.collector.Wait()
	s.processNormas()

	return nil
}
