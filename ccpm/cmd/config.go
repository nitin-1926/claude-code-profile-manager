package cmd

import (
	"fmt"
	"strconv"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/picker"
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
                                     assets will appear in profiles.
  statusline            true|false — auto-inject a default statusLine (` + "`ccpm statusline`" + `)
                                     into launched profiles that have none, so the TUI
                                     shows the active profile + usage/limit windows
                                     (default: true). Never overwrites your own
                                     statusLine. Enabling it for the first time offers
                                     the segment picker; reach it any time with
                                     ` + "`ccpm statusline configure`" + `.
  usage_tracking        true|false — inject a SessionEnd hook (` + "`ccpm usage sync`" + `) into
                                     launched profiles so the per-profile token
                                     usage store stays warm (default: false).
                                     ` + "`ccpm usage`" + ` works without it (lazy catch-up).`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigSet,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Print a ccpm config value",
	Long: `Supported keys:
  check_default_drift   bool — drift-warning setting
  cascade_auto_adopt    bool — host-asset auto-link setting
  statusline            bool — default-statusLine auto-injection setting (which
                        segments it shows is set by ` + "`ccpm statusline configure`" + `)
  usage_tracking        bool — SessionEnd usage-sync hook injection setting
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

	// Set by the statusline case below when this is the first time the status
	// line has been enabled and no segment layout exists yet.
	offerSegments := false

	// Load, change and save under the config lock, which AGENTS.md makes
	// mandatory for every config.json mutation: without it a concurrent
	// `ccpm add` between this Load and Save is silently erased. The picker
	// offered afterwards stays OUTSIDE the lock — it waits on a human, and
	// applyStatusLineLayout takes the lock again for its own write.
	var cfg *config.Config
	err := withConfigLock(func() error {
		var err error
		if cfg, err = config.Load(); err != nil {
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
		case "statusline":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("expected true/false, got %q", value)
			}
			cfg.Settings.DefaultStatusLine = &b
			// Turning the status line on for the first time is the one moment the
			// user is definitely thinking about it, so offer the segment picker
			// then rather than hoping they discover `ccpm statusline configure`.
			// Deferred until after the save below so the enable lands even if they
			// abandon the picker.
			offerSegments = b && cfg.Settings.StatusLine == nil
		case "usage_tracking":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("expected true/false, got %q", value)
			}
			cfg.Settings.UsageTracking = &b
		default:
			return fmt.Errorf("unknown config key %q", key)
		}

		return config.Save(cfg)
	})
	if err != nil {
		return err
	}
	color.New(color.FgGreen, color.Bold).Printf("✓ Set %s = %s\n", key, value)

	if offerSegments {
		if !picker.IsInteractive() {
			color.New(color.Faint).Println("  Choose which segments it shows with `ccpm statusline configure`.")
			return nil
		}
		fmt.Println()
		// A failure here must not fail `config set` — the setting is already
		// saved, and the picker is an offer, not part of the operation.
		// Abandoning it writes no layout, so the offer simply returns next time.
		if err := promptStatusLineLayout(cfg, ""); err != nil {
			color.New(color.Faint).Printf("  Skipped segment setup (%v) — run `ccpm statusline configure` when you want it.\n", err)
		}
	}
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
	case "statusline":
		fmt.Println(cfg.Settings.StatusLineEnabled())
	case "usage_tracking":
		fmt.Println(cfg.Settings.UsageTrackingEnabled())
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
