package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	claudepkg "github.com/nitin-1926/ccpm/internal/claude"
	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/keystore"
	"github.com/nitin-1926/ccpm/internal/settingsmerge"
)

var runCmd = &cobra.Command{
	Use:   "run <name> [claude-args...]",
	Short: "Launch Claude Code with the given profile",
	Long: `Starts Claude Code with CLAUDE_CONFIG_DIR set to the profile's directory.

Everything after the profile name is forwarded to claude, including flags
ccpm doesn't know about:

  ccpm run work --dangerously-skip-permissions
  ccpm run work --model claude-sonnet-4-6

Three flags are intercepted by ccpm before they reach claude:
  --ccpm-env KEY=VALUE  — one-shot env override (repeatable)
  --help / -h           — show this help
  --version             — show ccpm version

To forward --help or --version to claude, use:
  ccpm run work -- --help
  ccpm run work -- --version`,
	Args:               cobra.MinimumNArgs(1),
	DisableFlagParsing: true,
	RunE:               runRun,
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	// With DisableFlagParsing the first arg after "run" is still the profile
	// name, but we own the parsing of anything ccpm-specific before it.
	claudeArgs, envOverrides, helpRequested, versionRequested, err := extractCCPMRunFlags(args)
	if err != nil {
		return err
	}
	if helpRequested {
		return cmd.Help()
	}
	if versionRequested {
		fmt.Println(rootCmd.Version)
		return nil
	}
	if len(claudeArgs) == 0 {
		return fmt.Errorf("profile name is required. See `ccpm run --help`")
	}
	name := claudeArgs[0]
	claudeArgs = claudeArgs[1:]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	p, exists := cfg.Profiles[name]
	if !exists {
		return fmt.Errorf("profile %q not found. Run 'ccpm list' to see available profiles", name)
	}

	// Update last used
	cfg.UpdateLastUsed(name)
	_ = config.Save(cfg)

	maybeNudgeDefaultDrift(cfg)

	// Discover the project root (first ancestor of CWD containing a
	// .claude/settings.json, .claude/settings.local.json, or .mcp.json).
	// Empty string means "no project layer" — merge behaves as pre-feature.
	projectRoot := ""
	if cwd, werr := os.Getwd(); werr == nil {
		projectRoot = settingsmerge.FindProjectRoot(cwd)
	}

	// Materialize shared settings/MCP into the profile dir before launch
	if err := settingsmerge.Materialize(p.Dir, name, projectRoot); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not materialize settings: %v\n", err)
	}
	if err := settingsmerge.MaterializeMCP(p.Dir, name, projectRoot); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not materialize MCP config: %v\n", err)
