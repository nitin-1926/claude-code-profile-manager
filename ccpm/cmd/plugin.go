package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/plugins"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
)

// pluginState holds cobra flag-bound values for `ccpm plugin`. Scoped per
// invocation so callers that re-execute rootCmd don't see stale profile names.
type pluginState struct {
	profile     string
	global      bool
	installOnly bool
	ssh         bool
}

func newPluginCmd() *cobra.Command {
	state := &pluginState{}

	root := &cobra.Command{
		Use:   "plugin",
		Short: "Install, remove, and toggle Claude Code plugins per profile",
		Long: `Manage plugins end-to-end without entering a Claude Code session.

ccpm clones marketplaces into a shared store, fetches plugin files into a
shared cache (deduped across profiles), and writes Claude Code-shaped state
files (installed_plugins.json, known_marketplaces.json) into each target
profile alongside the enabledPlugins activation key in the profile's settings
fragment.

Typical flow:
  ccpm plugin marketplace add <org>/<repo>
  ccpm plugin install <name>@<marketplace> --global

ccpm reads installs created by Claude Code's /plugin install too — running
both side-by-side is supported. Use ccpm plugin gc (or ccpm sync) to reclaim
disk space from removed plugins.`,
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

	installCmd := &cobra.Command{
		Use:   "install <plugin>@<marketplace>",
		Short: "Install a plugin into one or every profile",
		Long: `Install a plugin from a registered marketplace.

The marketplace must already be registered with ` + "`ccpm plugin marketplace add`" + `.
ccpm fetches the plugin into a shared cache (~/.ccpm/share/plugins/cache),
symlinks it into each target profile, updates that profile's
installed_plugins.json + known_marketplaces.json, and (unless --install-only
is given) sets enabledPlugins.<id>=true in the profile settings fragment so
the plugin is active on the next ` + "`ccpm run`" + `.

Use --global to install into every profile, or --profile <name> for one
profile.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginInstall(state, args[0])
		},
	}
	installCmd.Flags().StringVar(&state.profile, "profile", "", "target profile (mutually exclusive with --global)")
	installCmd.Flags().BoolVar(&state.global, "global", false, "install into every profile")
	installCmd.Flags().BoolVar(&state.installOnly, "install-only", false, "install but do not enable (skip writing enabledPlugins)")
	installCmd.Flags().BoolVar(&state.ssh, "ssh", false, "clone marketplaces and plugin sources via SSH (default is HTTPS)")

	removeCmd := &cobra.Command{
		Use:   "remove <plugin>@<marketplace>",
		Short: "Remove an installed plugin from one or every profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginRemove(state, args[0])
		},
	}
	removeCmd.Flags().StringVar(&state.profile, "profile", "", "target profile (mutually exclusive with --global)")
	removeCmd.Flags().BoolVar(&state.global, "global", false, "remove from every profile")

	gcCmd := &cobra.Command{
		Use:   "gc",
		Short: "Garbage-collect plugin cache entries no profile references",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginGC()
		},
	}

	root.AddCommand(listCmd, enableCmd, disableCmd, installCmd, removeCmd, gcCmd, newPluginMarketplaceCmd(state))
	return root
}

func newPluginMarketplaceCmd(state *pluginState) *cobra.Command {
	mkt := &cobra.Command{
		Use:   "marketplace",
		Short: "Manage plugin marketplaces",
	}

	addCmd := &cobra.Command{
		Use:   "add <github-org>/<repo>",
		Short: "Register a marketplace by cloning it into the shared store",
		Long: `Clone a plugin marketplace into ~/.ccpm/share/plugins/marketplaces/<name>.

Defaults to HTTPS so the clone works without a configured SSH key — pass
--ssh to opt back into git@ URLs.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMarketplaceAdd(state, args[0])
		},
	}
	addCmd.Flags().BoolVar(&state.ssh, "ssh", false, "clone via SSH instead of HTTPS")

	rmCmd := &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   "Remove a registered marketplace from the shared store",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMarketplaceRemove(args[0])
		},
	}

	listCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List registered marketplaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMarketplaceList()
		},
	}

	mkt.AddCommand(addCmd, rmCmd, listCmd)
	return mkt
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

	profiles := config.ProfileNames(cfg)
	if state.profile != "" {
		if _, ok := cfg.Profiles[state.profile]; !ok {
			return fmt.Errorf("profile %q not found", state.profile)
		}
		profiles = []string{state.profile}
	}
	sort.Strings(profiles)

	// Build enablement matrix: pluginID -> (profileName -> bool) and the
	// installed-in matrix: pluginID -> (profileName -> bool). With ccpm,
	// each profile has its own <profileDir>/plugins/installed_plugins.json,
	// so a plugin can be present in profile A and absent in profile B.
	enabled := make(map[string]map[string]bool)
	installedIn := make(map[string]map[string]bool)
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

		installed, err := loadInstalledPlugins(p.Dir)
		if err != nil {
			return fmt.Errorf("reading installed plugins for %s: %w", name, err)
		}
		for _, ip := range installed {
			id := ip.id()
			if _, ok := installedIn[id]; !ok {
				installedIn[id] = make(map[string]bool)
			}
			installedIn[id][name] = true
		}
	}

	// Union of installed + any enabled plugin (covers cases where a plugin
	// is enabled in a profile but no longer present in installed_plugins.json).
	ids := make(map[string]bool)
	for id := range installedIn {
		ids[id] = true
	}
	for id := range enabled {
		ids[id] = true
	}

	if len(ids) == 0 {
		fmt.Println("No plugins installed. Run `/plugin install <name>` inside a `ccpm run <profile>` session.")
		return nil
	}

	sortedIDs := make([]string, 0, len(ids))
	for id := range ids {
		sortedIDs = append(sortedIDs, id)
	}
	sort.Strings(sortedIDs)

	bold := color.New(color.Bold).SprintFunc()
	fmt.Printf("  %-40s %-22s %s\n", bold("PLUGIN"), bold("INSTALLED IN"), bold("ENABLED IN"))
	fmt.Printf("  %s\n", strings.Repeat("─", 90))

	for _, id := range sortedIDs {
		var installedNames []string
		for _, name := range profiles {
			if installedIn[id][name] {
				installedNames = append(installedNames, name)
			}
		}
		installedCol := strings.Join(installedNames, ", ")
		if installedCol == "" {
			installedCol = "—"
		}

		var enabledIn []string
		for _, name := range profiles {
			if enabled[id][name] {
				enabledIn = append(enabledIn, name)
			}
		}
		enabledCol := strings.Join(enabledIn, ", ")
		if enabledCol == "" {
			enabledCol = "(none)"
		}
		fmt.Printf("  %-40s %-22s %s\n", id, installedCol, enabledCol)
	}
	return nil
}

// installedPlugin mirrors one entry in <profile>/plugins/installed_plugins.json.
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

// loadInstalledPlugins reads <profileDir>/plugins/installed_plugins.json. With
// CLAUDE_CONFIG_DIR pointing at the profile, Claude Code writes plugin state
// into the profile dir — the global ~/.claude path is empty for ccpm-managed
// installs. Missing file is not an error (returns empty slice).
//
// The file's schema has shifted across Claude Code releases, so this tries
// each known shape in order from newest to oldest. Today's shape (v2) is:
//
//	{"version": 2, "plugins": {"<name>@<marketplace>": [{"version": ...}]}}
func loadInstalledPlugins(profileDir string) ([]installedPlugin, error) {
	if profileDir == "" {
		return nil, fmt.Errorf("profileDir is required")
	}
	path := filepath.Join(profileDir, "plugins", "installed_plugins.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Shape v2 (current Claude Code): {version: int, plugins: map[id][]entry}.
	// Multiple entries per ID represent successive installs of the same plugin
	// at different versions; the most recent (by lastUpdated, then installedAt)
	// is the live one.
	var v2 struct {
		Version int                                 `json:"version"`
		Plugins map[string][]map[string]interface{} `json:"plugins"`
	}
	if err := json.Unmarshal(data, &v2); err == nil && v2.Version >= 2 && v2.Plugins != nil {
		out := make([]installedPlugin, 0, len(v2.Plugins))
		for id, entries := range v2.Plugins {
			if len(entries) == 0 {
				continue
			}
			pick := entries[len(entries)-1]
			pickStamp := entryTimestamp(pick)
			for _, e := range entries[:len(entries)-1] {
				if entryTimestamp(e).After(pickStamp) {
					pick = e
					pickStamp = entryTimestamp(e)
				}
			}
			name, marketplace := splitPluginID(id)
			version, _ := pick["version"].(string)
			out = append(out, installedPlugin{Name: name, Marketplace: marketplace, Version: version})
		}
		return out, nil
	}

	// Shape: top-level array of plugin objects.
	var arr []installedPlugin
	if err := json.Unmarshal(data, &arr); err == nil && len(arr) > 0 {
		return arr, nil
	}

	// Shape: object keyed by plugin ID with metadata values.
	var obj map[string]struct {
		Marketplace string `json:"marketplace"`
		Version     string `json:"version"`
	}
	if err := json.Unmarshal(data, &obj); err == nil && len(obj) > 0 {
		out := make([]installedPlugin, 0, len(obj))
		for id, meta := range obj {
			name, marketplace := splitPluginID(id)
			if meta.Marketplace != "" {
				marketplace = meta.Marketplace
			}
			out = append(out, installedPlugin{Name: name, Marketplace: marketplace, Version: meta.Version})
		}
		return out, nil
	}

	// Shape: object with an "installs" or "plugins" array.
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

func splitPluginID(id string) (name, marketplace string) {
	if i := strings.Index(id, "@"); i >= 0 {
		return id[:i], id[i+1:]
	}
	return id, ""
}

func entryTimestamp(e map[string]interface{}) time.Time {
	for _, key := range []string{"lastUpdated", "installedAt"} {
		if s, ok := e[key].(string); ok && s != "" {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

// ---- marketplace commands ----

func runMarketplaceAdd(state *pluginState, slug string) error {
	if !strings.Contains(slug, "/") {
		return fmt.Errorf("expected <org>/<repo>, got %q", slug)
	}
	if err := plugins.EnsureDirs(); err != nil {
		return err
	}
	name, err := plugins.RegisterMarketplace(plugins.AddMarketplaceOptions{
		Repo: slug,
		SSH:  state.ssh,
	})
	if err != nil {
		return err
	}
	color.New(color.FgGreen, color.Bold).Printf("✓ Registered marketplace %q (from %s)\n", name, slug)
	return nil
}

func runMarketplaceRemove(name string) error {
	// Refuse if any profile still has plugins from this marketplace —
	// removing the clone would orphan their installs.
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	for profileName, p := range cfg.Profiles {
		installed, err := loadInstalledPlugins(p.Dir)
		if err != nil {
			continue
		}
		for _, ip := range installed {
			if ip.Marketplace == name {
				return fmt.Errorf("marketplace %q still has plugins installed in profile %q (e.g. %q). Run `ccpm plugin remove %s --profile %s` first or use --global", name, profileName, ip.id(), ip.id(), profileName)
			}
		}
	}
	if err := plugins.RemoveMarketplace(name); err != nil {
		return err
	}
	color.New(color.FgGreen, color.Bold).Printf("✓ Removed marketplace %q\n", name)
	return nil
}

func runMarketplaceList() error {
	reg, err := plugins.LoadRegistry()
	if err != nil {
		return err
	}
	if len(reg.Marketplaces) == 0 {
		fmt.Println("No marketplaces registered. Add one with `ccpm plugin marketplace add <org>/<repo>`.")
		return nil
	}
	bold := color.New(color.Bold).SprintFunc()
	fmt.Printf("  %-30s %-30s %s\n", bold("MARKETPLACE"), bold("SOURCE"), bold("LAST UPDATED"))
	fmt.Printf("  %s\n", strings.Repeat("─", 80))
	for _, name := range reg.MarketplaceNames() {
		e := reg.Marketplaces[name]
		src := e.Source.Repo
		if src == "" {
			src = e.Source.URL
		}
		fmt.Printf("  %-30s %-30s %s\n", name, src, e.LastUpdated)
	}
	return nil
}

// ---- install / remove / gc ----

func splitPluginRef(ref string) (name, marketplace string, err error) {
	parts := strings.SplitN(ref, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("expected <plugin>@<marketplace>, got %q", ref)
	}
	return parts[0], parts[1], nil
}

func resolveTargetProfiles(state *pluginState, cfg *config.Config) ([]string, error) {
	if state.global && state.profile != "" {
		return nil, fmt.Errorf("--global and --profile are mutually exclusive")
	}
	if state.global {
		profiles := config.ProfileNames(cfg)
		sort.Strings(profiles)
		if len(profiles) == 0 {
			return nil, fmt.Errorf("no profiles configured")
		}
		return profiles, nil
	}
	if state.profile == "" {
		return nil, fmt.Errorf("either --global or --profile <name> is required")
	}
	if _, ok := cfg.Profiles[state.profile]; !ok {
		return nil, fmt.Errorf("profile %q not found", state.profile)
	}
	return []string{state.profile}, nil
}

func runPluginInstall(state *pluginState, ref string) error {
	pluginName, marketplace, err := splitPluginRef(ref)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	targets, err := resolveTargetProfiles(state, cfg)
	if err != nil {
		return err
	}

	mktDir, err := plugins.MarketplaceCloneDir(marketplace)
	if err != nil {
		return err
	}
	manifest, err := plugins.LoadMarketplaceManifest(mktDir)
	if err != nil {
		return fmt.Errorf("loading marketplace %q: %w", marketplace, err)
	}
	spec := manifest.FindPlugin(pluginName)
	if spec == nil {
		return fmt.Errorf("plugin %q not found in marketplace %q", pluginName, marketplace)
	}

	version, err := plugins.FetchPluginIntoCache(marketplace, *spec, state.ssh)
	if err != nil {
		return fmt.Errorf("fetching plugin: %w", err)
	}

	if err := share.EnsureDirs(); err != nil {
		return err
	}

	for _, profileName := range targets {
		p := cfg.Profiles[profileName]
		if err := plugins.LinkIntoProfile(p.Dir, marketplace, pluginName, version); err != nil {
			return fmt.Errorf("linking into profile %q: %w", profileName, err)
		}
		if !state.installOnly {
			if err := setEnabledPluginInFragment(profileName, ref, true); err != nil {
				return fmt.Errorf("enabling plugin in profile %q: %w", profileName, err)
			}
		}
		color.New(color.FgGreen, color.Bold).Printf("✓ Installed %s (v%s) into profile %q\n", ref, version, profileName)
	}
	if state.installOnly {
		color.New(color.Faint).Println("  (--install-only) plugin not enabled — turn on with `ccpm plugin enable`")
	}
	return nil
}

// setEnabledPluginInFragment writes enabledPlugins.<id>=enabled to the
// profile's settings fragment (the same file `ccpm settings set` writes to)
// and records the key as ccpm-owned so the next materialize keeps it.
func setEnabledPluginInFragment(profileName, pluginID string, enabled bool) error {
	fragPath, err := settingsFragmentPath(profileName)
	if err != nil {
		return err
	}
	frag, err := settingsmerge.LoadJSON(fragPath)
	if err != nil {
		return err
	}
	key := "enabledPlugins." + pluginID
	setNestedKey(frag, key, enabled)
	if err := settingsmerge.WriteJSON(fragPath, frag); err != nil {
		return err
	}
	return settingsmerge.MarkOwned(fragPath, key)
}

func runPluginRemove(state *pluginState, ref string) error {
	pluginName, marketplace, err := splitPluginRef(ref)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	targets, err := resolveTargetProfiles(state, cfg)
	if err != nil {
		return err
	}

	for _, profileName := range targets {
		p := cfg.Profiles[profileName]
		if err := plugins.UnlinkFromProfile(p.Dir, marketplace, pluginName); err != nil {
			return fmt.Errorf("unlinking from profile %q: %w", profileName, err)
		}
		if err := setEnabledPluginInFragment(profileName, ref, false); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not clear enabled flag for %s in profile %q: %v\n", ref, profileName, err)
		}
		color.New(color.FgGreen, color.Bold).Printf("✓ Removed %s from profile %q\n", ref, profileName)
	}
	return nil
}

// runPluginGC walks every profile to compute the set of cache references that
// are still in use, then deletes any shared-cache entry not in that set.
func runPluginGC() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	referenced := make(map[string]bool)
	for _, p := range cfg.Profiles {
		installed, err := loadInstalledPlugins(p.Dir)
		if err != nil {
			continue
		}
		for _, ip := range installed {
			if ip.Marketplace == "" || ip.Version == "" {
				continue
			}
			referenced[ip.Marketplace+"/"+ip.Name+"/"+ip.Version] = true
		}
	}
	removed, err := plugins.GarbageCollect(referenced)
	if err != nil {
		return err
	}
	if len(removed) == 0 {
		fmt.Println("Plugin cache: nothing to garbage-collect.")
		return nil
	}
	color.New(color.FgGreen, color.Bold).Printf("✓ Garbage-collected %d plugin cache entr%s\n", len(removed), pluralizeY(len(removed)))
	for _, r := range removed {
		fmt.Printf("  %s/%s/%s\n", r.Marketplace, r.Plugin, r.Version)
	}
	return nil
}

func pluralizeY(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
