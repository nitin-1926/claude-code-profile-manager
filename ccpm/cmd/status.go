package cmd

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	claudepkg "github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/claude"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/credentials"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/keystore"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show system overview",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	bold := color.New(color.Bold)
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)

	bold.Println("ccpm status")
	fmt.Printf("  Version:    %s\n", version)

	baseDir, _ := config.BaseDir()
	fmt.Printf("  Config dir: %s\n", baseDir)

	// Claude binary
	bin, err := claudepkg.FindBinary()
	if err != nil {
		red.Printf("  Claude:     not found (%v)\n", err)
	} else {
		green.Printf("  Claude:     %s\n", bin)
	}

	// Active shell profile
	activeProfile := os.Getenv("CCPM_ACTIVE_PROFILE")
	if activeProfile != "" {
		fmt.Printf("  Active:     %s (shell)\n", activeProfile)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if cfg.DefaultProfile != "" {
		fmt.Printf("  Default:    %s (IDE/VS Code)\n", cfg.DefaultProfile)
	}

	fmt.Printf("  Profiles:   %d\n", len(cfg.Profiles))

	if len(cfg.Profiles) > 0 {
		fmt.Println()
		store := keystore.New()
		checker := credentials.NewChecker(store)
		for _, p := range cfg.Profiles {
			status := checker.Check(p.Dir, p.Name, p.AuthMethod)
			icon := "✗"
			c := red
			if status.Valid {
				icon = "✓"
				c = green
			}
			c.Printf("  %s %s (%s) — %s\n", icon, p.Name, p.AuthMethod, status.Detail)
		}
	}

	return nil
}
