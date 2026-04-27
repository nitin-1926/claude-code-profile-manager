package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
)

// hooksState holds the cobra flag-bound variables for the `ccpm hooks`
// command tree. Scoped per invocation so tests (and any future library use
// that calls rootCmd.Execute more than once) don't see stale flag values.
type hooksState struct {
	profile string
	matcher string
	index   int
}

// Recognized hook event names. Claude Code may add more over time; unknown
// names are still accepted at runtime (a warning is printed) so ccpm doesn't
// block a valid config that predates the CLI being updated.
var knownHookEvents = []string{
	"PreToolUse",
	"PostToolUse",
	"UserPromptSubmit",
	"SessionStart",
	"SessionEnd",
	"Notification",
	"Stop",
	"SubagentStop",
	"PreCompact",
}

func newHooksCmd() *cobra.Command {
	state := &hooksState{}

	root := &cobra.Command{
		Use:   "hooks",
		Short: "Manage Claude Code hooks in profile settings",
		Long: `Manage the hooks key in a profile's Claude Code settings.

Hooks are shell commands Claude Code runs on lifecycle events like PreToolUse,
SessionStart, or UserPromptSubmit. Each entry has an optional matcher (regex or
literal tool name, empty matches all) and a command to run.

ccpm writes entries into ~/.ccpm/share/settings/<profile>.json under the "hooks"
key; materialization at ccpm run merges them into the profile's settings.json.
The hook script directory ~/.claude/hooks/ is managed separately via
ccpm import default --only hooks or ccpm skill-style deduplication.`,
	}

	addCmd := &cobra.Command{
		Use:   "add <event> <command>",
		Short: "Append a hook to an event for a profile",
		Long: `Append a hook to an event.

Event is one of: PreToolUse, PostToolUse, UserPromptSubmit, SessionStart,
SessionEnd, Notification, Stop, SubagentStop, PreCompact.

Use --matcher to restrict the hook to a specific tool name pattern (empty
matches every tool).

Examples:
  ccpm hooks add PreToolUse "echo firing" --profile work
  ccpm hooks add PostToolUse "lint-check" --matcher "Edit|Write" --profile work`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHooksAdd(state, cmd, args)
		},
	}
	addCmd.Flags().StringVar(&state.profile, "profile", "", "target profile (required)")
	addCmd.Flags().StringVar(&state.matcher, "matcher", "", "tool-name matcher (regex or literal)")
	_ = addCmd.MarkFlagRequired("profile")

	removeCmd := &cobra.Command{
		Use:     "remove <event>",
		Short:   "Remove a hook entry from an event for a profile",
		Aliases: []string{"rm"},
		Long: `Remove a hook entry from an event.

By default the last-added entry for the event is removed. Pass --index to target
a specific position (0-based), matching the numbering shown in ccpm hooks list.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHooksRemove(state, args)
		},
	}
	removeCmd.Flags().StringVar(&state.profile, "profile", "", "target profile (required)")
	removeCmd.Flags().IntVar(&state.index, "index", -1, "0-based index of the entry to remove (default: last)")
	_ = removeCmd.MarkFlagRequired("profile")

	listCmd := &cobra.Command{
		Use:     "list",
		Short:   "List hooks for a profile (merged view)",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHooksList(state)
		},
	}
	listCmd.Flags().StringVar(&state.profile, "profile", "", "profile to list (required)")
	_ = listCmd.MarkFlagRequired("profile")

	root.AddCommand(addCmd)
	root.AddCommand(removeCmd)
	root.AddCommand(listCmd)
	return root
}

func init() {
	rootCmd.AddCommand(newHooksCmd())
}

func runHooksAdd(state *hooksState, cmd *cobra.Command, args []string) error {
	event := args[0]
	command := args[1]

	if !isKnownHookEvent(event) {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %q is not a known hook event; writing anyway.\n", event)
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

	hooksRoot := ensureHooksRoot(frag)
	events, _ := hooksRoot[event].([]interface{})

	entry := map[string]interface{}{
		"matcher": state.matcher,
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": command,
			},
		},
	}
	events = append(events, entry)
	hooksRoot[event] = events

	if err := settingsmerge.WriteJSON(fragPath, frag); err != nil {
		return fmt.Errorf("writing fragment: %w", err)
	}
	if err := settingsmerge.MarkOwned(fragPath, "hooks."+event); err != nil {
		return fmt.Errorf("recording owned key: %w", err)
	}

	color.New(color.FgGreen, color.Bold).Printf("✓ Hook added to %s for profile %q\n", event, state.profile)
	return nil
}

func runHooksRemove(state *hooksState, args []string) error {
	event := args[0]

	if err := ensureProfileExists(state.profile); err != nil {
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

	hooksRoot, _ := frag["hooks"].(map[string]interface{})
	events, _ := hooksRoot[event].([]interface{})
	if len(events) == 0 {
		return fmt.Errorf("no hooks set for event %q on profile %q", event, state.profile)
	}

	idx := state.index
	if idx < 0 {
		idx = len(events) - 1
	}
	if idx >= len(events) {
		return fmt.Errorf("index %d out of range (profile has %d entries for %s)", idx, len(events), event)
	}

	events = append(events[:idx], events[idx+1:]...)
	if len(events) == 0 {
		delete(hooksRoot, event)
	} else {
		hooksRoot[event] = events
	}
	if len(hooksRoot) == 0 {
		delete(frag, "hooks")
	}

	if err := settingsmerge.WriteJSON(fragPath, frag); err != nil {
		return fmt.Errorf("writing fragment: %w", err)
	}

	color.New(color.FgGreen, color.Bold).Printf("✓ Hook removed from %s (index %d) for profile %q\n", event, idx, state.profile)
	return nil
}

func runHooksList(state *hooksState) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	p, exists := cfg.Profiles[state.profile]
	if !exists {
		return fmt.Errorf("profile %q not found", state.profile)
	}

	merged, err := buildMergedSettings(p.Dir, state.profile)
	if err != nil {
		return err
	}

	// Layer in project-level hooks from the nearest .claude/settings.json so
	// users see everything Claude Code will actually run in the profile.
	if cwd, werr := os.Getwd(); werr == nil {
		if root := settingsmerge.FindProjectRoot(cwd); root != "" {
			if projectSettings, _, perr := settingsmerge.LoadProjectSettings(root); perr == nil {
				merged = settingsmerge.DeepMerge(merged, projectSettings)
			}
		}
	}

	hooksRoot, _ := merged["hooks"].(map[string]interface{})
	if len(hooksRoot) == 0 {
		fmt.Printf("No hooks set for profile %q (check ~/.claude/settings.json for global hooks).\n", state.profile)
		return nil
	}

	events := make([]string, 0, len(hooksRoot))
	for k := range hooksRoot {
		events = append(events, k)
	}
	sort.Strings(events)

	bold := color.New(color.Bold).SprintFunc()
	for _, event := range events {
		entries, _ := hooksRoot[event].([]interface{})
		fmt.Printf("%s\n", bold(event))
		for i, raw := range entries {
			entry, _ := raw.(map[string]interface{})
			matcher, _ := entry["matcher"].(string)
			cmds := describeHookCommands(entry)
			fmt.Printf("  [%d] matcher=%q  %s\n", i, matcher, cmds)
		}
	}
	return nil
}

func ensureHooksRoot(frag map[string]interface{}) map[string]interface{} {
	existing, _ := frag["hooks"].(map[string]interface{})
	if existing == nil {
		existing = map[string]interface{}{}
		frag["hooks"] = existing
	}
	return existing
}

func describeHookCommands(entry map[string]interface{}) string {
	list, _ := entry["hooks"].([]interface{})
	parts := make([]string, 0, len(list))
	for _, raw := range list {
		cmd, _ := raw.(map[string]interface{})
		t, _ := cmd["type"].(string)
		c, _ := cmd["command"].(string)
		if t == "" {
			t = "command"
		}
		parts = append(parts, fmt.Sprintf("%s=%q", t, c))
	}
	if len(parts) == 0 {
		return "(no commands)"
	}
	return strings.Join(parts, ", ")
}

func isKnownHookEvent(event string) bool {
	for _, e := range knownHookEvents {
		if e == event {
			return true
		}
	}
	return false
}
