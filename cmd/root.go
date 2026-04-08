package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	verbose bool
	version = "0.1.0"
)

var rootCmd = &cobra.Command{
	Use:   "ccpm",
	Short: "Claude Code Profile Manager",
	Long:  "Manage multiple Claude Code accounts with isolated profiles, supporting both OAuth and API key authentication.",
	Version: version,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
}
