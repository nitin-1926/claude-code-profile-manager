package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

var (
	promptFormat      string
	promptShowDefault bool
)

var promptCmd = &cobra.Command{
	Use:   "prompt",
	Short: "Print the active profile name for embedding in a shell prompt",
	Long: `Prints the profile that the current shell/session is using, so you can show
it in your shell prompt (PS1, starship, powerlevel10k, etc.) and always know
which Claude Code account a terminal is bound to.

Resolution order:
  1. $CCPM_ACTIVE_PROFILE  (set by 'ccpm use')
  2. $CLAUDE_CONFIG_DIR    (matched back to a known profile dir)
  3. the configured default profile, only with --show-default

Prints nothing (exit 0) when no profile is active, so it stays quiet in
non-ccpm shells. Use --format to wrap the name, e.g. --format '(%s) '.

Examples:
  # bash/zsh PS1
  PS1='$(ccpm prompt --format "[ccpm:%s] ")'"$PS1"

  # starship custom command
  command = "ccpm prompt"`,
	Args: cobra.NoArgs,
	RunE: runPrompt,
}

func init() {
	promptCmd.Flags().StringVar(&promptFormat, "format", "%s", "printf-style format applied to the name; must contain a single %s")
	promptCmd.Flags().BoolVar(&promptShowDefault, "show-default", false, "fall back to the configured default profile when nothing is active")
	rootCmd.AddCommand(promptCmd)
}

func runPrompt(cmd *cobra.Command, args []string) error {
	name := activeProfileName()
	if name == "" {
		return nil // quiet: not a ccpm-bound shell
	}
	format := promptFormat
	if !strings.Contains(format, "%s") {
		// Guard against a format with no verb (would drop the name silently).
		format = "%s"
	}
	fmt.Fprintf(cmd.OutOrStdout(), format, name)
	return nil
}

// activeProfileName resolves which profile the current environment is using,
// without loading credentials or touching the keychain (this runs on every
// prompt render, so it must be cheap and never fail loudly).
func activeProfileName() string {
	if n := strings.TrimSpace(os.Getenv("CCPM_ACTIVE_PROFILE")); n != "" {
		return n
	}

	cfg, err := config.Load()
	if err != nil {
		return ""
	}

	if dir := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); dir != "" {
		want := filepath.Clean(dir)
		for name, p := range cfg.Profiles {
			if filepath.Clean(p.Dir) == want {
				return name
			}
		}
		return ""
	}

	if promptShowDefault && cfg.DefaultProfile != "" {
		return cfg.DefaultProfile
	}
	return ""
}
