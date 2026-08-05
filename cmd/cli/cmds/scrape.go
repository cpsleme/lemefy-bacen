package cmds

import (
	"fmt"

	"github.com/lemefy/lemefy-bacen/internal/models"
	"github.com/spf13/cobra"
)

type scrapeCmdFlags struct {
	tipo   string
	recent bool
	days   int
	limit  int
}

func NewScrapeCmd() *cobra.Command {
	flags := &scrapeCmdFlags{}

	cmd := &cobra.Command{
		Use:   "scrape",
		Short: "Run the scraper to fetch norms from Banco Central",
		Long:  "Run the web scraper to fetch norms from the Banco Central do Brasil website and store them in the database",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := InitApp(configPath)
			if err != nil {
				return err
			}
			defer app.Close()

			if flags.recent {
				if flags.days <= 0 {
					flags.days = 30
				}
				return runScrapeRecent(app, flags.days)
			}

		if flags.tipo != "" {
			return runScrapeByTipo(app, flags.tipo, flags.limit)
		}

		if flags.limit > 0 {
			return runScrapeAllWithLimit(app, flags.limit)
		}

		return runScrape(app)
		},
	}

		cmd.Flags().StringVarP(&flags.tipo, "tipo", "t", "", "Scrape norms by type (Resolução, Circular, Instrução, Comunicado, Carta-Circular)")
		cmd.Flags().BoolVarP(&flags.recent, "recent", "r", false, "Scrape recent norms")
		cmd.Flags().IntVarP(&flags.days, "days", "d", 30, "Number of days for recent scrape (default: 30)")
		cmd.Flags().IntVarP(&flags.limit, "limit", "l", 0, "Limit number of norms to scrape per type (0 = no limit)")

	return cmd
}

func runScrape(app *App) error {
	fmt.Println("Starting scraper...")

	err := app.Scraper.Run()
	if err != nil {
		return fmt.Errorf("scrape failed: %w", err)
	}

	printScrapeStats(app)
	return nil
}

func runScrapeByTipo(app *App, tipo string, limit int) error {
	normaTipo := models.TipoNorma(tipo)
	fmt.Printf("Scraping norms by type: %s (limit: %d)\n", normaTipo, limit)

	err := app.Scraper.ScrapeByTipoWithLimit(normaTipo, limit)
	if err != nil {
		return fmt.Errorf("scrape by tipo failed: %w", err)
	}

	printScrapeStats(app)
	return nil
}

func runScrapeAllWithLimit(app *App, limit int) error {
	tipos := []models.TipoNorma{
		models.TipoResolucao,
		models.TipoCircular,
		models.TipoInstrucao,
		models.TipoComunicado,
		models.TipoCartaCircular,
		models.TipoOutros,
	}

	totalFound := 0
	totalAdded := 0
	totalUpdated := 0
	totalErrors := 0

	for _, tipo := range tipos {
		fmt.Printf("Scraping %s (limit: %d)...\n", tipo, limit)

		err := app.Scraper.ScrapeByTipoWithLimit(tipo, limit)
		if err != nil {
			fmt.Printf("Error scraping %s: %v\n", tipo, err)
			totalErrors++
			continue
		}

		s := app.Scraper.GetStats()
		totalFound += s.NormasFound
		totalAdded += s.NormasAdded
		totalUpdated += s.NormasUpdated
		totalErrors += s.Errors
	}

	fmt.Printf("\n=== Total ===\n")
	fmt.Printf("Found: %d, Added: %d, Updated: %d, Errors: %d\n", totalFound, totalAdded, totalUpdated, totalErrors)
	return nil
}

func runScrapeRecent(app *App, days int) error {
	fmt.Printf("Scraping recent norms from the last %d days...\n", days)

	err := app.Scraper.ScrapeRecent(days)
	if err != nil {
		return fmt.Errorf("recent scrape failed: %w", err)
	}

	printScrapeStats(app)
	return nil
}

func printScrapeStats(app *App) {
	stats := app.Scraper.GetStats()
	duration := stats.EndTime.Sub(stats.StartTime).Milliseconds()

	fmt.Printf("Scrape completed in %dms\n", duration)
	fmt.Printf("Found: %d, Added: %d, Updated: %d, Errors: %d\n",
		stats.NormasFound, stats.NormasAdded, stats.NormasUpdated, stats.Errors)
}