package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/ccpm/internal/share"
)

// pluginState holds cobra flag-bound values for `ccpm plugin`. Scoped per
// invocation so callers that re-execute rootCmd don't see stale profile names.
type pluginState struct {
	profile string
}

func newPluginCmd() *cobra.Command {
	state := &pluginState{}

	root := &cobra.Command{
		Use:   "plugin",
		Short: "Manage Claude Code plugin activation per profile",
		Long: `Manage plugin activation per profile.

Plugin files are installed by Claude Code itself (run /plugin install <name> inside
a session). ccpm manages only the enabledPlugins settings key so profiles can
override which plugins are active — disable one in a specific profile, or
turn one on only for a subset.

The cross-profile baseline is ~/.claude/settings.json (written by Claude Code
when you install a plugin with "user" scope). A profile fragment can override
any key in that baseline.`,
	}

	listCmd := &cobra.Command{
		Use:     "list",
		Short:   "List installed plugins and their enabled state per profile",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginList(state)
		},
	}
	listCmd.Flags().StringVar(&state.profile, "profile", "", "limit output to one profile")

	enableCmd := &cobra.Command{
		Use:   "enable <plugin>",
		Short: "Enable a plugin for a profile",
		Long: `Enable a plugin for one profile.

The plugin must already be installed via Claude Code's /plugin install.
Use the full "<name>@<marketplace>" identifier shown in ccpm plugin list.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginSetEnabled(state, args[0], true)
		},
	}
	enableCmd.Flags().StringVar(&state.profile, "profile", "", "target profile (required)")
	_ = enableCmd.MarkFlagRequired("profile")

	disableCmd := &cobra.Command{
		Use:   "disable <plugin>",
		Short: "Disable a plugin for a profile (overrides global activation)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginSetEnabled(state, args[0], false)
		},
	}
	disableCmd.Flags().StringVar(&state.profile, "profile", "", "target profile (required)")
	_ = disableCmd.MarkFlagRequired("profile")

	installStub := &cobra.Command{
		Use:    "install <plugin>",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("ccpm does not install plugin files — run `/plugin install <plugin>` inside a Claude Code session (e.g. `ccpm run <profile>`); ccpm only manages per-profile activation via `ccpm plugin enable|disable`")
		},
	}

	root.AddCommand(listCmd, enableCmd, disableCmd, installStub)
	return root
}

func init() {
	rootCmd.AddCommand(newPluginCmd())
}

func runPluginSetEnabled(state *pluginState, pluginID string, enabled bool) error {
	if err := ensureProfileExists(state.profile); err != nil {
		return err
	}
	if err := share.EnsureDirs(); err != nil {
		return err
	}

	fragPath, err := settingsFragmentPath(state.profile)
	if err != nil {
		return err
	}

	frag, err := settingsmerge.LoadJSON(fragPath)
	if err != nil {
		return fmt.Errorf("loading fragment: %w", err)
	}

	key := "enabledPlugins." + pluginID
	setNestedKey(frag, key, enabled)

	if err := settingsmerge.WriteJSON(fragPath, frag); err != nil {
		return fmt.Errorf("writing fragment: %w", err)
	}
	if err := settingsmerge.MarkOwned(fragPath, key); err != nil {
		return fmt.Errorf("recording owned key: %w", err)
	}

	verb := "enabled"
	if !enabled {
		verb = "disabled"
	}
	color.New(color.FgGreen, color.Bold).Printf("✓ Plugin %q %s for profile %q\n", pluginID, verb, state.profile)
	return nil
}

func runPluginList(state *pluginState) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	installed, err := loadInstalledPlugins()
	if err != nil {
		return fmt.Errorf("reading installed plugins: %w", err)
	}

	profiles := config.ProfileNames(cfg)
	if state.profile != "" {
		if _, ok := cfg.Profiles[state.profile]; !ok {
			return fmt.Errorf("profile %q not found", state.profile)
		}
		profiles = []string{state.profile}
