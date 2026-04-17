package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	verbose bool
	version = "0.2.2"
)

var rootCmd = &cobra.Command{
	Use:   "ccpm",
	Short: "Claude Code Profile Manager",
	Long: `Manage multiple Claude Code accounts with isolated profiles, supporting both OAuth and API key authentication.

All data stays on your machine. ccpm does not collect, transmit, or store any
data externally. Credentials are stored in your OS keychain, config lives in
~/.ccpm/, and vault backups use AES-256-GCM encryption with a local master key.
No telemetry. No analytics. No network calls. Fully open source.`,
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
