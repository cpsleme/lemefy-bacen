package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lemefy/lemefy-bacen/internal/config"
	"github.com/lemefy/lemefy-bacen/internal/meilisearch"
	"github.com/lemefy/lemefy-bacen/internal/models"
	"github.com/lemefy/lemefy-bacen/internal/scraper"
	"github.com/lemefy/lemefy-bacen/internal/scheduler"
	"github.com/lemefy/lemefy-bacen/internal/storage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	server    *mcp.Server
	config    *config.Config
	storage   *storage.Database
	scraper   *scraper.Scraper
	scheduler *scheduler.Scheduler
	meili     *meilisearch.Client
}

func NewServer() (*Server, error) {
	cfg, err := config.Init(".")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize config: %w", err)
	}

	db, err := storage.NewDatabase(cfg.Database.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	sc := scraper.NewScraper(cfg, db)
	sched := scheduler.NewScheduler(cfg, db, sc)

	s := &Server{
		config:    cfg,
		storage:   db,
		scraper:   sc,
		scheduler: sched,
	}

	if cfg.Meilisearch.Enabled {
		meili := meilisearch.NewClient(&cfg.Meilisearch, config.GetLogger())
		s.scraper.SetSearchClient(meili)
		if meili.Available() {
			if err := meili.EnsureIndex(); err != nil {
				config.GetLogger().WithError(err).Warn("Failed to ensure Meilisearch index")
			}
			if meili.NeedsLoad() {
				if all, err := db.GetAllNormas(); err == nil && len(all) > 0 {
					if n, err := meili.SyncAll(all); err != nil {
						config.GetLogger().WithError(err).Warn("Failed to bulk load normas into Meilisearch")
					} else if n > 0 {
						config.GetLogger().Infof("Loaded %d normas into Meilisearch", n)
					}
				}
			}
		}
		s.meili = meili
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "lemfey-bacen",
		Title:   "Lemfey Bacen MCP Server",
		Version: "1.0.0",
	}, nil)

	s.server = server
	s.registerTools()

	return s, nil
}

func (s *Server) registerTools() {
	s.server.AddTool(s.scrapeTool(), mcp.ToolHandler(s.handleScrape))
	s.server.AddTool(s.scrapeRecentTool(), mcp.ToolHandler(s.handleScrapeRecent))
	s.server.AddTool(s.scrapeByTypeTool(), mcp.ToolHandler(s.handleScrapeByType))
	s.server.AddTool(s.listNormasTool(), mcp.ToolHandler(s.handleListNormas))
	s.server.AddTool(s.getNormaTool(), mcp.ToolHandler(s.handleGetNorma))
	s.server.AddTool(s.getStatsTool(), mcp.ToolHandler(s.handleGetStats))
	s.server.AddTool(s.getScheduleInfoTool(), mcp.ToolHandler(s.handleGetScheduleInfo))
	s.server.AddTool(s.runNowTool(), mcp.ToolHandler(s.handleRunNow))
	s.server.AddTool(s.getConfigTool(), mcp.ToolHandler(s.handleGetConfig))
}

func (s *Server) Run(ctx context.Context) error {
	return s.server.Run(ctx, &mcp.StdioTransport{})
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}

func parseArgs(args json.RawMessage) map[string]any {
	var parsed map[string]any
	if err := json.Unmarshal(args, &parsed); err != nil {
		return map[string]any{}
	}
	return parsed
}

func (s *Server) scrapeTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "scrape",
		Description: "Run the scraper to fetch norms from Banco Central do Brasil",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tipo": map[string]any{
					"type":        "string",
					"description": "Scrape norms by type (Resolução, Circular, Instrução, Comunicado, Carta-Circular)",
				},
				"recent": map[string]any{
					"type":        "boolean",
					"description": "Scrape recent norms instead of all",
				},
				"days": map[string]any{
					"type":        "integer",
					"description": "Number of days for recent scrape (default: 30)",
				},
			},
		},
	}
}

func (s *Server) scrapeRecentTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "scrape_recent",
		Description: "Scrape recent norms from the last N days",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"days": map[string]any{
					"type":        "integer",
					"description": "Number of days to look back (default: 30)",
				},
			},
		},
	}
}

func (s *Server) scrapeByTypeTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "scrape_by_type",
		Description: "Scrape norms by type",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tipo": map[string]any{
					"type":        "string",
					"description": "Norm type (Resolução, Circular, Instrução, Comunicado, Carta-Circular)",
				},
			},
		},
	}
}

func (s *Server) listNormasTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "list_normas",
		Description: "Query norms from the database with optional filters",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tipo": map[string]any{
					"type":        "string",
					"description": "Filter by norm type",
				},
				"numero": map[string]any{
					"type":        "string",
					"description": "Filter by norm number",
				},
				"titulo": map[string]any{
					"type":        "string",
					"description": "Filter by title",
				},
				"situacao": map[string]any{
					"type":        "string",
					"description": "Filter by situation (Vigente, Revogada)",
				},
				"page": map[string]any{
					"type":        "integer",
					"description": "Page number (default: 1)",
				},
				"page_size": map[string]any{
					"type":        "integer",
					"description": "Items per page (default: 50, max: 1000)",
				},
			},
		},
	}
}

func (s *Server) getNormaTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_norma",
		Description: "Get a specific norm by ID",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "integer",
					"description": "Norm ID",
				},
			},
		},
	}
}

func (s *Server) getStatsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_stats",
		Description: "Get database statistics",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{},
		},
	}
}

func (s *Server) getScheduleInfoTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_schedule_info",
		Description: "Get scheduler status and configuration",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{},
		},
	}
}

func (s *Server) runNowTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "run_now",
		Description: "Run an immediate scheduler update (scrape)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{},
		},
	}
}

func (s *Server) getConfigTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_config",
		Description: "Get the current application configuration",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{},
		},
	}
}

func (s *Server) handleScrape(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := parseArgs(req.Params.Arguments)

	tipo, _ := args["tipo"].(string)
	recent, _ := args["recent"].(bool)
	days, _ := args["days"].(int)

	if days <= 0 {
		days = 30
	}

	var err error
	if recent {
		err = s.scraper.ScrapeRecent(days)
	} else if tipo != "" {
		err = s.scraper.ScrapeByTipo(models.TipoNorma(tipo))
	} else {
		err = s.scraper.Run()
	}

	if err != nil {
		return textResult(fmt.Sprintf("Scrape failed: %v", err)), nil
	}

	stats := s.scraper.GetStats()
	duration := stats.EndTime.Sub(stats.StartTime).Milliseconds()

	return textResult(fmt.Sprintf(
		"Scrape completed in %dms\nFound: %d, Added: %d, Updated: %d, Errors: %d",
		duration, stats.NormasFound, stats.NormasAdded, stats.NormasUpdated, stats.Errors,
	)), nil
}

func (s *Server) handleScrapeRecent(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := parseArgs(req.Params.Arguments)

	days, _ := args["days"].(int)
	if days <= 0 {
		days = 30
	}

	err := s.scraper.ScrapeRecent(days)
	if err != nil {
		return textResult(fmt.Sprintf("Recent scrape failed: %v", err)), nil
	}

	stats := s.scraper.GetStats()
	duration := stats.EndTime.Sub(stats.StartTime).Milliseconds()

	return textResult(fmt.Sprintf(
		"Recent scrape completed in %dms (last %d days)\nFound: %d, Added: %d, Updated: %d, Errors: %d",
		duration, days, stats.NormasFound, stats.NormasAdded, stats.NormasUpdated, stats.Errors,
	)), nil
}

func (s *Server) handleScrapeByType(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := parseArgs(req.Params.Arguments)

	tipo, _ := args["tipo"].(string)
	if tipo == "" {
		return textResult("Error: tipo parameter is required"), nil
	}

	err := s.scraper.ScrapeByTipo(models.TipoNorma(tipo))
	if err != nil {
		return textResult(fmt.Sprintf("Scrape by type failed: %v", err)), nil
	}

	stats := s.scraper.GetStats()
	duration := stats.EndTime.Sub(stats.StartTime).Milliseconds()

	return textResult(fmt.Sprintf(
		"Scrape by type '%s' completed in %dms\nFound: %d, Added: %d, Updated: %d, Errors: %d",
		tipo, duration, stats.NormasFound, stats.NormasAdded, stats.NormasUpdated, stats.Errors,
	)), nil
}

func (s *Server) handleListNormas(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := parseArgs(req.Params.Arguments)

	search := &models.NormaSearch{
		Page:     1,
		PageSize: 50,
	}

	if tipo, ok := args["tipo"].(string); ok && tipo != "" {
		t := models.TipoNorma(tipo)
		search.Tipo = &t
	}

	if numero, ok := args["numero"].(string); ok && numero != "" {
		search.Numero = &numero
	}

	if titulo, ok := args["titulo"].(string); ok && titulo != "" {
		search.Titulo = &titulo
	}

	if situacao, ok := args["situacao"].(string); ok && situacao != "" {
		search.Situacao = &situacao
	}

	if page, ok := args["page"].(int); ok && page > 0 {
		search.Page = page
	}

	if pageSize, ok := args["page_size"].(int); ok && pageSize > 0 {
		if pageSize > 1000 {
			pageSize = 1000
		}
		search.PageSize = pageSize
	}

	normas, total, err := s.storage.ListNormas(search)
	if err != nil {
		return textResult(fmt.Sprintf("Failed to list normas: %v", err)), nil
	}

	totalPages := total / search.PageSize
	if total%search.PageSize > 0 {
		totalPages++
	}

	result := fmt.Sprintf("Total: %d norms (Page %d of %d)\n\n", total, search.Page, totalPages)
	for _, n := range normas {
		result += fmt.Sprintf("[%d] %s %s - %s (%s)\n", n.ID, n.Tipo, n.Numero, n.Titulo, n.Situacao)
	}

	return textResult(result), nil
}

func (s *Server) handleGetNorma(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := parseArgs(req.Params.Arguments)

	id, ok := args["id"].(int)
	if !ok {
		return textResult("Error: id parameter is required and must be an integer"), nil
	}

	norma, err := s.storage.GetNormaByID(int64(id))
	if err != nil {
		return textResult(fmt.Sprintf("Failed to get norma: %v", err)), nil
	}

	if norma == nil {
		return textResult(fmt.Sprintf("Norma with ID %d not found", id)), nil
	}

	result := fmt.Sprintf("ID: %d\nNumero: %s\nTipo: %s\nTitulo: %s\nData Publicacao: %s\nData Vigencia: %s\nSituacao: %s\nAssunto: %s\nURL: %s\nArquivo PDF: %s\n",
		norma.ID, norma.Numero, norma.Tipo, norma.Titulo,
		norma.DataPublicacao, norma.DataVigencia, norma.Situacao,
		norma.Assunto, norma.URL, norma.ArquivoPDF,
	)

	return textResult(result), nil
}

func (s *Server) handleGetStats(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	stats, err := s.storage.GetStats()
	if err != nil {
		return textResult(fmt.Sprintf("Failed to get stats: %v", err)), nil
	}

	result := fmt.Sprintf("=== Database Statistics ===\nTotal norms: %d\nVigentes: %d\nRevogadas: %d\nLast update: %s\n\nBy type:\n",
		stats.TotalNormas, stats.NormasVigentes, stats.NormasRevogadas,
		stats.UltimaAtualizacao.Format("2006-01-02 15:04:05"),
	)

	for tipo, count := range stats.Tipos {
		result += fmt.Sprintf("  %s: %d\n", tipo, count)
	}

	return textResult(result), nil
}

func (s *Server) handleGetScheduleInfo(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	info, err := s.scheduler.GetScheduleInfo()
	if err != nil {
		return textResult(fmt.Sprintf("Failed to get schedule info: %v", err)), nil
	}

	result := fmt.Sprintf("=== Scheduler Status ===\nEnabled: %v\nRunning: %v\nUpdate cron: %s\nCleanup cron: %s\nCleanup days: %d\nLast update: %s\n",
		info.Enabled, info.IsRunning, info.UpdateCron, info.CleanupCron,
		info.CleanupDays, info.LastUpdate.Format("2006-01-02 15:04:05"),
	)

	if !info.NextUpdate.IsZero() {
		result += fmt.Sprintf("Next update: %s\n", info.NextUpdate.Format("2006-01-02 15:04:05"))
	}

	return textResult(result), nil
}

func (s *Server) handleRunNow(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	err := s.scheduler.RunNow()
	if err != nil {
		return textResult(fmt.Sprintf("Immediate update failed: %v", err)), nil
	}

	return textResult("Immediate update completed successfully"), nil
}

func (s *Server) handleGetConfig(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cfg := s.config

 	result := fmt.Sprintf("=== Application Configuration ===\nApp Name: %s\nVersion: %s\nEnvironment: %s\nPort: %d\n\nDatabase:\n  Path: %s\n\nMeilisearch:\n  Enabled:        %v\n  Host:           %s\n  Index Prefix:   %s\n  Index per tipo: bcb_resolucao, bcb_circular, bcb_instrucao, bcb_comunicado, bcb_carta_circular, bcb_outros\n\nScraper:\n  Base URL: %s\n  Timeout: %ds\n  Max Depth: %d\n  Concurrency: %d\n  Request Delay: %dms\n  Max Pages: %d\n  Page Size: %d\n\nScheduler:\n  Enabled: %v\n  Update cron: %s\n  Cleanup cron: %s\n  Cleanup days: %d\n\nLogging:\n  Level: %s\n  Format: %s\n  File: %s\n",
		cfg.App.Name, cfg.App.Version, cfg.App.Env, cfg.App.Port,
		cfg.Database.Path, cfg.Meilisearch.Enabled, cfg.Meilisearch.Host,
		cfg.Meilisearch.IndexPrefix,
		cfg.Scraper.BaseURL, cfg.Scraper.Timeout,
		cfg.Scraper.MaxDepth, cfg.Scraper.Concurrency, cfg.Scraper.RequestDelay,
		cfg.Scraper.MaxPages, cfg.Scraper.PageSize,
		cfg.Scheduler.Enabled, cfg.Scheduler.UpdateCron, cfg.Scheduler.CleanupCron,
		cfg.Scheduler.CleanupDays, cfg.Logging.Level, cfg.Logging.Format, cfg.Logging.File,
	)

	return textResult(result), nil
}