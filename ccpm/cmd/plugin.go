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

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
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
	}
	sort.Strings(profiles)

	// Build enablement matrix: pluginID -> (profileName -> bool).
	enabled := make(map[string]map[string]bool)
	for _, name := range profiles {
		p := cfg.Profiles[name]
		merged, err := buildMergedSettings(p.Dir, name)
		if err != nil {
			return fmt.Errorf("merging settings for %s: %w", name, err)
		}
		raw, _ := merged["enabledPlugins"].(map[string]interface{})
		for pluginID, v := range raw {
			b, _ := v.(bool)
			if _, ok := enabled[pluginID]; !ok {
				enabled[pluginID] = make(map[string]bool)
			}
			enabled[pluginID][name] = b
		}
	}

	// Union of installed + any enabled plugin (covers cases where a plugin
	// is enabled in a profile but no longer present in installed_plugins.json).
	ids := make(map[string]bool)
	for _, p := range installed {
		ids[p.id()] = true
	}
	for id := range enabled {
		ids[id] = true
	}

	if len(ids) == 0 {
		fmt.Println("No plugins installed. Run `/plugin install <name>` inside a Claude Code session.")
		return nil
	}

	sortedIDs := make([]string, 0, len(ids))
	for id := range ids {
		sortedIDs = append(sortedIDs, id)
	}
	sort.Strings(sortedIDs)

	bold := color.New(color.Bold).SprintFunc()
	fmt.Printf("  %-40s %-10s %s\n", bold("PLUGIN"), bold("INSTALLED"), bold("ENABLED IN"))
	fmt.Printf("  %s\n", strings.Repeat("─", 80))

	installedSet := make(map[string]bool)
	for _, p := range installed {
		installedSet[p.id()] = true
	}

	for _, id := range sortedIDs {
		installedCol := "—"
		if installedSet[id] {
			installedCol = "yes"
		}

		var enabledIn []string
		for _, name := range profiles {
			if enabled[id][name] {
				enabledIn = append(enabledIn, name)
			}
		}
		sort.Strings(enabledIn)
		enabledCol := strings.Join(enabledIn, ", ")
		if enabledCol == "" {
			enabledCol = "(none)"
		}
		fmt.Printf("  %-40s %-10s %s\n", id, installedCol, enabledCol)
	}
	return nil
}

// installedPlugin mirrors one entry in ~/.claude/plugins/installed_plugins.json.
type installedPlugin struct {
	Name        string `json:"name"`
	Marketplace string `json:"marketplace"`
	Version     string `json:"version"`
}

func (p installedPlugin) id() string {
	if p.Marketplace != "" {
		return p.Name + "@" + p.Marketplace
	}
	return p.Name
}

// loadInstalledPlugins reads ~/.claude/plugins/installed_plugins.json. Missing
// file is not an error (returns empty slice). The file's schema has shifted
// between Claude Code releases, so this tries a few common shapes.
func loadInstalledPlugins() ([]installedPlugin, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, ".claude", "plugins", "installed_plugins.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Shape 1: top-level array of plugin objects.
	var arr []installedPlugin
	if err := json.Unmarshal(data, &arr); err == nil && len(arr) > 0 {
		return arr, nil
	}

	// Shape 2: object keyed by plugin ID with metadata values.
	var obj map[string]struct {
		Marketplace string `json:"marketplace"`
		Version     string `json:"version"`
	}
	if err := json.Unmarshal(data, &obj); err == nil {
		out := make([]installedPlugin, 0, len(obj))
		for id, meta := range obj {
			name := id
			marketplace := meta.Marketplace
			if i := strings.Index(id, "@"); i >= 0 {
				name = id[:i]
				if marketplace == "" {
					marketplace = id[i+1:]
				}
			}
			out = append(out, installedPlugin{Name: name, Marketplace: marketplace, Version: meta.Version})
		}
		return out, nil
	}

	// Shape 3: object with an "installs" or "plugins" array.
	var wrap struct {
		Installs []installedPlugin `json:"installs"`
		Plugins  []installedPlugin `json:"plugins"`
	}
	if err := json.Unmarshal(data, &wrap); err == nil {
		if len(wrap.Installs) > 0 {
			return wrap.Installs, nil
		}
		if len(wrap.Plugins) > 0 {
			return wrap.Plugins, nil
		}
	}

	return nil, nil
}
