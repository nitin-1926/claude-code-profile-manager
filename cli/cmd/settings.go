package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/ccpm/internal/share"
)

var settingsProfile string

var settingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Manage Claude Code settings per profile",
	Long: `Manage settings per profile.

ccpm no longer keeps its own global settings layer. The cross-profile baseline
is ~/.claude/settings.json (the file native Claude Code already uses) — edit it
directly, or run ` + "`claude /config`" + ` natively, to change defaults for every profile.

Use ` + "`ccpm settings set --profile <name>`" + ` for profile-specific overrides.`,
}

var settingsSetCmd = &cobra.Command{
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
	RunE: runSettingsSet,
}

var settingsGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get the effective value of a setting",
	Args:  cobra.ExactArgs(1),
	RunE:  runSettingsGet,
}

var settingsApplyCmd = &cobra.Command{
	Use:   "apply <file.json>",
	Short: "Apply a JSON settings fragment to a profile",
	Args:  cobra.ExactArgs(1),
	RunE:  runSettingsApply,
}

var settingsShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the effective merged settings for a profile",
	RunE:  runSettingsShow,
}

func init() {
	settingsSetCmd.Flags().StringVar(&settingsProfile, "profile", "", "profile to modify (required)")
	_ = settingsSetCmd.MarkFlagRequired("profile")

	settingsGetCmd.Flags().StringVar(&settingsProfile, "profile", "", "profile to read from (required)")
	_ = settingsGetCmd.MarkFlagRequired("profile")

	settingsApplyCmd.Flags().StringVar(&settingsProfile, "profile", "", "profile to apply to (required)")
	_ = settingsApplyCmd.MarkFlagRequired("profile")

	settingsShowCmd.Flags().StringVar(&settingsProfile, "profile", "", "profile to show (required)")
	_ = settingsShowCmd.MarkFlagRequired("profile")

	settingsCmd.AddCommand(settingsSetCmd)
	settingsCmd.AddCommand(settingsGetCmd)
	settingsCmd.AddCommand(settingsApplyCmd)
	settingsCmd.AddCommand(settingsShowCmd)
	rootCmd.AddCommand(settingsCmd)
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

func ensureProfileExists(profileName string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if _, exists := cfg.Profiles[profileName]; !exists {
		return fmt.Errorf("profile %q not found", profileName)
	}
	return nil
}

func runSettingsSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	rawValue := args[1]

	if err := ensureProfileExists(settingsProfile); err != nil {
		return err
	}
	if err := share.EnsureDirs(); err != nil {
		return err
	}

	fragPath, err := settingsFragmentPath(settingsProfile)
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

	if err := settingsmerge.WriteJSON(fragPath, frag); err != nil {
		return fmt.Errorf("writing fragment: %w", err)
	}

	if err := settingsmerge.MarkOwned(fragPath, key); err != nil {
		return fmt.Errorf("recording owned key: %w", err)
	}

	color.New(color.FgGreen, color.Bold).Printf("✓ Set %s = %s (profile %q)\n", key, rawValue, settingsProfile)
	return nil
}

func runSettingsGet(cmd *cobra.Command, args []string) error {
	key := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	p, exists := cfg.Profiles[settingsProfile]
	if !exists {
		return fmt.Errorf("profile %q not found", settingsProfile)
	}

	merged, err := buildMergedSettings(p.Dir, settingsProfile)
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

func runSettingsApply(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	if err := ensureProfileExists(settingsProfile); err != nil {
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

	if err := share.EnsureDirs(); err != nil {
		return err
	}

	fragPath, err := settingsFragmentPath(settingsProfile)
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

	color.New(color.FgGreen, color.Bold).Printf("✓ Applied settings from %s (profile %q)\n", filePath, settingsProfile)
	return nil
}

func runSettingsShow(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	p, exists := cfg.Profiles[settingsProfile]
	if !exists {
		return fmt.Errorf("profile %q not found", settingsProfile)
	}

	merged, err := buildMergedSettings(p.Dir, settingsProfile)
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

// buildMergedSettings computes the merge result used by `settings get/show`.
// Must stay in sync with settingsmerge.Materialize's precedence: existing ←
// ~/.claude/settings.json ← profile fragment ← profile owned-keys.
func buildMergedSettings(profileDir, profileName string) (map[string]interface{}, error) {
	settingsDir, err := share.SettingsDir()
	if err != nil {
		return nil, err
	}

	profileFragPath := filepath.Join(settingsDir, profileName+".json")

	profileFrag, err := settingsmerge.LoadJSON(profileFragPath)
	if err != nil {
		return nil, err
	}

	existing, err := settingsmerge.LoadJSON(filepath.Join(profileDir, "settings.json"))
	if err != nil {
		return nil, err
	}

	hostSettings := readHostSettings()

	merged := settingsmerge.DeepMerge(existing, hostSettings)
	merged = settingsmerge.DeepMerge(merged, profileFrag)
	return merged, nil
}

// readHostSettings is a best-effort read of ~/.claude/settings.json. Errors
// are swallowed because this function is used for advisory commands (`get`,
// `show`); a broken host file shouldn't block the user from inspecting a
// profile. The authoritative read lives inside settingsmerge.Materialize.
func readHostSettings() map[string]interface{} {
	home, err := os.UserHomeDir()
	if err != nil {
		return map[string]interface{}{}
	}
	m, err := settingsmerge.LoadJSON(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		return map[string]interface{}{}
	}
	delete(m, "mcpServers")
	return m
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
