package cmds

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

const (
	version = "1.0.0"
	commit  = ""
	date    = ""
)

func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Long:  "Display version, build date, and Go runtime information",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVersion()
		},
	}
}

func runVersion() error {
	info := map[string]string{
		"version": version,
		"commit":  commit,
		"date":    date,
		"go":      runtime.Version(),
		"os":      runtime.GOOS,
		"arch":    runtime.GOARCH,
	}

	if outputFmt == "json" {
		return outputJSON(info)
	}

	fmt.Printf("lemefy-bacen %s\n", version)
	fmt.Printf("Go %s (%s/%s)\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	if date != "" {
		fmt.Printf("Build date: %s\n", date)
	}
	if commit != "" {
		fmt.Printf("Commit: %s\n", commit)
	}
	fmt.Println()

	return nil
}