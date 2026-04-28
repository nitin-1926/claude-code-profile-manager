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
