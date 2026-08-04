package cmds

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewStatsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show database statistics",
		Long:  "Display statistics about the stored norms in the database",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := InitApp(configPath)
			if err != nil {
				return err
			}
			defer app.Close()

			return runStats(app)
		},
	}

	return cmd
}

func runStats(app *App) error {
	stats, err := app.Storage.GetStats()
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	if outputFmt == "json" {
		return outputJSON(stats)
	}

	fmt.Println("\n=== Database Statistics ===")
	fmt.Printf("Total norms:          %d\n", stats.TotalNormas)
	fmt.Printf("Vigentes:            %d\n", stats.NormasVigentes)
	fmt.Printf("Revogadas:           %d\n", stats.NormasRevogadas)
	fmt.Printf("Last update:         %s\n", stats.UltimaAtualizacao.Format("2006-01-02 15:04:05"))
	fmt.Println("\nBy type:")
	for tipo, count := range stats.Tipos {
		fmt.Printf("  %-20s %d\n", tipo, count)
	}
	fmt.Println()

	return nil
}