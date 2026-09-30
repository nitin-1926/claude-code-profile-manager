package cmd

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/atomicwrite"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
)

// settingsState holds the cobra flag-bound values for the `ccpm settings`
// command tree.
type settingsState struct {
	profile string
}

// knownOutputStyles mirrors the values native Claude Code accepts for the
// outputStyle key (see Claude Code settings docs: "Build"/"Explanatory"/
// "Learning"/"Direct"). Kept as a soft allowlist so we warn on unknown
// values without blocking them — Claude Code adds styles over time.
var knownOutputStyles = []string{"default", "Build", "Explanatory", "Learning", "Direct"}

// dangerousSettingsKeys are top-level keys that, when supplied by a third
// party, could grant shell access or bypass safety rails. `ccpm settings
// apply` requires --i-know-what-this-does to write them so users don't
// paste-run a malicious fragment. The second group are Claude Code settings
// whose value is a command Claude Code executes (credential helpers, auth
// refresh, OTEL headers, file suggestion, status lines, process wrapper).
var dangerousSettingsKeys = []string{
	"permissions", "hooks", "env", "statusLine", "mcpServers", "enabledPlugins",
	"apiKeyHelper", "awsAuthRefresh", "awsCredentialExport", "gcpAuthRefresh",
	"otelHeadersHelper", "fileSuggestion", "subagentStatusLine", "processWrapper",
}

func newSettingsCmd() *cobra.Command {
	state := &settingsState{}

	root := &cobra.Command{
		Use:   "settings",
		Short: "Manage Claude Code settings per profile",
		Long: `Manage settings per profile.

ccpm no longer keeps its own global settings layer. The cross-profile baseline
is ~/.claude/settings.json (the file native Claude Code already uses) — edit it
directly, or run ` + "`claude /config`" + ` natively, to change defaults for every profile.

Use ` + "`ccpm settings set --profile <name>`" + ` for profile-specific overrides.`,
	}

	setCmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a profile-specific setting (dot-notation key path)",
		Long: `Set a Claude Code setting for one profile by key path.

Shared-across-all-profiles settings should go in ~/.claude/settings.json
directly; ccpm treats that file as the user/global baseline and merges it
into every profile at launch.

Examples:
  ccpm settings set model claude-sonnet-4-20250514 --profile work
  ccpm settings set permissions.allow '["Bash(git:*)"]' --profile work`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withConfigLock(func() error { return runSettingsSet(state, args) })
		},
	}
	requireProfileFlag(setCmd, &state.profile, "profile to modify (required)")

	getCmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Get the effective value of a setting",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSettingsGet(state, args)
		},
	}
	requireProfileFlag(getCmd, &state.profile, "profile to read from (required)")

	var applyAllowDangerous bool
	applyCmd := &cobra.Command{
		Use:   "apply <file.json>",
		Short: "Apply a JSON settings fragment to a profile",
		Long: `Apply a JSON settings fragment to a profile's ccpm layer.

The fragment is deep-merged into the profile's ccpm fragment, so existing
keys are preserved unless overridden. Dangerous top-level keys —
permissions, hooks, env, statusLine, mcpServers, enabledPlugins, and the
command-running apiKeyHelper, awsAuthRefresh, awsCredentialExport,
gcpAuthRefresh, otelHeadersHelper, fileSuggestion, subagentStatusLine,
processWrapper — are rejected by default; pass --i-know-what-this-does to
override, which acknowledges that the JSON grants shell access or can bypass
safety rails.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withConfigLock(func() error { return runSettingsApply(state, args, applyAllowDangerous) })
		},
	}
	requireProfileFlag(applyCmd, &state.profile, "profile to apply to (required)")
	applyCmd.Flags().BoolVar(&applyAllowDangerous, "i-know-what-this-does", false, "allow the patch to touch security-sensitive keys (permissions, hooks, env, command helpers — see --help)")

	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show the effective merged settings for a profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSettingsShow(state)
		},
	}
	requireProfileFlag(showCmd, &state.profile, "profile to show (required)")

	statusLineCmd := &cobra.Command{
		Use:   "statusline [command]",
		Short: "Set or clear the statusLine command for a profile",
		Long: `Configure the Claude Code statusLine shell command.

Pass a command to set it; pass an empty string to remove the statusLine key.
ccpm writes the native shape:
  { "statusLine": { "type": "command", "command": "<cmd>" } }

Note: 'ccpm run' auto-injects a default statusLine ('ccpm statusline', which
shows the active profile + usage/limits) into profiles that have none. Setting
your own here takes precedence; clearing it with "" both removes a custom one
and an auto-injected default. Disable the auto-injection entirely with
'ccpm config set statusline false'.

Examples:
  ccpm settings statusline "~/.claude/statusline.sh" --profile work
  ccpm settings statusline "" --profile work       # remove`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withConfigLock(func() error { return runSettingsStatusLine(state, args) })
		},
	}
	requireProfileFlag(statusLineCmd, &state.profile, "profile to modify (required)")

	outputStyleCmd := &cobra.Command{
		Use:   "outputstyle <style>",
		Short: "Set the outputStyle key for a profile",
		Long: `Set the Claude Code output style (shape of the assistant's responses).

Known values: ` + strings.Join(knownOutputStyles, ", ") + `. Unknown values are
allowed with a warning so ccpm doesn't block newer styles native claude adds.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withConfigLock(func() error { return runSettingsOutputStyle(state, cmd, args) })
		},
	}
	requireProfileFlag(outputStyleCmd, &state.profile, "profile to modify (required)")

	root.AddCommand(setCmd, getCmd, applyCmd, showCmd, statusLineCmd, outputStyleCmd)
	return root
}

func init() {
	rootCmd.AddCommand(newSettingsCmd())
}

func runSettingsStatusLine(state *settingsState, args []string) error {
	command := args[0]

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

	green := color.New(color.FgGreen, color.Bold)
	if strings.TrimSpace(command) == "" {
		delete(frag, "statusLine")
		if err := saveFragment(fragPath, frag); err != nil {
			return err
		}
		green.Printf("✓ statusLine cleared (profile %q)\n", state.profile)
		return nil
	}

	frag["statusLine"] = map[string]interface{}{
		"type":    "command",
		"command": command,
	}
	if err := saveFragment(fragPath, frag, "statusLine"); err != nil {
		return err
	}
	green.Printf("✓ statusLine = %q (profile %q)\n", command, state.profile)
	return nil
}

func runSettingsOutputStyle(state *settingsState, cmd *cobra.Command, args []string) error {
	style := args[0]
	if !stringSliceContains(knownOutputStyles, style) {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %q is not a known output style; writing anyway.\n", style)
	}

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
	frag["outputStyle"] = style
	if err := saveFragment(fragPath, frag, "outputStyle"); err != nil {
		return err
	}
	color.New(color.FgGreen, color.Bold).Printf("✓ outputStyle = %q (profile %q)\n", style, state.profile)
	return nil
}

// settingsFragmentPath returns the profile-scoped fragment path. Global
// fragments are no longer supported — shared settings live in
// ~/.claude/settings.json instead.
func settingsFragmentPath(profileName string) (string, error) {
	settingsDir, err := share.SettingsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(settingsDir, profileName+".json"), nil
}

// saveFragment writes a profile fragment and marks ownedKeys in its
// owned-keys sidecar as ONE atomicwrite transaction (AGENTS.md invariant 9):
// two separate writes let a crash leave a value the merge would not
// re-assert over settings.json. Callers hold the config lock around the whole
// load→mutate→save so parallel edits can't drop each other's keys.
func saveFragment(fragPath string, frag map[string]interface{}, ownedKeys ...string) error {
	data, err := json.MarshalIndent(frag, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling fragment: %w", err)
	}
	changes := []atomicwrite.FileChange{atomicwrite.WriteFile(fragPath, append(data, '\n'), config.FilePerm)}
	if len(ownedKeys) > 0 {
		owned, err := settingsmerge.LoadOwnedKeys(fragPath)
		if err != nil {
			return fmt.Errorf("recording owned key: %w", err)
		}
		for _, k := range ownedKeys {
			owned[k] = struct{}{}
		}
		sidecar, err := json.MarshalIndent(settingsmerge.OwnedKeysFile{Keys: slices.Sorted(maps.Keys(owned))}, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling owned keys: %w", err)
		}
		// Same "<fragment>.owned.json" naming settingsmerge reads back; the
		// round-trip is covered by the fragment concurrency tests.
		ownedPath := strings.TrimSuffix(fragPath, ".json") + ".owned.json"
		changes = append(changes, atomicwrite.WriteFile(ownedPath, append(sidecar, '\n'), config.FilePerm))
	}
	if err := atomicwrite.Apply(changes); err != nil {
		return fmt.Errorf("writing fragment: %w", err)
	}
	return nil
}

func ensureProfileExists(profileName string) error {
	_, _, err := loadProfile(profileName)
	return err
}

func runSettingsSet(state *settingsState, args []string) error {
	key := args[0]
	rawValue := args[1]

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

	var value interface{}
	if err := json.Unmarshal([]byte(rawValue), &value); err != nil {
		value = rawValue
	}

	setNestedKey(frag, key, value)

	if err := saveFragment(fragPath, frag, key); err != nil {
		return err
	}

	color.New(color.FgGreen, color.Bold).Printf("✓ Set %s = %s (profile %q)\n", key, rawValue, state.profile)
	return nil
}

func runSettingsGet(state *settingsState, args []string) error {
	key := args[0]

	_, p, err := loadProfile(state.profile)
	if err != nil {
		return err
	}

	merged, err := buildMergedSettings(p.Dir, state.profile)
	if err != nil {
		return err
	}

	val := getNestedKey(merged, key)
	if val == nil {
		fmt.Printf("%s: <not set>\n", key)
		return nil
	}

	out, err := json.MarshalIndent(val, "", "  ")
	if err != nil {
		return err
	}
	fmt.Printf("%s: %s\n", key, string(out))
	return nil
}

func runSettingsApply(state *settingsState, args []string, allowDangerous bool) error {
	filePath := args[0]

	if err := ensureProfileExists(state.profile); err != nil {
		return err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filePath, err)
	}

	var patch map[string]interface{}
	if err := json.Unmarshal(data, &patch); err != nil {
		return fmt.Errorf("parsing %s: %w", filePath, err)
	}

	if triggered := dangerousKeysIn(patch); len(triggered) > 0 && !allowDangerous {
		return fmt.Errorf("patch touches security-sensitive keys %v — re-run with --i-know-what-this-does if that is intended. These keys can grant shell access or bypass permission checks", triggered)
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

	merged := settingsmerge.DeepMerge(frag, patch)
	if err := settingsmerge.WriteJSON(fragPath, merged); err != nil {
		return fmt.Errorf("writing fragment: %w", err)
	}

	if err := settingsmerge.MarkOwnedFromPatch(fragPath, patch); err != nil {
		return fmt.Errorf("recording owned keys: %w", err)
	}

	color.New(color.FgGreen, color.Bold).Printf("✓ Applied settings from %s (profile %q)\n", filePath, state.profile)
	return nil
}

func runSettingsShow(state *settingsState) error {
	_, p, err := loadProfile(state.profile)
	if err != nil {
		return err
	}

	merged, err := buildMergedSettings(p.Dir, state.profile)
	if err != nil {
		return err
	}

	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

// buildMergedSettings returns the merge result used by advisory commands
// (`settings get/show`, `hooks list`, `plugin list`). Delegates to
// settingsmerge.ComputeMerged so every caller sees the exact layer stack
// Claude Code will see — including owned-keys re-assertion, project settings
// discovered from the CWD, and enterprise managed settings.
func buildMergedSettings(profileDir, profileName string) (map[string]interface{}, error) {
	projectRoot := ""
	if cwd, err := os.Getwd(); err == nil {
		projectRoot = settingsmerge.FindProjectRoot(cwd)
	}
	return settingsmerge.ComputeMerged(profileDir, profileName, projectRoot)
}

// dangerousKeysIn returns the subset of top-level keys in patch that appear
// in dangerousSettingsKeys.
func dangerousKeysIn(patch map[string]interface{}) []string {
	var hit []string
	for _, k := range dangerousSettingsKeys {
		if _, ok := patch[k]; ok {
			hit = append(hit, k)
		}
	}
	return hit
}

func setNestedKey(m map[string]interface{}, key string, value interface{}) {
	parts := strings.Split(key, ".")
	current := m
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}
		if next, ok := current[part].(map[string]interface{}); ok {
			current = next
		} else {
			next := make(map[string]interface{})
			current[part] = next
			current = next
		}
	}
}

func getNestedKey(m map[string]interface{}, key string) interface{} {
	parts := strings.Split(key, ".")
	var current interface{} = m
	for _, part := range parts {
		obj, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current = obj[part]
	}
	return current
}
