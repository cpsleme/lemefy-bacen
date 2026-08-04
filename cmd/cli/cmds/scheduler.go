package cmds

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewSchedulerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scheduler",
		Short: "Manage the scheduler",
		Long:  "View and control the automatic update scheduler for norm scraping",
	}

	cmd.AddCommand(NewSchedulerStatusCmd())
	cmd.AddCommand(NewSchedulerRunNowCmd())
	cmd.AddCommand(NewSchedulerEnableCmd())
	cmd.AddCommand(NewSchedulerDisableCmd())
	cmd.AddCommand(NewSchedulerConfigCmd())

	return cmd
}

func NewSchedulerStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show scheduler status",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := InitApp(configPath)
			if err != nil {
				return err
			}
			defer app.Close()

			return runSchedulerStatus(app)
		},
	}
}

func NewSchedulerRunNowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run-now",
		Short: "Run an immediate scheduler update",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := InitApp(configPath)
			if err != nil {
				return err
			}
			defer app.Close()

			return runSchedulerRunNow(app)
		},
	}
}

func NewSchedulerEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable",
		Short: "Enable the scheduler",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := InitApp(configPath)
			if err != nil {
				return err
			}
			defer app.Close()

			app.Config.Scheduler.Enabled = true
			fmt.Println("Scheduler enabled")
			return nil
		},
	}
}

func NewSchedulerDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "Disable the scheduler",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := InitApp(configPath)
			if err != nil {
				return err
			}
			defer app.Close()

			app.Config.Scheduler.Enabled = false
			fmt.Println("Scheduler disabled")
			return nil
		},
	}
}

func NewSchedulerConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show scheduler configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := InitApp(configPath)
			if err != nil {
				return err
			}
			defer app.Close()

			return runSchedulerConfig(app)
		},
	}
}

func runSchedulerStatus(app *App) error {
	info, err := app.Scheduler.GetScheduleInfo()
	if err != nil {
		return fmt.Errorf("failed to get schedule info: %w", err)
	}

	if outputFmt == "json" {
		return outputJSON(info)
	}

	fmt.Println("\n=== Scheduler Status ===")
	fmt.Printf("Enabled:        %v\n", info.Enabled)
	fmt.Printf("Running:        %v\n", info.IsRunning)
	fmt.Printf("Update cron:    %s\n", info.UpdateCron)
	fmt.Printf("Cleanup cron:   %s\n", info.CleanupCron)
	fmt.Printf("Cleanup days:   %d\n", info.CleanupDays)
	fmt.Printf("Last update:    %s\n", info.LastUpdate.Format("2006-01-02 15:04:05"))
	if !info.NextUpdate.IsZero() {
		fmt.Printf("Next update:    %s\n", info.NextUpdate.Format("2006-01-02 15:04:05"))
	}
	fmt.Println()

	return nil
}

func runSchedulerRunNow(app *App) error {
	fmt.Println("Running immediate update...")

	err := app.Scheduler.RunNow()
	if err != nil {
		return fmt.Errorf("immediate update failed: %w", err)
	}

	fmt.Println("Immediate update completed")
	return nil
}

func runSchedulerConfig(app *App) error {
	cfg := app.Config.Scheduler

	if outputFmt == "json" {
		return outputJSON(cfg)
	}

	fmt.Println("\n=== Scheduler Configuration ===")
	fmt.Printf("Enabled:      %v\n", cfg.Enabled)
	fmt.Printf("Update cron:  %s\n", cfg.UpdateCron)
	fmt.Printf("Cleanup cron: %s\n", cfg.CleanupCron)
	fmt.Printf("Cleanup days: %d\n", cfg.CleanupDays)
	fmt.Println()

	return nil
}