package cmd

import (
	"fmt"
	"strconv"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Read or update ccpm configuration",
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a ccpm config value",
	Long: `Supported keys:
  check_default_drift   true|false — enable drift warnings on 'ccpm run' and 'ccpm use'
  cascade_auto_adopt    true|false — auto-link ~/.claude/<asset>/ entries into every
                                     profile at launch (default: true). Disable for
                                     strict reproducibility — only manifest-tracked
                                     assets will appear in profiles.`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigSet,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Print a ccpm config value",
	Long: `Supported keys:
  check_default_drift   bool — drift-warning setting
  cascade_auto_adopt    bool — host-asset auto-link setting
  default_dir           string — absolute path of the current default profile's
                        directory, or empty if no default is set. Used by
                        ccpm shell-init's claude() wrapper to set
                        CLAUDE_CONFIG_DIR for plain 'claude' invocations.`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigGet,
}

func init() {
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	switch key {
	case "check_default_drift":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("expected true/false, got %q", value)
		}
		cfg.Settings.CheckDefaultDrift = b
	case "cascade_auto_adopt":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("expected true/false, got %q", value)
		}
		cfg.Settings.CascadeAutoAdopt = &b
	default:
		return fmt.Errorf("unknown config key %q", key)
	}

	if err := config.Save(cfg); err != nil {
		return err
	}
	color.New(color.FgGreen, color.Bold).Printf("✓ Set %s = %s\n", key, value)
	return nil
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	switch key {
	case "check_default_drift":
		fmt.Println(cfg.Settings.CheckDefaultDrift)
	case "cascade_auto_adopt":
		fmt.Println(cfg.Settings.CascadeAutoAdoptEnabled())
	case "default_dir":
		// Print the default profile's directory, or empty when unset.
		// Designed for shell scripts (notably the claude() wrapper in
		// ccpm shell-init): always exit 0, no error noise on stderr,
		// so a missing default profile simply prints "" and the caller
		// falls through to invoking claude directly.
		if cfg.DefaultProfile == "" {
			return nil
		}
		if p, ok := cfg.Profiles[cfg.DefaultProfile]; ok {
			fmt.Println(p.Dir)
		}
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
}
