package main

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "linkrot",
	Short:   "Check links in local files",
	Long:    "linkrot reads links from local files and checks whether each URL is still alive.",
	Version: version,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
