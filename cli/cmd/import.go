package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/defaultclaude"
	"github.com/nitin-1926/ccpm/internal/filetree"
	"github.com/nitin-1926/ccpm/internal/picker"
	"github.com/nitin-1926/ccpm/internal/settingsmerge"
)

// importDefaultState and importFromProfileState encapsulate the flag-bound
// values for each subcommand. Separated so the two subcommands don't share
// --only / --force / --profile values by accident.
type importDefaultState struct {
	profile         string
	all             bool
	dryRun          bool
	only            []string
	force           bool
	noShare         bool
	liveSymlinks    bool
	noLiveSymlink   bool
	selectAll       bool
	mcpScope        string
	confirmHooks    bool
	confirmMCP      bool
	includeRunnable bool
}

type importFromProfileState struct {
	src    string
	target string
	only   []string
	force  bool
}

func newImportCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "import",
		Short: "Import assets from external sources into profiles",
	}

	defaults := &importDefaultState{}
	defaultCmd := &cobra.Command{
		Use:   "default",
		Short: "Import skills, hooks, MCP servers, and settings from ~/.claude into profiles",
		Long: `Copy or merge selected subtrees from the default Claude Code config
directory (~/.claude) into one or all ccpm profiles.

By default, imports inert assets only (skills, commands, rules, agents,
settings). Hooks and MCP servers — both of which can execute shell commands
or spawn processes — are excluded unless --include-runnable is set, because
a compromised ~/.claude hook/MCP would silently apply to every profile.

When hooks or MCP imports are requested, ccpm previews each item (command
body for hooks, command+args+env or url for MCP) and requires explicit
confirmation per item. In non-TTY contexts ccpm refuses to import hooks or
MCP unless --yes-hooks / --yes-mcp is passed, so piping 'yes' or running
under CI can't auto-grant shell access.

Settings are deep-merged into the profile's settings.json via the same merge
engine used by 'ccpm run'; directory targets are copied file-by-file,
preserving existing profile files unless --force is passed; MCP servers are
written into the appropriate ccpm MCP fragment and materialized into
settings.json#mcpServers on 'ccpm use' / 'ccpm run'.

Examples:
  ccpm import default --profile work                # inert assets only
  ccpm import default --profile work --include-runnable  # prompts for each hook/MCP
  ccpm import default --all --dry-run
  ccpm import default --profile personal --only skills,agents
  ccpm import default --all --only settings --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImportDefault(defaults)
		},
	}
	defaultCmd.Flags().StringVar(&defaults.profile, "profile", "", "target profile name")
	defaultCmd.Flags().BoolVar(&defaults.all, "all", false, "import into every profile")
	defaultCmd.Flags().BoolVar(&defaults.dryRun, "dry-run", false, "preview without writing")
	defaultCmd.Flags().StringSliceVar(&defaults.only, "only", nil, "comma-separated targets (skills, commands, rules, hooks, agents, settings, mcp, plugins)")
	defaultCmd.Flags().BoolVar(&defaults.force, "force", false, "overwrite existing files in profiles")
	defaultCmd.Flags().BoolVar(&defaults.noShare, "no-share", false, "copy assets directly into the profile instead of symlinking from ~/.ccpm/share")
	defaultCmd.Flags().BoolVar(&defaults.liveSymlinks, "live-symlinks", false, "for deduped skills/agents/commands, keep symlink-to-dir entries as symlinks in the share store (live updates from source)")
	defaultCmd.Flags().BoolVar(&defaults.noLiveSymlink, "no-live-symlinks", false, "always snapshot (disable the interactive symlink prompt)")
	defaultCmd.Flags().BoolVar(&defaults.selectAll, "select-all", false, "skip per-item prompts and import every entry under the selected targets")
	defaultCmd.Flags().StringVar(&defaults.mcpScope, "mcp-scope", "", "where imported MCP servers live: global (all profiles) or profile (selected profile only)")
	defaultCmd.Flags().BoolVar(&defaults.includeRunnable, "include-runnable", false, "also import hooks and MCP servers (subject to per-item confirmation)")
	defaultCmd.Flags().BoolVar(&defaults.confirmHooks, "yes-hooks", false, "non-TTY: confirm import of every discovered hook without prompting (use only for trusted ~/.claude state)")
	defaultCmd.Flags().BoolVar(&defaults.confirmMCP, "yes-mcp", false, "non-TTY: confirm import of every discovered MCP server without prompting (use only for trusted ~/.claude state)")

	fromProfile := &importFromProfileState{}
	fromProfileCmd := &cobra.Command{
		Use:   "from-profile",
		Short: "Copy assets from one ccpm profile into another",
		Long: `Clone skills, commands, rules, hooks, agents, and (optionally) settings
from a source ccpm profile into a target profile. Useful when spinning up a
new profile that should start with the same loadout as an existing one.

Examples:
  ccpm import from-profile --src work --profile personal
  ccpm import from-profile --src work --profile playground --only skills
  ccpm import from-profile --src work --profile playground --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImportFromProfile(fromProfile)
		},
	}
	fromProfileCmd.Flags().StringVar(&fromProfile.src, "src", "", "source profile to copy from (required)")
	fromProfileCmd.Flags().StringVar(&fromProfile.target, "profile", "", "target profile to copy into (required)")
	fromProfileCmd.Flags().StringSliceVar(&fromProfile.only, "only", nil, "comma-separated targets (skills, commands, rules, hooks, agents, settings)")
	fromProfileCmd.Flags().BoolVar(&fromProfile.force, "force", false, "overwrite existing files in target profile")

	root.AddCommand(defaultCmd, fromProfileCmd)
	return root
}

func init() {
	rootCmd.AddCommand(newImportCmd())
}

func runImportDefault(state *importDefaultState) error {
	if state.profile != "" && state.all {
		return fmt.Errorf("use either --profile or --all, not both")
	}
	if state.liveSymlinks && state.noShare {
		return fmt.Errorf("--live-symlinks only applies with deduped import; omit --no-share")
	}
	if state.liveSymlinks && state.noLiveSymlink {
		return fmt.Errorf("--live-symlinks and --no-live-symlinks are mutually exclusive")
	}
	if state.mcpScope != "" && state.mcpScope != defaultclaude.MCPImportScopeGlobal && state.mcpScope != defaultclaude.MCPImportScopeProfile {
		return fmt.Errorf("--mcp-scope must be %q or %q", defaultclaude.MCPImportScopeGlobal, defaultclaude.MCPImportScopeProfile)
	}

	if !defaultclaude.Exists() {
		src, _ := defaultclaude.DefaultDir()
		return fmt.Errorf("no default Claude config found at %s", src)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if state.profile == "" && !state.all {
		if err := pickImportTarget(state, cfg); err != nil {
			return err
		}
	}

	targets, err := defaultclaude.ParseTargets(state.only)
	if err != nil {
		return err
	}
	if len(state.only) == 0 {
		picked, err := pickImportTargets(state.includeRunnable)
		if err == nil {
			targets = picked
		} else if !errors.Is(err, picker.ErrNonInteractive) {
			return err
		}
	}

	// Fail closed on runnable imports unless the user opted in.
	if !state.includeRunnable {
		filtered := make([]defaultclaude.Target, 0, len(targets))
		dropped := []defaultclaude.Target{}
		for _, t := range targets {
			if t == defaultclaude.TargetHooks || t == defaultclaude.TargetMCP {
				dropped = append(dropped, t)
				continue
			}
			filtered = append(filtered, t)
		}
		if len(dropped) > 0 {
			fmt.Fprintf(os.Stderr, "Skipping %v (shell/process-executing). Re-run with --include-runnable to import these after previewing each item.\n", dropped)
		}
		targets = filtered
	}

	// Prompt once for live-symlink strategy if the user didn't pass either flag.
	if !state.liveSymlinks && !state.noLiveSymlink && !state.noShare {
		if has, _ := anyLiveSymlinkCandidate(targets); has {
			choice, err := picker.Select(
				"Some skills/agents/commands in ~/.claude are symlinked. How should ccpm install them?",
				[]picker.Option{
					{Value: "symlink", Label: "Keep as symlinks (recommended)", Description: "live updates from the source repo"},
					{Value: "copy", Label: "Snapshot (copy)", Description: "take a copy now; future source edits stay local"},
				},
			)
			if err == nil {
				state.liveSymlinks = choice == "symlink"
			} else if !errors.Is(err, picker.ErrNonInteractive) {
				return err
			}
		}
	}

	// Per-item selection.
	itemFilter := map[defaultclaude.Target]map[string]bool{}
	for _, t := range targets {
		switch t {
		case defaultclaude.TargetHooks:
			picked, err := pickHooksWithPreview(state)
			if err != nil {
				return err
			}
			itemFilter[t] = picked
		case defaultclaude.TargetMCP:
			picked, err := pickMCPWithPreview(state)
			if err != nil {
				return err
			}
			itemFilter[t] = picked
		default:
			if state.selectAll {
				continue
			}
			picked, err := pickItemsForTarget(t)
			if err != nil {
				if errors.Is(err, picker.ErrNonInteractive) {
					continue
				}
				return err
			}
			if picked != nil {
				itemFilter[t] = picked
			}
		}
	}

	// Resolve MCP scope. Only relevant when MCP is being imported.
	mcpScope := state.mcpScope
	if containsTarget(targets, defaultclaude.TargetMCP) && mcpScope == "" {
		mcpSelected, hasFilter := itemFilter[defaultclaude.TargetMCP]
		shouldPrompt := hasFilter && len(mcpSelected) > 0
		if !hasFilter {
			if entries, _ := defaultclaude.LoadMCPEntries(); len(entries) > 0 {
				shouldPrompt = true
			}
		}
		if shouldPrompt {
			choice, err := picker.Select(
				"Where should imported MCP servers live?",
				[]picker.Option{
					{Value: defaultclaude.MCPImportScopeGlobal, Label: "Global", Description: "available in every ccpm profile"},
					{Value: defaultclaude.MCPImportScopeProfile, Label: "Selected profile only", Description: "scoped to the target profile"},
				},
			)
			if err == nil {
				mcpScope = choice
			} else if errors.Is(err, picker.ErrNonInteractive) {
				mcpScope = defaultclaude.MCPImportScopeGlobal
			} else {
				return err
			}
		}
	}

	var names []string
	if state.all {
		names = config.ProfileNames(cfg)
		if len(names) == 0 {
			return fmt.Errorf("no profiles found — create one with 'ccpm add'")
		}
	} else {
		if _, ok := cfg.Profiles[state.profile]; !ok {
			return fmt.Errorf("profile %q not found", state.profile)
		}
		names = []string{state.profile}
	}

	green := color.New(color.FgGreen, color.Bold)
	cyan := color.New(color.FgCyan)
	dim := color.New(color.Faint)

	for idx, name := range names {
		p := cfg.Profiles[name]

		perProfileTargets := targets
		if mcpScope == defaultclaude.MCPImportScopeGlobal && idx > 0 {
			perProfileTargets = filterOutTarget(targets, defaultclaude.TargetMCP)
		}

		plan, err := defaultclaude.Import(p.Dir, defaultclaude.ImportOptions{
			Targets:      perProfileTargets,
			DryRun:       state.dryRun,
			Force:        state.force,
			Dedupe:       !state.noShare,
			ProfileName:  name,
			LiveSymlinks: state.liveSymlinks && !state.noShare,
			ItemFilter:   itemFilter,
			MCPScope:     mcpScope,
		})
		if err != nil {
			return fmt.Errorf("import into %q: %w", name, err)
		}

		header := fmt.Sprintf("Profile %q", name)
		if state.dryRun {
			header += " (dry-run)"
		}
		cyan.Println(header)

		for _, action := range plan.Actions {
			switch action.Kind {
			case "skip-missing":
				dim.Printf("  - %-9s skipped (%s)\n", action.Target, fallback(action.Note, "source missing"))
			case "merge-settings":
				cyan.Printf("  ~ %-9s merge into settings.json\n", action.Target)
			case "copy":
				green.Printf("  + %-9s %s%s\n", action.Target, relOrAbs(action.TargetPath), noteSuffix(action.Note))
			case "link":
				green.Printf("  → %-9s %s (shared)%s\n", action.Target, relOrAbs(action.TargetPath), noteSuffix(action.Note))
			case "mcp-add":
				green.Printf("  + %-9s %s\n", action.Target, action.Note)
			default:
				fmt.Printf("  · %-9s %s\n", action.Target, action.Kind)
			}
		}

		if !state.dryRun {
			if err := mergeImportedSettings(p.Dir); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: settings merge failed: %v\n", err)
			}

			if err := settingsmerge.Materialize(p.Dir, name, ""); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: re-materializing settings: %v\n", err)
			}
			if err := settingsmerge.MaterializeMCP(p.Dir, name, ""); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: re-materializing MCP: %v\n", err)
			}
		}
	}

	if !state.dryRun {
		snap, err := defaultclaude.Snapshot(defaultclaude.DefaultTargets())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not snapshot ~/.claude: %v\n", err)
		} else if err := defaultclaude.SaveFingerprint(snap); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not save fingerprint: %v\n", err)
		} else {
			dim.Printf("\nFingerprint updated (%d files tracked).\n", len(snap.Files))
		}
	}

	return nil
}

// mergeImportedSettings consumes the staged settings.json.ccpm-import produced
// by Import and deep-merges it into the profile's settings.json.
func mergeImportedSettings(profileDir string) error {
	staged := filepath.Join(profileDir, "settings.json.ccpm-import")
	if _, err := os.Stat(staged); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	patch, err := settingsmerge.LoadJSON(staged)
	if err != nil {
		return err
	}
	target := filepath.Join(profileDir, "settings.json")
	existing, err := settingsmerge.LoadJSON(target)
	if err != nil {
		return err
	}
	merged := settingsmerge.DeepMerge(existing, patch)
	if err := settingsmerge.WriteJSON(target, merged); err != nil {
		return err
	}
	return os.Remove(staged)
}

func runImportFromProfile(state *importFromProfileState) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	names := config.ProfileNames(cfg)

	if state.src == "" {
		if len(names) == 0 {
			return fmt.Errorf("no profiles exist yet — create one with `ccpm add <name>`")
		}
		opts := make([]picker.Option, len(names))
		for i, n := range names {
			opts[i] = picker.Option{Value: n, Label: n}
		}
		choice, err := picker.Select("Which profile should we copy from?", opts)
		if err != nil {
			if errors.Is(err, picker.ErrNonInteractive) {
				return fmt.Errorf("--src <name> is required")
			}
			return err
		}
		state.src = choice
	}
	if state.target == "" {
		remaining := make([]string, 0, len(names))
		for _, n := range names {
			if n != state.src {
				remaining = append(remaining, n)
			}
		}
		if len(remaining) == 0 {
			return fmt.Errorf("no other profiles to copy into — create one with `ccpm add <name>`")
		}
		opts := make([]picker.Option, len(remaining))
		for i, n := range remaining {
			opts[i] = picker.Option{Value: n, Label: n}
		}
		choice, err := picker.Select("Which profile should we copy into?", opts)
		if err != nil {
			if errors.Is(err, picker.ErrNonInteractive) {
				return fmt.Errorf("--profile <name> is required")
			}
			return err
		}
		state.target = choice
	}
	if state.src == state.target {
		return fmt.Errorf("source and target profiles must differ")
	}

	src, ok := cfg.Profiles[state.src]
	if !ok {
		return fmt.Errorf("source profile %q not found", state.src)
	}
	dst, ok := cfg.Profiles[state.target]
	if !ok {
		return fmt.Errorf("target profile %q not found", state.target)
	}

	targets, err := defaultclaude.ParseTargets(state.only)
	if err != nil {
		return err
	}
	if len(state.only) == 0 {
		picked, err := pickImportTargets(true)
		if err == nil {
			targets = picked
		} else if !errors.Is(err, picker.ErrNonInteractive) {
			return err
		}
	}

	if err := importFromProfile(src.Dir, dst.Dir, targets, state.force); err != nil {
		return err
	}

	if err := settingsmerge.Materialize(dst.Dir, state.target, ""); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: re-materializing settings: %v\n", err)
	}
	if err := settingsmerge.MaterializeMCP(dst.Dir, state.target, ""); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: re-materializing MCP: %v\n", err)
	}

	color.New(color.FgGreen, color.Bold).Printf("✓ Imported assets from %q into %q\n", state.src, state.target)
	return nil
}

// importFromProfile copies selected targets from srcProfileDir into
// dstProfileDir.
func importFromProfile(srcProfileDir, dstProfileDir string, targets []defaultclaude.Target, force bool) error {
	for _, t := range targets {
		srcPath := profileTargetPath(srcProfileDir, t)
		dstPath := profileTargetPath(dstProfileDir, t)

		info, err := os.Stat(srcPath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("stat %s: %w", srcPath, err)
		}

		if t == defaultclaude.TargetSettings {
			if err := mergeProfileSettings(srcPath, dstPath); err != nil {
				return fmt.Errorf("merging settings from %s: %w", srcProfileDir, err)
			}
			continue
		}

		if info.IsDir() {
			if err := copyProfileTree(srcPath, dstPath, force); err != nil {
				return fmt.Errorf("copying %s: %w", srcPath, err)
			}
			continue
		}
	}
	return nil
}

func profileTargetPath(root string, t defaultclaude.Target) string {
	if t == defaultclaude.TargetSettings {
		return filepath.Join(root, "settings.json")
	}
	return filepath.Join(root, string(t))
}

func mergeProfileSettings(src, dst string) error {
	srcData, err := settingsmerge.LoadJSON(src)
	if err != nil {
		return err
	}
	dstData, err := settingsmerge.LoadJSON(dst)
	if err != nil {
		return err
	}
	merged := settingsmerge.DeepMerge(srcData, dstData)
	return settingsmerge.WriteJSON(dst, merged)
}

func copyProfileTree(src, dst string, force bool) error {
	return filetree.CopyTree(src, dst, !force)
}

func pickImportTarget(state *importDefaultState, cfg *config.Config) error {
	names := config.ProfileNames(cfg)
	if len(names) == 0 {
		return fmt.Errorf("no profiles found — create one with `ccpm add <name>`")
	}
	opts := []picker.Option{{Value: "__all__", Label: "All profiles", Description: "import into every profile"}}
	for _, n := range names {
		opts = append(opts, picker.Option{Value: n, Label: n})
	}
	choice, err := picker.Select("Which profile should we import into?", opts)
	if err != nil {
		if errors.Is(err, picker.ErrNonInteractive) {
			return fmt.Errorf("specify --profile <name> or --all")
		}
		return err
	}
	if choice == "__all__" {
		state.all = true
	} else {
		state.profile = choice
	}
	return nil
}

// pickImportTargets offers a multi-select over the known target subtrees.
// When includeRunnable=false, hooks and mcp are hidden from the default list
// so hitting enter doesn't silently import shell-executing assets.
func pickImportTargets(includeRunnable bool) ([]defaultclaude.Target, error) {
	all := []defaultclaude.Target{
		defaultclaude.TargetSkills,
		defaultclaude.TargetCommands,
		defaultclaude.TargetRules,
		defaultclaude.TargetAgents,
		defaultclaude.TargetSettings,
		defaultclaude.TargetPlugins,
	}
	if includeRunnable {
		all = append(all, defaultclaude.TargetHooks, defaultclaude.TargetMCP)
	}
	defaults := map[defaultclaude.Target]bool{}
	for _, t := range defaultclaude.DefaultTargets() {
		defaults[t] = true
	}
	opts := make([]picker.Option, len(all))
	defs := []string{}
	for i, t := range all {
		opts[i] = picker.Option{Value: string(t), Label: string(t)}
		if defaults[t] && (includeRunnable || (t != defaultclaude.TargetHooks && t != defaultclaude.TargetMCP)) {
			defs = append(defs, string(t))
		}
	}
	values, err := picker.MultiSelect("Which targets should we import?", opts, defs)
	if err != nil {
		return nil, err
	}
	out := make([]defaultclaude.Target, 0, len(values))
	for _, v := range values {
		out = append(out, defaultclaude.Target(v))
	}
	if len(out) == 0 {
		safe := make([]defaultclaude.Target, 0)
		for _, t := range defaultclaude.DefaultTargets() {
			if !includeRunnable && (t == defaultclaude.TargetHooks || t == defaultclaude.TargetMCP) {
				continue
			}
			safe = append(safe, t)
		}
		return safe, nil
	}
	return out, nil
}

// pickHooksWithPreview lists every hook entry in ~/.claude/settings.json with
// its command body and requires explicit confirmation per hook. In non-TTY
// contexts: returns an empty set unless --yes-hooks was passed (fail closed).
func pickHooksWithPreview(state *importDefaultState) (map[string]bool, error) {
	entries, err := defaultclaude.LoadHookEntries()
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return map[string]bool{}, nil
	}

	if !picker.IsInteractive() {
		if !state.confirmHooks {
			fmt.Fprintln(os.Stderr, "Skipping hook import: non-TTY and --yes-hooks not passed. Re-run with --yes-hooks only if ~/.claude is trusted.")
			return map[string]bool{}, nil
		}
		fmt.Fprintln(os.Stderr, "--yes-hooks: importing every discovered hook without prompting.")
		out := make(map[string]bool, len(entries))
		for _, e := range entries {
			out[e.ID()] = true
		}
		return out, nil
	}

	opts := make([]picker.Option, len(entries))
	defs := []string{}
	for i, e := range entries {
		opts[i] = picker.Option{
			Value:       e.ID(),
			Label:       e.Event + " · " + e.Matcher,
			Description: e.Command,
		}
	}
	chosen, err := picker.MultiSelect(
		"Which hooks should we import? Each runs a shell command — review carefully.",
		opts, defs,
	)
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool, len(chosen))
	for _, id := range chosen {
		m[id] = true
	}
	return m, nil
}

// pickMCPWithPreview lists every MCP server entry in ~/.claude.json with its
// command/url/env and requires explicit confirmation per server. Non-TTY
// behaviour mirrors pickHooksWithPreview.
func pickMCPWithPreview(state *importDefaultState) (map[string]bool, error) {
	entries, err := defaultclaude.LoadMCPEntries()
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return map[string]bool{}, nil
	}

	if !picker.IsInteractive() {
		if !state.confirmMCP {
			fmt.Fprintln(os.Stderr, "Skipping MCP import: non-TTY and --yes-mcp not passed. Re-run with --yes-mcp only if ~/.claude is trusted.")
			return map[string]bool{}, nil
		}
		fmt.Fprintln(os.Stderr, "--yes-mcp: importing every discovered MCP server without prompting.")
		out := make(map[string]bool, len(entries))
		for _, e := range entries {
			out[e.ID()] = true
		}
		return out, nil
	}

	opts := make([]picker.Option, len(entries))
	defs := []string{}
	for i, e := range entries {
		opts[i] = picker.Option{
			Value:       e.ID(),
			Label:       e.Name,
			Description: mcpPreviewDescription(e),
		}
	}
	chosen, err := picker.MultiSelect(
		"Which MCP servers should we import? Each can spawn a process or open a remote connection — review carefully.",
		opts, defs,
	)
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool, len(chosen))
	for _, id := range chosen {
		m[id] = true
	}
	return m, nil
}

func mcpPreviewDescription(e defaultclaude.MCPEntry) string {
	parts := []string{e.Source()}
	def := e.Definition
	if cmd, ok := def["command"].(string); ok && cmd != "" {
		parts = append(parts, "cmd="+cmd)
	}
	if rawArgs, ok := def["args"].([]interface{}); ok && len(rawArgs) > 0 {
		args := make([]string, 0, len(rawArgs))
		for _, a := range rawArgs {
			if s, ok := a.(string); ok {
				args = append(args, s)
			}
		}
		parts = append(parts, "args="+strings.Join(args, " "))
	}
	if url, ok := def["url"].(string); ok && url != "" {
		parts = append(parts, "url="+url)
	}
	if env, ok := def["env"].(map[string]interface{}); ok && len(env) > 0 {
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		parts = append(parts, "env="+strings.Join(keys, ","))
	}
	return strings.Join(parts, " · ")
}

// anyLiveSymlinkCandidate reports whether any top-level entry under the given
// dedupable targets in ~/.claude is itself a symlink to a directory.
func anyLiveSymlinkCandidate(targets []defaultclaude.Target) (bool, error) {
	root, err := defaultclaude.DefaultDir()
	if err != nil {
		return false, err
	}
	dedupable := map[defaultclaude.Target]bool{
		defaultclaude.TargetSkills:   true,
		defaultclaude.TargetAgents:   true,
		defaultclaude.TargetCommands: true,
	}
	for _, t := range targets {
		if !dedupable[t] {
			continue
		}
		dir := filepath.Join(root, string(t))
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			isLink, _ := filetree.SymlinkToDirectory(filepath.Join(dir, e.Name()))
			if isLink {
				return true, nil
			}
		}
	}
	return false, nil
}

func relOrAbs(p string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
			return "~/" + rel
		}
	}
	return p
}

// granularTargets is the set of targets that expose per-item selection.
func granularTargets() map[defaultclaude.Target]bool {
	return map[defaultclaude.Target]bool{
		defaultclaude.TargetSkills:   true,
		defaultclaude.TargetCommands: true,
		defaultclaude.TargetRules:    true,
		defaultclaude.TargetHooks:    true,
		defaultclaude.TargetAgents:   true,
		defaultclaude.TargetMCP:      true,
	}
}

// pickItemsForTarget offers a multi-select listing the top-level entries of a
// target. Used for inert asset kinds; hooks and MCP use their dedicated
// preview functions instead.
func pickItemsForTarget(t defaultclaude.Target) (map[string]bool, error) {
	if !granularTargets()[t] {
		return nil, nil
	}
	items, err := listTargetItems(t)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	opts := make([]picker.Option, len(items))
	defs := make([]string, len(items))
	for i, it := range items {
		opts[i] = picker.Option{Value: it.id, Label: it.label, Description: it.description}
		defs[i] = it.id
	}
	title := fmt.Sprintf("Which %s should we import? (all selected by default)", t)
	chosen, err := picker.MultiSelect(title, opts, defs)
	if err != nil {
		return nil, err
	}
	if len(chosen) == len(items) {
		return nil, nil
	}
	m := make(map[string]bool, len(chosen))
	for _, id := range chosen {
		m[id] = true
	}
	return m, nil
}

type targetItem struct {
	id          string
	label       string
	description string
}

func listTargetItems(t defaultclaude.Target) ([]targetItem, error) {
	if t == defaultclaude.TargetMCP {
		entries, err := defaultclaude.LoadMCPEntries()
		if err != nil {
			return nil, err
		}
		out := make([]targetItem, 0, len(entries))
		for _, e := range entries {
			out = append(out, targetItem{
				id:          e.ID(),
				label:       e.Name,
				description: e.Source(),
			})
		}
		return out, nil
	}
	root, err := defaultclaude.DefaultDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, string(t))
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]targetItem, 0, len(entries))
	for _, e := range entries {
		out = append(out, targetItem{
			id:    e.Name(),
			label: e.Name(),
		})
	}
	return out, nil
}

func containsTarget(targets []defaultclaude.Target, t defaultclaude.Target) bool {
	for _, x := range targets {
		if x == t {
			return true
		}
	}
	return false
}

func filterOutTarget(targets []defaultclaude.Target, drop defaultclaude.Target) []defaultclaude.Target {
	out := make([]defaultclaude.Target, 0, len(targets))
	for _, t := range targets {
		if t == drop {
			continue
		}
		out = append(out, t)
	}
	return out
}

func fallback(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func noteSuffix(note string) string {
	if note == "" {
		return ""
	}
	return " (" + note + ")"
}
