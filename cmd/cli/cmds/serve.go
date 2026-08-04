package cmds

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lemefy/lemefy-bacen/internal/models"
	"github.com/spf13/cobra"
)

type serveCmdFlags struct {
	port int
}

func NewServeCmd() *cobra.Command {
	flags := &serveCmdFlags{}

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server",
		Long:  "Start the HTTP server for the lemfey-bacen API and web interface",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := InitApp(configPath)
			if err != nil {
				return err
			}
			defer app.Close()

			if flags.port > 0 {
				app.Config.App.Port = flags.port
			}

			return runServe(app)
		},
	}

	cmd.Flags().IntVarP(&flags.port, "port", "p", 0, "Override server port")

	return cmd
}

func runServe(app *App) error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/normas", app.handleNormas)
	mux.HandleFunc("GET /api/normas/", app.handleNormaByID)
	mux.HandleFunc("GET /api/stats", app.handleStats)
	mux.HandleFunc("POST /api/scrape", app.handleScrape)
	mux.HandleFunc("GET /api/schedule", app.handleSchedule)
	mux.HandleFunc("POST /api/schedule", app.handleScheduleUpdate)
	mux.HandleFunc("GET /api/health", app.handleHealth)

	mux.Handle("/", http.FileServer(http.Dir("web")))

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", app.Config.App.Port),
		Handler:          mux,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:     30 * time.Second,
		IdleTimeout:      60 * time.Second,
	}

	fmt.Printf("Starting server on port %d...\n", app.Config.App.Port)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Server failed: %v\n", err)
			os.Exit(1)
		}
	}()

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-done

	fmt.Println("\nShutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Server shutdown error: %v\n", err)
	}

	fmt.Println("Server stopped")
	return nil
}

func (a *App) handleNormas(w http.ResponseWriter, r *http.Request) {
	search := &models.NormaSearch{
		Page:     1,
		PageSize: 50,
	}

	if r.URL.Query().Get("page") != "" {
		if _, err := fmt.Sscanf(r.URL.Query().Get("page"), "%d", &search.Page); err != nil {
			search.Page = 1
		}
	}

	if r.URL.Query().Get("page_size") != "" {
		if _, err := fmt.Sscanf(r.URL.Query().Get("page_size"), "%d", &search.PageSize); err != nil {
			search.PageSize = 50
		}
		if search.PageSize > 1000 {
			search.PageSize = 1000
		}
	}

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

	normas, total, err := a.Storage.ListNormas(search)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list normas: "+err.Error())
		return
	}

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

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to encode response: %v\n", err)
	}
}

func (a *App) handleNormaByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/api/normas/"):]
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid norma ID")
		return
	}

	norma, err := a.Storage.GetNormaByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get norma: "+err.Error())
		return
	}

	if norma == nil {
		writeError(w, http.StatusNotFound, "Norma not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(norma); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to encode norma: %v\n", err)
	}
}

func (a *App) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := a.Storage.GetStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get stats: "+err.Error())
		return
	}

	history, err := a.Storage.GetLatestScrapeHistory()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get scrape history: %v\n", err)
	}

	response := map[string]interface{}{
		"stats":   stats,
		"history": history,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to encode stats: %v\n", err)
	}
}

func (a *App) handleScrape(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	go func() {
		if err := a.Scraper.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Background scrape failed: %v\n", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "started", "message": "Scrape started in background"})
}

func (a *App) handleSchedule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	info, err := a.Scheduler.GetScheduleInfo()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get schedule info: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(info); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to encode schedule info: %v\n", err)
	}
}

func (a *App) handleScheduleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	go func() {
		if err := a.Scheduler.RunNow(); err != nil {
			fmt.Fprintf(os.Stderr, "Immediate update failed: %v\n", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "started", "message": "Immediate update started"})
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	_, err := a.Storage.GetStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database unhealthy")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

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