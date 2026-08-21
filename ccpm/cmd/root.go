package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var version = "0.6.0"

var rootCmd = &cobra.Command{
	Use:   "ccpm",
	Short: "Claude Code Profile Manager",
	Long: `Manage multiple Claude Code accounts with isolated profiles, supporting both OAuth and API key authentication.

All data stays on your machine. ccpm does not collect, transmit, or store any
data externally. Credentials are stored in your OS keychain, config lives in
~/.ccpm/, and vault backups use AES-256-GCM encryption with a local master key.
No telemetry. No analytics. No network calls (the only exception: the
explicit opt-in ` + "`ccpm version --check-latest`" + ` release check). Fully open source.`,
	Version: version,
	// Suppress the auto-printed usage block on RunE errors. ccpm commands
	// already return human-readable error messages; reprinting the help text
	// for every failure (missing profile, wrong flag, etc.) buries the real
	// error in 40 lines of noise.
	SilenceUsage: true,
}

// registerProfileFlagCompletion walks the whole command tree and wires
// profile-name completion onto every `--profile` flag, so `ccpm mcp add
// --profile <TAB>` completes the same way positional profile args do.
func registerProfileFlagCompletion(cmd *cobra.Command) {
	if cmd.Flags().Lookup("profile") != nil {
		_ = cmd.RegisterFlagCompletionFunc("profile", completeProfileNames)
	}
	for _, sub := range cmd.Commands() {
		registerProfileFlagCompletion(sub)
	}
}

var logLevel string

// configureLogging maps --log-level onto the process-wide slog default.
// Default "warn" keeps existing output untouched; "debug" surfaces cascade,
// lock, and materialize decisions.
func configureLogging() {
	var lvl slog.Level
	switch strings.ToLower(logLevel) {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelWarn
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})))
}

func Execute() {
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "warn", "log verbosity: debug | info | warn | error")
	cobra.OnInitialize(configureLogging)
	registerProfileFlagCompletion(rootCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var coded *codedError
		if errors.As(err, &coded) {
			os.Exit(coded.code)
		}
		os.Exit(exitErr)
	}
}
