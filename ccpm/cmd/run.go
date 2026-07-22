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
	profilesync "github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/sync"
)

var runCmd = &cobra.Command{
	Use:   "run <name> [claude-args...]",
	Short: "Launch Claude Code with the given profile",
	Long: `Starts Claude Code with CLAUDE_CONFIG_DIR set to the profile's directory.

Everything after the profile name is forwarded to claude, including flags
ccpm doesn't know about:

  ccpm run work --dangerously-skip-permissions
  ccpm run work --model claude-sonnet-4-6

Five flags are intercepted by ccpm before they reach claude:
  --ccpm-env KEY=VALUE  — one-shot env override (repeatable)
  --no-auto-adopt       — skip the host-asset cascade scan for this launch
                          (does not change the persistent cascade_auto_adopt setting)
  --no-statusline       — skip injecting the default statusLine for this launch
                          (does not change the persistent statusline setting)
  --help / -h           — show this help
  --version             — show ccpm version

To forward --help or --version to claude, use:
  ccpm run work -- --help
  ccpm run work -- --version`,
	Args:               cobra.MinimumNArgs(1),
	DisableFlagParsing: true,
	RunE:               runRun,
	ValidArgsFunction:  completeProfileNames,
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	// With DisableFlagParsing the first arg after "run" is still the profile
	// name, but we own the parsing of anything ccpm-specific before it.
	claudeArgs, envOverrides, skipAdopt, skipStatusLine, helpRequested, versionRequested, err := extractCCPMRunFlags(args)
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

	// Pre-launch mutations are split by lock policy so a launch is never both
	// blocked AND allowed to corrupt shared state:
	//
	//   - Host adoption appends to the SHARED manifest (read-modify-write) and
	//     statusLine injection writes the profile fragment; both must be
	//     serialized so two `ccpm run`s in parallel terminals don't lose each
	//     other's updates (H7). If the lock is unavailable we SKIP this step
	//     with a warning — a missed adoption/injection is retried next launch —
	//     rather than run it unlocked and reintroduce the race.
	//   - Materialize only rewrites THIS profile's own settings.json/.claude.json
	//     via a single atomicwrite transaction, so it is safe to run unlocked
	//     and must NEVER be skipped: the launch needs an up-to-date settings.json.
	prelaunchLocked := func() error {
		// Idempotent — entries already in the manifest are skipped. Failures
		// are non-fatal so a launch is never blocked by a transient host-disk
		// issue.
		if err := profilesync.EnsureHostAdoption(p.Dir, name, skipAdopt); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: host-asset cascade: %v\n", err)
		}

		// Inject the default statusLine so the launched TUI shows which profile
		// is active (and, for subscription accounts, usage/limit windows).
		// Idempotent and skipped when the user opted out or already has one.
		if !skipStatusLine && cfg.Settings.StatusLineEnabled() {
			if wrote, err := settingsmerge.EnsureDefaultStatusLine(name, settingsmerge.DefaultStatusLineCommand); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not set default statusLine: %v\n", err)
			} else if wrote {
				fmt.Fprintf(os.Stderr, "ccpm: enabled status line for profile %q — disable with `ccpm config set statusline false`\n", name)
			}
		}

		// Inject a SessionEnd hook that keeps the per-profile usage store warm.
		// Opt-in (default off) and idempotent; `ccpm usage` works without it.
		if cfg.Settings.UsageTrackingEnabled() {
			if wrote, err := settingsmerge.EnsureUsageHook(name); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not install usage tracking hook: %v\n", err)
			} else if wrote {
				fmt.Fprintf(os.Stderr, "ccpm: usage tracking enabled for profile %q — disable with `ccpm config set usage_tracking false`\n", name)
			}
		} else if removed, err := settingsmerge.RemoveUsageHook(name); err != nil {
			// Turning the setting off has to uninstall the hook, not just stop
			// injecting it — otherwise it keeps firing on every session forever.
			fmt.Fprintf(os.Stderr, "Warning: could not remove usage tracking hook: %v\n", err)
		} else if removed {
			fmt.Fprintf(os.Stderr, "ccpm: usage tracking disabled for profile %q — removed the SessionEnd hook\n", name)
		}
		return nil
	}
	if lockErr := withConfigLock(prelaunchLocked); lockErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: skipping host cascade + statusline this launch (lock unavailable): %v\n", lockErr)
	}

	// Materialize shared settings + MCP into the profile dir before launch.
	// Profile-local and atomic, so it runs unlocked and always — even when the
	// lock above was unavailable — so the launch never uses stale settings.
	if err := settingsmerge.MaterializeAll(p.Dir, name, projectRoot); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not materialize profile settings: %v\n", err)
	}

	// Surface project-local assets so the user knows what's active in this
	// directory beyond what their profile already provides. Cheap directory
	// scan; printing one line beats showing nothing when a repo has its own
	// .claude/skills tree.
	maybeReportProjectAssets()

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
		k, v, ok := strings.Cut(raw, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("expected KEY=VALUE, got %q", raw)
		}
		out[k] = v
	}
	return out, nil
}

// extractCCPMRunFlags scans args for ccpm-owned flags (--ccpm-env,
// --no-auto-adopt, --help, --version) while leaving everything else —
// including flags unknown to ccpm — intact so they flow through to claude.
//
// Recognised shapes:
//
//	--ccpm-env KEY=VAL        two-token form
//	--ccpm-env=KEY=VAL        single-token form
//	--no-auto-adopt           boolean flag — skip host cascade for this run
//	--no-statusline           boolean flag — skip statusLine injection for this run
//	--help / -h / --version   boolean flags
//	--                        stop processing, pass the rest through verbatim
//
// Anything after a bare "--" is copied verbatim (including further --help or
// --ccpm-env occurrences), matching native shell convention.
func extractCCPMRunFlags(args []string) (forwarded []string, envOverrides []string, skipAdopt, skipStatusLine, help, ver bool, err error) {
	i := 0
	for i < len(args) {
		a := args[i]
		if a == "--" {
			forwarded = append(forwarded, args[i+1:]...)
			return forwarded, envOverrides, skipAdopt, skipStatusLine, help, ver, nil
		}
		switch {
		case a == "--ccpm-env":
			if i+1 >= len(args) {
				return nil, nil, false, false, false, false, fmt.Errorf("--ccpm-env requires a KEY=VALUE argument")
			}
			envOverrides = append(envOverrides, args[i+1])
			i += 2
			continue
		case strings.HasPrefix(a, "--ccpm-env="):
			envOverrides = append(envOverrides, strings.TrimPrefix(a, "--ccpm-env="))
			i++
			continue
		case a == "--no-auto-adopt":
			skipAdopt = true
			i++
			continue
		case a == "--no-statusline":
			skipStatusLine = true
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
	return forwarded, envOverrides, skipAdopt, skipStatusLine, help, ver, nil
}

// maybeReportProjectAssets prints a one-line stderr summary when CWD is
// inside a project tree containing .claude/<asset>/ entries. Silent when
// no project root is found or the project carries no assets.
func maybeReportProjectAssets() {
	plurals := []string{"skills", "agents", "commands", "rules"}
	counts := map[string]int{}
	root := ""
	for _, p := range plurals {
		r, entries := discoverProjectAssets(p)
		if r != "" {
			root = r
		}
		if n := len(entries); n > 0 {
			counts[p] = n
		}
	}
	if root == "" || len(counts) == 0 {
		return
	}
	parts := make([]string, 0, len(counts))
	for _, p := range plurals {
		if n := counts[p]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d", p, n))
		}
	}
	fmt.Fprintf(os.Stderr,
		"ccpm: project-local assets active (%s) from %s/.claude\n",
		strings.Join(parts, " "), root,
	)
}
