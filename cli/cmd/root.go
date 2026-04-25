package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "0.4.1"

var rootCmd = &cobra.Command{
	Use:   "ccpm",
	Short: "Claude Code Profile Manager",
	Long: `Manage multiple Claude Code accounts with isolated profiles, supporting both OAuth and API key authentication.

All data stays on your machine. ccpm does not collect, transmit, or store any
data externally. Credentials are stored in your OS keychain, config lives in
~/.ccpm/, and vault backups use AES-256-GCM encryption with a local master key.
No telemetry. No analytics. No network calls. Fully open source.`,
	Version: version,
	// Suppress the auto-printed usage block on RunE errors. ccpm commands
	// already return human-readable error messages; reprinting the help text
	// for every failure (missing profile, wrong flag, etc.) buries the real
	// error in 40 lines of noise.
	SilenceUsage: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
