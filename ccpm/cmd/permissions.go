package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
)

// validPermissionModes mirrors the native Claude Code enum so `ccpm
// permissions mode` rejects typos instead of silently writing garbage.
// Source: Claude Code settings docs (permissions.defaultMode).
var validPermissionModes = []string{
	"default",
	"acceptEdits",
	"plan",
	"auto",
	"dontAsk",
	"bypassPermissions",
}

// permissionBucket is one of the three arrays under `permissions` that
// Claude Code understands — allow, ask, deny. Remove is a fourth verb at the
// CLI layer that strips a pattern from all three.
type permissionBucket string

const (
	permAllow  permissionBucket = "allow"
	permAsk    permissionBucket = "ask"
	permDeny   permissionBucket = "deny"
	permRemove permissionBucket = ""
)

// permState holds the cobra flag-bound variables for the `ccpm permissions`
// command tree. Scoped per invocation.
type permState struct {
	profile string
	global  bool
}

func newPermissionsCmd() *cobra.Command {
	state := &permState{}

	root := &cobra.Command{
		Use:   "permissions",
		Short: "Manage Claude Code permission rules per profile",
		Long: `Manage the permissions.{allow,ask,deny,defaultMode} settings Claude Code
reads at launch. Writes land in the profile fragment
(~/.ccpm/share/settings/<profile>.json) or, with --global, directly in
~/.claude/settings.json so every profile and every native-claude session sees
them.

Rules follow the same pattern syntax native claude uses — e.g. Bash(git:*),
Edit(**/*.go), Read(**/secrets/**). ccpm doesn't try to validate the pattern
shape, only that the string is non-empty.`,
	}

	makeBucketCmd := func(use, short string, bucket permissionBucket) *cobra.Command {
		c := &cobra.Command{
			Use:   use,
			Short: short,
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return runPermsAdd(state, bucket, args[0])
			},
		}
		c.Flags().StringVar(&state.profile, "profile", "", "target profile")
		c.Flags().BoolVar(&state.global, "global", false, "write to ~/.claude/settings.json instead of a profile fragment")
		return c
	}

	allowCmd := makeBucketCmd("allow <rule>", "Add a rule to permissions.allow", permAllow)
	askCmd := makeBucketCmd("ask <rule>", "Add a rule to permissions.ask", permAsk)
	denyCmd := makeBucketCmd("deny <rule>", "Add a rule to permissions.deny", permDeny)
	removeCmd := makeBucketCmd("remove <rule>", "Remove a rule from permissions.{allow,ask,deny}", permRemove)
	removeCmd.Aliases = []string{"rm"}

	listCmd := &cobra.Command{
		Use:     "list",
		Short:   "List permission rules and default mode",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPermsList(state)
		},
	}
	listCmd.Flags().StringVar(&state.profile, "profile", "", "target profile")
	listCmd.Flags().BoolVar(&state.global, "global", false, "read from ~/.claude/settings.json instead of a profile fragment")

	modeCmd := &cobra.Command{
		Use:   "mode <" + strings.Join(validPermissionModes, "|") + ">",
		Short: "Set permissions.defaultMode (permission behavior at launch)",
		Long: `Set the permission mode Claude Code starts in.

  default            — prompt for each permission (native default)
  acceptEdits        — auto-approve file edits, prompt for other tools
  plan               — propose changes via a plan, require approval
  auto               — auto-approve safe actions, soft-deny risky ones
  dontAsk            — approve everything (use with caution)
  bypassPermissions  — skip all checks (highest risk; can be blocked by the
                       managed-settings key permissions.disableBypassPermissionsMode)`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPermsMode(state, args)
		},
	}
	modeCmd.Flags().StringVar(&state.profile, "profile", "", "target profile")
	modeCmd.Flags().BoolVar(&state.global, "global", false, "write to ~/.claude/settings.json instead of a profile fragment")

	root.AddCommand(allowCmd, askCmd, denyCmd, removeCmd, listCmd, modeCmd)
	return root
}

func init() {
	rootCmd.AddCommand(newPermissionsCmd())
}

func runPermsAdd(state *permState, bucket permissionBucket, rule string) error {
	if strings.TrimSpace(rule) == "" {
		return fmt.Errorf("rule must be a non-empty string")
	}
	if err := requirePermScope(state); err != nil {
		return err
	}

	path, err := permsSettingsPath(state)
	if err != nil {
		return err
	}
	doc, err := settingsmerge.LoadJSON(path)
	if err != nil {
		return fmt.Errorf("loading %s: %w", path, err)
	}
	permsRoot := ensurePermsRoot(doc)

	// When removing, strip from every bucket; otherwise add to the named
	// bucket and remove from the others so the three lists stay disjoint.
	buckets := []permissionBucket{permAllow, permAsk, permDeny}
	if bucket == permRemove {
		for _, b := range buckets {
			permsRoot[string(b)] = filterRule(listBucket(permsRoot, b), rule)
		}
	} else {
		for _, b := range buckets {
			list := filterRule(listBucket(permsRoot, b), rule)
			if b == bucket {
				list = append(list, rule)
			}
			permsRoot[string(b)] = list
		}
	}
	pruneEmptyBuckets(permsRoot)
	if len(permsRoot) == 0 {
		delete(doc, "permissions")
	} else {
		doc["permissions"] = permsRoot
	}

	if err := settingsmerge.WriteJSON(path, doc); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if !state.global {
		// Owned-keys tracking only applies to the ccpm profile fragment;
		// the host ~/.claude/settings.json uses its own (native) semantics.
		for _, b := range buckets {
			_ = settingsmerge.MarkOwned(path, "permissions."+string(b))
		}
	}

	label := "removed from all"
	if bucket != permRemove {
		label = "added to " + string(bucket)
	}
	color.New(color.FgGreen, color.Bold).Printf("✓ Rule %q %s (%s)\n", rule, label, permsScopeDescription(state))
	return nil
}

func runPermsMode(state *permState, args []string) error {
	mode := args[0]
	if !stringSliceContains(validPermissionModes, mode) {
		return fmt.Errorf("invalid mode %q; expected one of: %s", mode, strings.Join(validPermissionModes, ", "))
	}
	if err := requirePermScope(state); err != nil {
		return err
	}

	path, err := permsSettingsPath(state)
	if err != nil {
		return err
	}
	doc, err := settingsmerge.LoadJSON(path)
	if err != nil {
		return fmt.Errorf("loading %s: %w", path, err)
	}
	root := ensurePermsRoot(doc)
	root["defaultMode"] = mode
	doc["permissions"] = root

	if err := settingsmerge.WriteJSON(path, doc); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if !state.global {
		_ = settingsmerge.MarkOwned(path, "permissions.defaultMode")
	}

	color.New(color.FgGreen, color.Bold).Printf("✓ permissions.defaultMode = %q (%s)\n", mode, permsScopeDescription(state))
	return nil
}

func runPermsList(state *permState) error {
	if err := requirePermScope(state); err != nil {
		return err
	}

	path, err := permsSettingsPath(state)
	if err != nil {
		return err
	}
	doc, err := settingsmerge.LoadJSON(path)
	if err != nil {
		return fmt.Errorf("loading %s: %w", path, err)
	}
	root, _ := doc["permissions"].(map[string]interface{})
	if len(root) == 0 {
		fmt.Printf("No permission rules set (%s).\n", permsScopeDescription(state))
		return nil
	}

	bold := color.New(color.Bold).SprintFunc()
	if mode, ok := root["defaultMode"].(string); ok {
		fmt.Printf("%s: %s\n", bold("defaultMode"), mode)
	}
	for _, b := range []permissionBucket{permAllow, permAsk, permDeny} {
		list := listBucket(root, b)
		sort.Strings(list)
		fmt.Printf("%s (%d):\n", bold(string(b)), len(list))
		for _, rule := range list {
			fmt.Printf("  - %s\n", rule)
		}
	}
	return nil
}

// requirePermScope enforces that exactly one of --profile / --global is set.
// We don't default to a picker here because permission changes are
// security-sensitive — an ambiguous scope should fail loud, not prompt.
func requirePermScope(state *permState) error {
	if state.global && state.profile != "" {
		return fmt.Errorf("--global and --profile are mutually exclusive")
	}
	if !state.global && state.profile == "" {
		return fmt.Errorf("specify either --profile <name> or --global")
	}
	if state.profile != "" {
		return ensureProfileExists(state.profile)
	}
	return nil
}

// permsSettingsPath returns the file to read/write based on scope.
// Global writes go to ~/.claude/settings.json directly; profile writes go to
// the per-profile fragment under ~/.ccpm/share/settings/.
func permsSettingsPath(state *permState) (string, error) {
	if state.global {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".claude", "settings.json"), nil
	}
	if err := share.EnsureDirs(); err != nil {
		return "", err
	}
	return settingsFragmentPath(state.profile)
}

func permsScopeDescription(state *permState) string {
	if state.global {
		return "~/.claude/settings.json"
	}
	return "profile " + state.profile
}

func ensurePermsRoot(doc map[string]interface{}) map[string]interface{} {
	existing, _ := doc["permissions"].(map[string]interface{})
	if existing == nil {
		existing = map[string]interface{}{}
	}
	return existing
}

// listBucket returns the current []string for a bucket, tolerating either
// []string or []interface{} on disk (the second is what encoding/json
// produces for untyped maps). Non-string elements are dropped.
func listBucket(root map[string]interface{}, bucket permissionBucket) []string {
	raw, ok := root[string(bucket)]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	default:
		return nil
	}
}

func filterRule(list []string, rule string) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		if s != rule {
			out = append(out, s)
		}
	}
	return out
}

// pruneEmptyBuckets removes empty allow/ask/deny arrays from the permissions
// root so we don't persist noise. Handles both []string (what runPermsAdd
// writes) and []interface{} (what the JSON decoder produces on reload), so
// future callers don't silently leave empty arrays behind.
func pruneEmptyBuckets(root map[string]interface{}) {
	for _, b := range []string{"allow", "ask", "deny"} {
		raw, present := root[b]
		if !present {
			continue
		}
		switch list := raw.(type) {
		case []string:
			if len(list) == 0 {
				delete(root, b)
			}
		case []interface{}:
			if len(list) == 0 {
				delete(root, b)
			}
		}
	}
}
