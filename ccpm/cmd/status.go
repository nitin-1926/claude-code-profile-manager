package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	claudepkg "github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/claude"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/credentials"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/keystore"
)

var statusJSON bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show system overview",
	RunE:  runStatus,
}

func init() {
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "machine-readable JSON output")
	rootCmd.AddCommand(statusCmd)
}

// statusReport is the machine-readable shape of `ccpm status --json`. The
// text renderer reads from the same struct so the two outputs can't drift.
type statusReport struct {
	Version        string                `json:"version"`
	ConfigDir      string                `json:"config_dir"`
	ClaudeBinary   string                `json:"claude_binary,omitempty"`
	ClaudeError    string                `json:"claude_error,omitempty"`
	ActiveProfile  string                `json:"active_profile,omitempty"`
	DefaultProfile string                `json:"default_profile,omitempty"`
	Profiles       []profileStatusReport `json:"profiles"`
}

type profileStatusReport struct {
	Name       string `json:"name"`
	AuthMethod string `json:"auth_method"`
	Valid      bool   `json:"valid"`
	Detail     string `json:"detail"`
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	report := statusReport{
		Version:        version,
		ActiveProfile:  os.Getenv("CCPM_ACTIVE_PROFILE"),
		DefaultProfile: cfg.DefaultProfile,
		Profiles:       []profileStatusReport{},
	}
	report.ConfigDir, _ = config.BaseDir()
	if bin, err := claudepkg.FindBinary(); err != nil {
		report.ClaudeError = err.Error()
	} else {
		report.ClaudeBinary = bin
	}

	if len(cfg.Profiles) > 0 {
		store := keystore.New()
		checker := credentials.NewChecker(store)
		// Stable order (the old map iteration was random per run).
		names := config.ProfileNames(cfg)
		sort.Strings(names)
		for _, name := range names {
			p := cfg.Profiles[name]
			st := checker.Check(p.Dir, p.Name, p.AuthMethod)
			report.Profiles = append(report.Profiles, profileStatusReport{
				Name:       p.Name,
				AuthMethod: p.AuthMethod,
				Valid:      st.Valid,
				Detail:     st.Detail,
			})
		}
	}

	if statusJSON {
		out, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	}

	bold := color.New(color.Bold)
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)

	bold.Println("ccpm status")
	fmt.Printf("  Version:    %s\n", report.Version)
	fmt.Printf("  Config dir: %s\n", report.ConfigDir)

	if report.ClaudeError != "" {
		red.Printf("  Claude:     not found (%s)\n", report.ClaudeError)
	} else {
		green.Printf("  Claude:     %s\n", report.ClaudeBinary)
	}

	if report.ActiveProfile != "" {
		fmt.Printf("  Active:     %s (shell)\n", report.ActiveProfile)
	}
	if report.DefaultProfile != "" {
		fmt.Printf("  Default:    %s (IDE/VS Code)\n", report.DefaultProfile)
	}

	fmt.Printf("  Profiles:   %d\n", len(cfg.Profiles))

	if len(report.Profiles) > 0 {
		fmt.Println()
		for _, p := range report.Profiles {
			icon := "✗"
			c := red
			if p.Valid {
				icon = "✓"
				c = green
			}
			c.Printf("  %s %s (%s) — %s\n", icon, p.Name, p.AuthMethod, p.Detail)
		}
	}

	return nil
}
