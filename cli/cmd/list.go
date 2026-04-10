package cmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/credentials"
	"github.com/nitin-1926/ccpm/internal/keystore"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all profiles with status",
	Aliases: []string{"ls"},
	RunE:    runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if len(cfg.Profiles) == 0 {
		fmt.Println("No profiles found. Create one with: ccpm add <name>")
		return nil
	}

	store := keystore.New()
	checker := credentials.NewChecker(store)

	// Sort profiles by name
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	bold := color.New(color.Bold).SprintFunc()

	// Print header
	fmt.Printf("  %-16s %-10s %-30s %-8s %s\n",
		bold("NAME"), bold("AUTH"), bold("STATUS"), bold("DEFAULT"), bold("LAST USED"))
	fmt.Printf("  %s\n", strings.Repeat("─", 80))

	for _, name := range names {
		p := cfg.Profiles[name]

		status := checker.Check(p.Dir, p.Name, p.AuthMethod)
		statusStr := red("✗ " + status.Detail)
		if status.Valid {
			if status.ExpireAt != "" {
				exp, _ := time.Parse(time.RFC3339, status.ExpireAt)
				if time.Until(exp) < 7*24*time.Hour {
					statusStr = yellow("⚠ " + status.Detail)
				} else {
					statusStr = green("✓ " + status.Detail)
				}
			} else {
				statusStr = green("✓ " + status.Detail)
			}
		}

		defaultMark := ""
		if cfg.DefaultProfile == name {
			defaultMark = "★"
		}

		lastUsed := "-"
		if p.LastUsed != "" {
			t, err := time.Parse(time.RFC3339, p.LastUsed)
			if err == nil {
				lastUsed = humanizeTime(t)
			}
		}

		fmt.Printf("  %-16s %-10s %-30s %-8s %s\n", name, p.AuthMethod, statusStr, defaultMark, lastUsed)
	}

	return nil
}

func humanizeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
