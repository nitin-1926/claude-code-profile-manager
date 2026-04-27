package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	claudepkg "github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/claude"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/keystore"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
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
	}

	// Get API key if needed
	var apiKey string
	if p.AuthMethod == "api_key" {
		store := keystore.New()
		apiKey, err = store.GetAPIKey(name)
		if err != nil {
			return fmt.Errorf("retrieving API key: %w\nRun 'ccpm auth refresh %s' to re-enter your key", err, name)
		}
	}

	extraEnv, err := parseEnvKVs(envOverrides)
	if err != nil {
		return fmt.Errorf("parsing --ccpm-env: %w", err)
	}

	fmt.Printf("Launching Claude Code with profile: %s\n", name)
	fmt.Printf("Config dir: %s\n\n", p.Dir)

	// Exec replaces this process with claude
	return claudepkg.Exec(p.Dir, apiKey, p.Env, extraEnv, claudeArgs)
}

// parseEnvKVs converts a slice of "KEY=VALUE" strings to a map. An entry
// without "=" is an error so a typo can't silently drop a variable.
func parseEnvKVs(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(pairs))
	for _, raw := range pairs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		idx := strings.IndexByte(raw, '=')
		if idx <= 0 {
			return nil, fmt.Errorf("expected KEY=VALUE, got %q", raw)
		}
		key := raw[:idx]
		value := raw[idx+1:]
		out[key] = value
	}
	return out, nil
}

// extractCCPMRunFlags scans args for ccpm-owned flags (--ccpm-env, --help,
// --version) while leaving everything else — including flags unknown to ccpm
// — intact so they flow through to claude.
//
// Recognised shapes:
//
//	--ccpm-env KEY=VAL        two-token form
//	--ccpm-env=KEY=VAL        single-token form
//	--help / -h / --version   boolean flags
//	--                        stop processing, pass the rest through verbatim
//
// Anything after a bare "--" is copied verbatim (including further --help or
// --ccpm-env occurrences), matching native shell convention.
func extractCCPMRunFlags(args []string) (forwarded []string, envOverrides []string, help, ver bool, err error) {
	i := 0
	for i < len(args) {
		a := args[i]
		if a == "--" {
			forwarded = append(forwarded, args[i+1:]...)
			return forwarded, envOverrides, help, ver, nil
		}
		switch {
		case a == "--ccpm-env":
			if i+1 >= len(args) {
				return nil, nil, false, false, fmt.Errorf("--ccpm-env requires a KEY=VALUE argument")
			}
			envOverrides = append(envOverrides, args[i+1])
			i += 2
			continue
		case strings.HasPrefix(a, "--ccpm-env="):
			envOverrides = append(envOverrides, strings.TrimPrefix(a, "--ccpm-env="))
			i++
			continue
		case a == "--help" || a == "-h":
			help = true
			i++
			continue
		case a == "--version":
			ver = true
			i++
			continue
		}
		forwarded = append(forwarded, a)
		i++
	}
	return forwarded, envOverrides, help, ver, nil
}
