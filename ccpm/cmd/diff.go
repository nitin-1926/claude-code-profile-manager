package cmd

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/manifest"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
)

var diffCmd = &cobra.Command{
	Use:   "diff <profile-a> <profile-b>",
	Short: "Compare two profiles' assets, settings, MCP servers, and plugins",
	Long: `Show what differs between two profiles: managed assets (skills, agents,
commands, rules, hooks), settings-fragment keys, env var names (values are
never printed), MCP servers, and installed plugins.

Useful before consolidating profiles or when one profile behaves differently
from another and you want to know why.`,
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: completeProfileNames,
	RunE:              runDiff,
}

func init() {
	rootCmd.AddCommand(diffCmd)
}

func runDiff(cmd *cobra.Command, args []string) error {
	nameA, nameB := args[0], args[1]
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	pa, ok := cfg.Profiles[nameA]
	if !ok {
		return fmt.Errorf("profile %q not found", nameA)
	}
	pb, ok := cfg.Profiles[nameB]
	if !ok {
		return fmt.Errorf("profile %q not found", nameB)
	}

	bold := color.New(color.Bold)
	dim := color.New(color.Faint)
	identical := true

	if pa.AuthMethod != pb.AuthMethod {
		bold.Println("Auth")
		fmt.Printf("  %s: %s | %s: %s\n", nameA, pa.AuthMethod, nameB, pb.AuthMethod)
		fmt.Println()
		identical = false
	}

	// Managed assets from the manifest (entries listing each profile, plus
	// global/host entries which apply to both and therefore never differ).
	m, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}
	assetsA, assetsB := map[string]bool{}, map[string]bool{}
	for _, inst := range m.Installs {
		if inst.Scope != manifest.ScopeProfile {
			continue
		}
		key := fmt.Sprintf("%s %q", inst.Kind, inst.ID)
		for _, p := range inst.Profiles {
			if p == nameA {
				assetsA[key] = true
			}
			if p == nameB {
				assetsB[key] = true
			}
		}
	}
	identical = printSetDiff("Profile-scoped assets", nameA, nameB, assetsA, assetsB) && identical

	// Settings fragments: top-level keys + env var names (never values).
	fragKeys := func(name string) (map[string]bool, map[string]bool) {
		keys, envs := map[string]bool{}, map[string]bool{}
		dir, err := share.SettingsDir()
		if err != nil {
			return keys, envs
		}
		doc, err := settingsmerge.LoadJSON(filepath.Join(dir, name+".json"))
		if err != nil {
			return keys, envs
		}
		for k := range doc {
			keys[k] = true
		}
		if env, ok := doc["env"].(map[string]interface{}); ok {
			for k := range env {
				envs[k] = true
			}
		}
		return keys, envs
	}
	keysA, envA := fragKeys(nameA)
	keysB, envB := fragKeys(nameB)
	identical = printSetDiff("Settings fragment keys", nameA, nameB, keysA, keysB) && identical
	identical = printSetDiff("Env var names (values not shown)", nameA, nameB, envA, envB) && identical

	// MCP fragments: server names per profile fragment.
	mcpNames := func(name string) map[string]bool {
		out := map[string]bool{}
		dir, err := share.MCPDir()
		if err != nil {
			return out
		}
		doc, err := settingsmerge.LoadJSON(filepath.Join(dir, name+".json"))
		if err != nil {
			return out
		}
		for k := range doc {
			out[k] = true
		}
		return out
	}
	identical = printSetDiff("MCP servers (profile fragment)", nameA, nameB, mcpNames(nameA), mcpNames(nameB)) && identical

	// Installed plugins per profile.
	pluginIDs := func(dir string) map[string]bool {
		out := map[string]bool{}
		installed, err := loadInstalledPlugins(dir)
		if err != nil {
			return out
		}
		for _, ip := range installed {
			out[ip.id()] = true
		}
		return out
	}
	identical = printSetDiff("Installed plugins", nameA, nameB, pluginIDs(pa.Dir), pluginIDs(pb.Dir)) && identical

	if identical {
		color.New(color.FgGreen, color.Bold).Printf("✓ Profiles %q and %q have no differences in tracked assets, settings keys, env names, MCP servers, or plugins.\n", nameA, nameB)
		dim.Println("  (Credential contents and untracked files inside the profile dirs are not compared.)")
	}
	return nil
}

// printSetDiff renders the only-in-A / only-in-B lines for one category.
// Returns true when the sets are identical (nothing printed).
func printSetDiff(title, nameA, nameB string, a, b map[string]bool) bool {
	var onlyA, onlyB []string
	for k := range a {
		if !b[k] {
			onlyA = append(onlyA, k)
		}
	}
	for k := range b {
		if !a[k] {
			onlyB = append(onlyB, k)
		}
	}
	if len(onlyA) == 0 && len(onlyB) == 0 {
		return true
	}
	sort.Strings(onlyA)
	sort.Strings(onlyB)
	color.New(color.Bold).Println(title)
	if len(onlyA) > 0 {
		fmt.Printf("  only in %s: %s\n", nameA, strings.Join(onlyA, ", "))
	}
	if len(onlyB) > 0 {
		fmt.Printf("  only in %s: %s\n", nameB, strings.Join(onlyB, ", "))
	}
	fmt.Println()
	return false
}
