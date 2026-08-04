package main

import (
	"os"

	"github.com/lemefy/lemefy-bacen/cmd/cli/cmds"
)

func main() {
	rootCmd := cmds.NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}