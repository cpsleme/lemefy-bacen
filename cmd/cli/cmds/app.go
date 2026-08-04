package cmds

import (
	"fmt"

	"github.com/lemefy/lemefy-bacen/internal/config"
	"github.com/lemefy/lemefy-bacen/internal/scraper"
	"github.com/lemefy/lemefy-bacen/internal/scheduler"
	"github.com/lemefy/lemefy-bacen/internal/storage"
)

type App struct {
	Config    *config.Config
	Storage   *storage.Database
	Scraper   *scraper.Scraper
	Scheduler *scheduler.Scheduler
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

	return &App{
		Config:    cfg,
		Storage:   db,
		Scraper:   sc,
		Scheduler: sched,
	}, nil
}

func (a *App) Close() {
	if a.Scheduler != nil && a.Scheduler.IsRunning() {
		a.Scheduler.Stop()
	}
	if a.Storage != nil {
		a.Storage.Close()
	}
}