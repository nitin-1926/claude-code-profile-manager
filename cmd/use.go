package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/shell"
)

var useCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Set profile for current shell session",
	Long:  "Outputs shell export statements to activate a profile in the current shell.\nRequires the shell hook: eval \"$(ccpm shell-init)\"",
	Args:  cobra.ExactArgs(1),
	RunE:  runUse,
}

func init() {
	rootCmd.AddCommand(useCmd)
}

func runUse(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	p, exists := cfg.Profiles[name]
	if !exists {
		return fmt.Errorf("profile %q not found", name)
	}

	// Update last used
	cfg.UpdateLastUsed(name)
	_ = config.Save(cfg)

	// Output export statements (evaluated by shell hook)
	s := shell.DetectShell()
	fmt.Print(shell.ExportStatements(s, p.Name, p.Dir))
	return nil
}
