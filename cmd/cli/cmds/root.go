package cmds

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	configPath string
	outputFmt  string
	verbose    bool
)

func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "lemefy-bacen",
		Short: "CLI tool for lemfey-bacen",
		Long:  "A CLI tool for scraping and managing Banco Central do Brasil norms",
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.Help(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		},
	}

	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", ".", "Path to config directory")
	rootCmd.PersistentFlags().StringVarP(&outputFmt, "output", "o", "text", "Output format (text, json)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	rootCmd.AddCommand(NewScrapeCmd())
	rootCmd.AddCommand(NewServeCmd())
	rootCmd.AddCommand(NewNormasCmd())
	rootCmd.AddCommand(NewStatsCmd())
	rootCmd.AddCommand(NewSchedulerCmd())
	rootCmd.AddCommand(NewConfigCmd())
	rootCmd.AddCommand(NewVersionCmd())

	return rootCmd
}