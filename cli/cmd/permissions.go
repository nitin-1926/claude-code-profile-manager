package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/ccpm/internal/share"
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
