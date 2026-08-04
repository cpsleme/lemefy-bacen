package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lemefy/lemefy-bacen/internal/config"
	"github.com/lemefy/lemefy-bacen/internal/models"
	"github.com/lemefy/lemefy-bacen/internal/scheduler"
	"github.com/lemefy/lemefy-bacen/internal/scraper"
	"github.com/lemefy/lemefy-bacen/internal/storage"
	"github.com/sirupsen/logrus"
)

// App represents the main application
type App struct {
	config     *config.Config
	storage    *storage.Database
	scraper    *scraper.Scraper
	scheduler  *scheduler.Scheduler
	logger     *logrus.Logger
	httpServer *http.Server
}

func main() {
	// Initialize application
	app, err := NewApp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize application: %v\n", err)
		os.Exit(1)
	}

	// Handle signals
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Start scheduler if enabled
	if app.config.Scheduler.Enabled {
		if err := app.scheduler.Start(); err != nil {
			app.logger.WithError(err).Error("Failed to start scheduler")
		} else {
			app.logger.Info("Scheduler started")
		}
	}

	// Start HTTP server
	go func() {
		if err := app.StartHTTP(); err != nil && err != http.ErrServerClosed {
			app.logger.WithError(err).Error("HTTP server failed")
		}
	}()

	// Wait for signal
	<-done
	app.logger.Info("Shutting down...")

	// Cleanup
	app.Cleanup()
	app.logger.Info("Shutdown complete")
}

// NewApp creates a new application instance
func NewApp() (*App, error) {
	// Initialize config
	cfg, err := config.Init(".")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize config: %w", err)
	}

	logger := config.GetLogger()

	// Initialize database
	db, err := storage.NewDatabase(cfg.Database.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Initialize scraper
	sc := scraper.NewScraper(cfg, db)

	// Initialize scheduler
	sched := scheduler.NewScheduler(cfg, db, sc)

	return &App{
		config:    cfg,
		storage:   db,
		scraper:   sc,
		scheduler: sched,
		logger:    logger,
	}, nil
}

// Cleanup performs cleanup on shutdown
func (a *App) Cleanup() {
	// Stop scheduler
	if a.scheduler.IsRunning() {
		a.scheduler.Stop()
	}

	// Stop HTTP server
	if a.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		a.httpServer.Shutdown(ctx)
	}

	// Close database
	if a.storage != nil {
		a.storage.Close()
	}
}

// StartHTTP starts the HTTP server
func (a *App) StartHTTP() error {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("GET /api/normas", a.handleNormas)
	mux.HandleFunc("GET /api/normas/", a.handleNormaByID)
	mux.HandleFunc("GET /api/stats", a.handleStats)
	mux.HandleFunc("POST /api/scrape", a.handleScrape)
	mux.HandleFunc("GET /api/schedule", a.handleSchedule)
	mux.HandleFunc("POST /api/schedule", a.handleScheduleUpdate)
	mux.HandleFunc("GET /api/health", a.handleHealth)

	// Serve static files
	mux.Handle("/", http.FileServer(http.Dir("web")))

	a.httpServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", a.config.App.Port),
		Handler:          mux,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:     30 * time.Second,
		IdleTimeout:      60 * time.Second,
	}

	a.logger.WithField("port", a.config.App.Port).Info("Starting HTTP server")
	return a.httpServer.ListenAndServe()
}

// handleNormas handles GET /api/normas endpoint
func (a *App) handleNormas(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	search := &models.NormaSearch{
		Page:     1,
		PageSize: 50,
	}

	// Parse pagination
	if r.URL.Query().Get("page") != "" {
		if _, err := fmt.Sscanf(r.URL.Query().Get("page"), "%d", &search.Page); err != nil {
			search.Page = 1
		}
	}

	if r.URL.Query().Get("page_size") != "" {
		if _, err := fmt.Sscanf(r.URL.Query().Get("page_size"), "%d", &search.PageSize); err != nil {
			search.PageSize = 50
		}
		// Limit page size
		if search.PageSize > 1000 {
			search.PageSize = 1000
		}
	}

	// Parse filters
	if tipo := r.URL.Query().Get("tipo"); tipo != "" {
		t := models.TipoNorma(tipo)
		search.Tipo = &t
	}

	if numero := r.URL.Query().Get("numero"); numero != "" {
		search.Numero = &numero
	}

	if titulo := r.URL.Query().Get("titulo"); titulo != "" {
		search.Titulo = &titulo
	}

	if assunto := r.URL.Query().Get("assunto"); assunto != "" {
		search.Assunto = &assunto
	}

	if situacao := r.URL.Query().Get("situacao"); situacao != "" {
		search.Situacao = &situacao
	}

	// Parse dates
	if dataDe := r.URL.Query().Get("data_de"); dataDe != "" {
		if date, err := time.Parse("2006-01-02", dataDe); err == nil {
			search.DataDe = &date
		}
	}

	if dataAte := r.URL.Query().Get("data_ate"); dataAte != "" {
		if date, err := time.Parse("2006-01-02", dataAte); err == nil {
			search.DataAte = &date
		}
	}

	// List normas
	normas, total, err := a.storage.ListNormas(search)
	if err != nil {
		a.logger.WithError(err).Error("Failed to list normas")
		writeError(w, http.StatusInternalServerError, "Failed to list normas: "+err.Error())
		return
	}

	// Calculate total pages
	totalPages := total / search.PageSize
	if total%search.PageSize > 0 {
		totalPages++
	}

	response := &models.NormaResponse{
		Normas:     normas,
		Total:      total,
		Page:       search.Page,
		PageSize:   search.PageSize,
		TotalPages: totalPages,
	}

	// Return JSON
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		a.logger.WithError(err).Error("Failed to encode response")
	}
}

// handleNormaByID handles GET /api/normas/{id} endpoint
func (a *App) handleNormaByID(w http.ResponseWriter, r *http.Request) {
	// Extract ID from URL
	idStr := r.URL.Path[len("/api/normas/"):]
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid norma ID")
		return
	}

	// Get norma by ID
	norma, err := a.storage.GetNormaByID(id)
	if err != nil {
		a.logger.WithError(err).Error("Failed to get norma by ID")
		writeError(w, http.StatusInternalServerError, "Failed to get norma: "+err.Error())
		return
	}

	if norma == nil {
		writeError(w, http.StatusNotFound, "Norma not found")
		return
	}

	// Return JSON
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(norma); err != nil {
		a.logger.WithError(err).Error("Failed to encode norma")
	}
}

// handleStats handles GET /api/stats endpoint
func (a *App) handleStats(w http.ResponseWriter, r *http.Request) {
	// Get stats
	stats, err := a.storage.GetStats()
	if err != nil {
		a.logger.WithError(err).Error("Failed to get stats")
		writeError(w, http.StatusInternalServerError, "Failed to get stats: "+err.Error())
		return
	}

	// Get latest scrape history
	history, err := a.storage.GetLatestScrapeHistory()
	if err != nil {
		a.logger.WithError(err).Error("Failed to get scrape history")
	}

	// Combine responses
	response := map[string]interface{}{
		"stats":   stats,
		"history": history,
	}

	// Return JSON
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		a.logger.WithError(err).Error("Failed to encode stats")
	}
}

// handleScrape handles POST /api/scrape endpoint
func (a *App) handleScrape(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Run scraper in background
	go func() {
		if err := a.scraper.Run(); err != nil {
			a.logger.WithError(err).Error("Background scrape failed")
		}
	}()

	// Return success immediately
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "started", "message": "Scrape started in background"})
}

// handleSchedule handles GET /api/schedule endpoint
func (a *App) handleSchedule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Get schedule info
	info, err := a.scheduler.GetScheduleInfo()
	if err != nil {
		a.logger.WithError(err).Error("Failed to get schedule info")
		writeError(w, http.StatusInternalServerError, "Failed to get schedule info: "+err.Error())
		return
	}

	// Return JSON
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(info); err != nil {
		a.logger.WithError(err).Error("Failed to encode schedule info")
	}
}

// handleScheduleUpdate handles POST /api/schedule endpoint
func (a *App) handleScheduleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Run immediate update
	go func() {
		if err := a.scheduler.RunNow(); err != nil {
			a.logger.WithError(err).Error("Immediate update failed")
		}
	}()

	// Return success immediately
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "started", "message": "Immediate update started"})
}

// handleHealth handles GET /api/health endpoint
func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Check database connection
	_, err := a.storage.GetStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database unhealthy")
		return
	}

	// Return health status
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// writeError writes an error response
func writeError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    statusCode,
			"message": message,
			"time":    time.Now().UTC().Format(time.RFC3339),
		},
	})
}
