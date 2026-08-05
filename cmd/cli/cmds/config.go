package cmds

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show current configuration",
		Long:  "Display the current application configuration loaded from config files and environment variables",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := InitApp(configPath)
			if err != nil {
				return err
			}
			defer app.Close()

			return runConfig(app)
		},
	}

	return cmd
}

func runConfig(app *App) error {
	if outputFmt == "json" {
		return outputJSON(app.Config)
	}

	cfg := app.Config

	fmt.Println("\n=== Application Configuration ===")
	fmt.Printf("App Name:    %s\n", cfg.App.Name)
	fmt.Printf("Version:     %s\n", cfg.App.Version)
	fmt.Printf("Environment: %s\n", cfg.App.Env)
	fmt.Printf("Port:        %d\n", cfg.App.Port)
	fmt.Println()
	fmt.Println("Database:")
	fmt.Printf("  Path: %s\n", cfg.Database.Path)
	fmt.Println()
		fmt.Println("Meilisearch:")
		fmt.Printf("  Enabled:        %v\n", cfg.Meilisearch.Enabled)
		fmt.Printf("  Host:           %s\n", cfg.Meilisearch.Host)
		fmt.Printf("  Index Prefix:   %s\n", cfg.Meilisearch.IndexPrefix)
		fmt.Printf("  Index per tipo: bcb_resolucao, bcb_circular, bcb_instrucao, bcb_comunicado, bcb_carta_circular, bcb_outros\n")
		if cfg.Meilisearch.APIKey != "" {
			fmt.Printf("  API Key:        set (length %d)\n", len(cfg.Meilisearch.APIKey))
		} else {
			fmt.Printf("  API Key:        (not set)\n")
		}
	fmt.Println()
		fmt.Println("Scraper:")
		fmt.Printf("  Base URL:        %s\n", cfg.Scraper.BaseURL)
		fmt.Printf("  Timeout:         %ds\n", cfg.Scraper.Timeout)
		fmt.Printf("  Max Depth:       %d\n", cfg.Scraper.MaxDepth)
		fmt.Printf("  Concurrency:     %d\n", cfg.Scraper.Concurrency)
		fmt.Printf("  Request Delay:   %dms\n", cfg.Scraper.RequestDelay)
		fmt.Printf("  Max Pages:       %d\n", cfg.Scraper.MaxPages)
		fmt.Printf("  Page Size:       %d\n", cfg.Scraper.PageSize)
	fmt.Println()
	fmt.Println("Scheduler:")
	fmt.Printf("  Enabled:      %v\n", cfg.Scheduler.Enabled)
	fmt.Printf("  Update cron:  %s\n", cfg.Scheduler.UpdateCron)
	fmt.Printf("  Cleanup cron: %s\n", cfg.Scheduler.CleanupCron)
	fmt.Printf("  Cleanup days: %d\n", cfg.Scheduler.CleanupDays)
	fmt.Println()
	fmt.Println("Logging:")
	fmt.Printf("  Level:  %s\n", cfg.Logging.Level)
	fmt.Printf("  Format: %s\n", cfg.Logging.Format)
	fmt.Printf("  File:   %s\n", cfg.Logging.File)
	fmt.Println()

	return nil
}