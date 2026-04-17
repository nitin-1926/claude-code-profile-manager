package cmd

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/ccpm/internal/shell"
)

var useCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Set profile for current shell session",
	Long: `Set the active Claude Code profile for the current shell session.

Requires the shell hook. Add this to your ~/.zshrc (or ~/.bashrc):
  eval "$(ccpm shell-init)"

Then reload your shell:
  source ~/.zshrc

Alternatively, use 'ccpm run <name>' which works without any shell setup.`,
	Args: cobra.ExactArgs(1),
	RunE: runUse,
}

func init() {
	rootCmd.AddCommand(useCmd)
}

func runUse(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	p, exists := cfg.Profiles[name]
	if !exists {
		return fmt.Errorf("profile %q not found", name)
	}

	maybeNudgeDefaultDrift(cfg)

	// Materialize shared settings/MCP before activating
	if err := settingsmerge.Materialize(p.Dir, name); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not materialize settings: %v\n", err)
	}
	if err := settingsmerge.MaterializeMCP(p.Dir, name); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not materialize MCP config: %v\n", err)
	}

	// Update last used
	cfg.UpdateLastUsed(name)
	_ = config.Save(cfg)

	s := shell.DetectShell()

	// If stdout is a terminal, that means we're being called directly
	// (not through the shell hook which captures stdout).
	// Warn the user that this won't work without the hook.
	if isTerminal() {
		yellow := color.New(color.FgYellow, color.Bold)
		yellow.Fprintln(os.Stderr, "Shell hook not detected!")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "`ccpm use` needs a shell hook to set environment variables.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Quick fix — run this in your terminal right now:")
		color.New(color.FgCyan).Fprintln(os.Stderr, "  eval \"$(ccpm shell-init)\"")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Permanent fix — add it to your shell config:")
		color.New(color.FgCyan).Fprintln(os.Stderr, "  echo 'eval \"$(ccpm shell-init)\"' >> ~/.zshrc && source ~/.zshrc")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Or just use this instead (works without any setup):")
		color.New(color.FgGreen, color.Bold).Fprintf(os.Stderr, "  ccpm run %s\n", name)
		fmt.Fprintln(os.Stderr, "")

		// Still print the exports so the user can copy-paste if they want
		fmt.Fprintln(os.Stderr, "Manual alternative — copy and paste this line:")
		color.New(color.FgCyan).Fprintf(os.Stderr, "  export CLAUDE_CONFIG_DIR='%s'\n", p.Dir)
		fmt.Fprintln(os.Stderr, "")
		return nil
	}

	// Called through shell hook — output export statements to be eval'd
	fmt.Print(shell.ExportStatements(s, p.Name, p.Dir))
	return nil
}

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
