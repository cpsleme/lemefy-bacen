package cmds

import (
	"fmt"

	"github.com/lemefy/lemefy-bacen/internal/config"
	"github.com/lemefy/lemefy-bacen/internal/meilisearch"
	"github.com/lemefy/lemefy-bacen/internal/scraper"
	"github.com/lemefy/lemefy-bacen/internal/scheduler"
	"github.com/lemefy/lemefy-bacen/internal/storage"
)

type App struct {
	Config    *config.Config
	Storage   *storage.Database
	Scraper   *scraper.Scraper
	Scheduler *scheduler.Scheduler
	Meili     *meilisearch.Client
}

func InitApp(configPath string) (*App, error) {
	cfg, err := config.Init(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize config: %w", err)
	}

	db, err := storage.NewDatabase(cfg.Database.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	sc := scraper.NewScraper(cfg, db)
	sched := scheduler.NewScheduler(cfg, db, sc)

	app := &App{
		Config:    cfg,
		Storage:   db,
		Scraper:   sc,
		Scheduler: sched,
	}

	// Optionally mirror scraped data to a local Meilisearch instance.
	if cfg.Meilisearch.Enabled {
		logger := config.GetLogger()
		meili := meilisearch.NewClient(&cfg.Meilisearch, logger)
		app.Meili = meili
		sc.SetSearchClient(meili)
		if meili.Available() {
			if err := meili.EnsureIndex(); err != nil {
				logger.WithError(err).Warn("Failed to ensure Meilisearch index")
			}
		// Perform an initial bulk load of existing normas when any target
		// index is empty, bucketing them by tipo into one index per type.
			if meili.NeedsLoad() {
				if all, err := db.GetAllNormas(); err == nil && len(all) > 0 {
					if n, err := meili.SyncAll(all); err != nil {
						logger.WithError(err).Warn("Failed to bulk load normas into Meilisearch")
					} else if n > 0 {
						logger.Infof("Loaded %d normas into Meilisearch", n)
					}
				}
			}
		}
	}

	return app, nil
}

func (a *App) Close() {
	if a.Scheduler != nil && a.Scheduler.IsRunning() {
		a.Scheduler.Stop()
	}
	if a.Meili != nil {
		a.Meili.Close()
	}
	if a.Storage != nil {
		a.Storage.Close()
	}
}