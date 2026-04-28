package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/ccpm/internal/config"
)

// Reserved env keys ccpm owns and will not allow profile persistence for.
// CLAUDE_CONFIG_DIR is computed from the profile directory; ANTHROPIC_API_KEY
// is sourced from the keystore when auth_method=api_key. Accepting either here
// would let a stale profile value overwrite ccpm's launch contract.
var reservedEnvKeys = map[string]string{
	"CLAUDE_CONFIG_DIR": "computed from the profile directory",
	"ANTHROPIC_API_KEY": "sourced from the keystore for api_key profiles",
}

// envState holds the cobra flag-bound values for the `ccpm env` command tree.
type envState struct {
	profile string
}

func newEnvCmd() *cobra.Command {
	state := &envState{}

	root := &cobra.Command{
		Use:   "env",
		Short: "Manage environment variables carried into Claude Code launches",
		Long: `Manage the env map persisted on a profile.

Entries set here are added to the environment whenever ` + "`ccpm run <profile>`" + `
launches claude, sitting below parent-process env and below any one-shot
` + "`ccpm run --ccpm-env KEY=VAL`" + ` override. Use it for per-profile base URLs,
proxy settings, or CLAUDE_CODE_* knobs.

CLAUDE_CONFIG_DIR and ANTHROPIC_API_KEY are reserved — ccpm always computes
those — and cannot be set here.`,
	}

	setCmd := &cobra.Command{
		Use:   "set <KEY=VALUE> [KEY=VALUE...]",
		Short: "Set one or more env vars on a profile",
		Long: `Persist environment variables on a profile.

Examples:
  ccpm env set ANTHROPIC_BASE_URL=https://proxy.example.com --profile work
  ccpm env set CLAUDE_CODE_MAX_OUTPUT_TOKENS=32768 HTTPS_PROXY=http://localhost:8888 --profile work`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvSet(state, args)
		},
	}
	setCmd.Flags().StringVar(&state.profile, "profile", "", "target profile (required)")
	_ = setCmd.MarkFlagRequired("profile")

	unsetCmd := &cobra.Command{
		Use:   "unset <KEY> [KEY...]",
		Short: "Remove one or more env vars from a profile",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvUnset(state, args)
		},
	}
	unsetCmd.Flags().StringVar(&state.profile, "profile", "", "target profile (required)")
	_ = unsetCmd.MarkFlagRequired("profile")

	listCmd := &cobra.Command{
		Use:     "list",
		Short:   "List env vars persisted on a profile",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvList(state)
		},
	}
	listCmd.Flags().StringVar(&state.profile, "profile", "", "profile to list (required)")
	_ = listCmd.MarkFlagRequired("profile")

	root.AddCommand(setCmd, unsetCmd, listCmd)
	return root
}

func init() {
	rootCmd.AddCommand(newEnvCmd())
}

func runEnvSet(state *envState, args []string) error {
	pairs, err := parseEnvKVs(args)
	if err != nil {
		return err
	}
	for key := range pairs {
		if reason, reserved := reservedEnvKeys[key]; reserved {
			return fmt.Errorf("%q is reserved (%s) — set it with `ccpm run --ccpm-env KEY=VALUE` for a one-shot override instead", key, reason)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	p, exists := cfg.Profiles[state.profile]
	if !exists {
		return fmt.Errorf("profile %q not found", state.profile)
	}
	if p.Env == nil {
		p.Env = map[string]string{}
	}
	for k, v := range pairs {
		p.Env[k] = v
	}
	cfg.Profiles[state.profile] = p
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	green := color.New(color.FgGreen, color.Bold)
	keys := sortedKeys(pairs)
	green.Printf("✓ Set %d env var(s) on profile %q: %s\n", len(pairs), state.profile, strings.Join(keys, ", "))
	return nil
}

func runEnvUnset(state *envState, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	p, exists := cfg.Profiles[state.profile]
	if !exists {
		return fmt.Errorf("profile %q not found", state.profile)
	}
	if len(p.Env) == 0 {
		fmt.Printf("Profile %q has no persisted env vars.\n", state.profile)
		return nil
	}

	removed := 0
	for _, key := range args {
		if _, ok := p.Env[key]; ok {
			delete(p.Env, key)
			removed++
		}
	}
	if len(p.Env) == 0 {
		p.Env = nil
	}
	cfg.Profiles[state.profile] = p
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	color.New(color.FgGreen, color.Bold).Printf("✓ Unset %d env var(s) on profile %q\n", removed, state.profile)
	return nil
}

func runEnvList(state *envState) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	p, exists := cfg.Profiles[state.profile]
	if !exists {
		return fmt.Errorf("profile %q not found", state.profile)
	}
	if len(p.Env) == 0 {
		fmt.Printf("No env vars set on profile %q. Add one with: ccpm env set KEY=VALUE --profile %s\n", state.profile, state.profile)
		return nil
	}

	bold := color.New(color.Bold).SprintFunc()
	fmt.Printf("  %-30s %s\n", bold("KEY"), bold("VALUE"))
	fmt.Printf("  %s\n", strings.Repeat("─", 60))
	for _, k := range sortedKeys(p.Env) {
		fmt.Printf("  %-30s %s\n", k, p.Env[k])
	}
	return nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
