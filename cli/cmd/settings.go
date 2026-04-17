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

var (
	settingsProfile string
	settingsGlobal  bool
)

var settingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Manage Claude Code settings across profiles",
}

var settingsSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a setting value (dot-notation key path)",
	Long: `Set a Claude Code setting by key path.

Examples:
  ccpm settings set permissions.allow '["Bash(git:*)"]' --global
  ccpm settings set model claude-sonnet-4-20250514 --profile work`,
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
	Short: "Apply a JSON settings fragment",
	Args:  cobra.ExactArgs(1),
	RunE:  runSettingsApply,
}

var settingsShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the effective merged settings for a profile",
	RunE:  runSettingsShow,
}

func init() {
	settingsSetCmd.Flags().BoolVar(&settingsGlobal, "global", false, "apply to global fragment (all profiles)")
	settingsSetCmd.Flags().StringVar(&settingsProfile, "profile", "", "apply to a specific profile fragment")
	settingsGetCmd.Flags().StringVar(&settingsProfile, "profile", "", "show value for a specific profile")
	settingsApplyCmd.Flags().BoolVar(&settingsGlobal, "global", false, "apply to global fragment")
	settingsApplyCmd.Flags().StringVar(&settingsProfile, "profile", "", "apply to a specific profile")
	settingsShowCmd.Flags().StringVar(&settingsProfile, "profile", "", "profile to show (required)")
	_ = settingsShowCmd.MarkFlagRequired("profile")

	settingsCmd.AddCommand(settingsSetCmd)
	settingsCmd.AddCommand(settingsGetCmd)
	settingsCmd.AddCommand(settingsApplyCmd)
	settingsCmd.AddCommand(settingsShowCmd)
	rootCmd.AddCommand(settingsCmd)
}

func settingsFragmentPath(profileName string) (string, error) {
	settingsDir, err := share.SettingsDir()
	if err != nil {
		return "", err
	}
	if profileName == "" {
		return filepath.Join(settingsDir, "global.json"), nil
	}
	return filepath.Join(settingsDir, profileName+".json"), nil
}

func runSettingsSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	rawValue := args[1]

	if !settingsGlobal && settingsProfile == "" {
		return fmt.Errorf("specify --global or --profile <name>")
	}

	if settingsProfile != "" {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		if _, exists := cfg.Profiles[settingsProfile]; !exists {
			return fmt.Errorf("profile %q not found", settingsProfile)
		}
	}

	if err := share.EnsureDirs(); err != nil {
		return err
	}

	targetProfile := ""
	if !settingsGlobal {
		targetProfile = settingsProfile
	}

	fragPath, err := settingsFragmentPath(targetProfile)
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

	scope := "global"
	if settingsProfile != "" {
		scope = fmt.Sprintf("profile %q", settingsProfile)
	}
	color.New(color.FgGreen, color.Bold).Printf("✓ Set %s = %s (%s)\n", key, rawValue, scope)
	return nil
}

func runSettingsGet(cmd *cobra.Command, args []string) error {
	key := args[0]

	if settingsProfile == "" {
		return fmt.Errorf("specify --profile <name>")
	}

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

	if !settingsGlobal && settingsProfile == "" {
		return fmt.Errorf("specify --global or --profile <name>")
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

	targetProfile := ""
	if !settingsGlobal {
		targetProfile = settingsProfile
	}

	fragPath, err := settingsFragmentPath(targetProfile)
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

	scope := "global"
	if settingsProfile != "" {
		scope = fmt.Sprintf("profile %q", settingsProfile)
	}
	color.New(color.FgGreen, color.Bold).Printf("✓ Applied settings from %s (%s)\n", filePath, scope)
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

func buildMergedSettings(profileDir, profileName string) (map[string]interface{}, error) {
	settingsDir, err := share.SettingsDir()
	if err != nil {
		return nil, err
	}

	global, err := settingsmerge.LoadJSON(filepath.Join(settingsDir, "global.json"))
	if err != nil {
		return nil, err
	}

	profileFrag, err := settingsmerge.LoadJSON(filepath.Join(settingsDir, profileName+".json"))
	if err != nil {
		return nil, err
	}

	existing, err := settingsmerge.LoadJSON(filepath.Join(profileDir, "settings.json"))
	if err != nil {
		return nil, err
	}

	merged := settingsmerge.DeepMerge(global, profileFrag)
	merged = settingsmerge.DeepMerge(merged, existing)
	return merged, nil
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
