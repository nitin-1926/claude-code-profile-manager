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
  check_default_drift   true|false — enable drift warnings on 'ccpm run' and 'ccpm use'`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigSet,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Print a ccpm config value",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigGet,
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
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
}
