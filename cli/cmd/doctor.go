package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/ccpm/internal/claude"
	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/credentials"
	"github.com/nitin-1926/ccpm/internal/defaultclaude"
	"github.com/nitin-1926/ccpm/internal/keystore"
	"github.com/nitin-1926/ccpm/internal/manifest"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose ccpm environment health and drift",
	Long: `Run a health check over ccpm:
  - Locate the Claude Code binary and report its version
  - Summarize configured profiles and their auth health
  - Compare ~/.claude contents against profiles (unadopted items)
  - Check symlink integrity for shared skills
  - Show drift against the last ~/.claude fingerprint
  - Flag platform-specific caveats (e.g. macOS OAuth requires Claude Code 2.1.56+)`,
	RunE: runDoctor,
}

// minMacOSOAuthClaudeVersion is the first Claude Code release that namespaces
// keychain entries by CLAUDE_CONFIG_DIR. Below this, OAuth profiles on macOS
// cannot be isolated and `ccpm auth status` will look wrong.
const minMacOSOAuthClaudeVersion = "2.1.56"

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	green := color.New(color.FgGreen, color.Bold)
	yellow := color.New(color.FgYellow, color.Bold)
	red := color.New(color.FgRed, color.Bold)
	bold := color.New(color.Bold)
	dim := color.New(color.Faint)

	issues := 0
	warnings := 0

	// -----------------------------------------------------------------
	// Section 1: environment
	// -----------------------------------------------------------------
	bold.Println("Environment")
	bin, binErr := claude.FindBinary()
	if binErr == nil {
		green.Printf("  ✓ claude binary: %s\n", bin)
		if v := claude.Version(); v != "" {
			fmt.Printf("    version: %s\n", v)
			if runtime.GOOS == "darwin" {
				if cmp := compareSemver(v, minMacOSOAuthClaudeVersion); cmp < 0 {
					yellow.Printf("  ! Claude Code %s is older than %s — OAuth profiles on macOS\n", v, minMacOSOAuthClaudeVersion)
					yellow.Println("    cannot be isolated. Upgrade with: npm i -g @anthropic-ai/claude-code")
					warnings++
				}
			}
		} else {
			dim.Println("    version: unknown (claude --version returned nothing)")
		}
	} else {
		red.Printf("  ✗ %v\n", binErr)
		issues++
	}
	fmt.Printf("  ccpm version: %s\n", rootVersion())
	fmt.Printf("  platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Println()

	// -----------------------------------------------------------------
	// Section 2: ccpm base directory
	// -----------------------------------------------------------------
	bold.Println("ccpm base directory")
	base, err := config.BaseDir()
	if err != nil {
		red.Printf("  ✗ %v\n", err)
		issues++
	} else if _, err := os.Stat(base); err != nil {
		yellow.Printf("  ! %s does not exist yet (will be created on first use)\n", base)
		warnings++
	} else {
		green.Printf("  ✓ %s\n", base)
	}
	fmt.Println()

	cfg, err := config.Load()
	if err != nil {
		red.Printf("Config load failed: %v\n", err)
		return nil
	}

	// -----------------------------------------------------------------
	// Section 3: profiles + credential health
	// -----------------------------------------------------------------
	bold.Println("Profiles")
	profileDirs := map[string]string{}
	if len(cfg.Profiles) == 0 {
		yellow.Println("  ! no profiles yet — create one with 'ccpm add'")
		warnings++
	} else {
		store := keystore.New()
		checker := credentials.NewChecker(store)

		names := make([]string, 0, len(cfg.Profiles))
		for n := range cfg.Profiles {
			names = append(names, n)
		}
		sort.Strings(names)

		for _, name := range names {
			p := cfg.Profiles[name]
			marker := ""
			if name == cfg.DefaultProfile {
				marker = " (default)"
			}

			if _, err := os.Stat(p.Dir); err != nil {
				red.Printf("  ✗ %s%s — profile directory missing: %s\n", name, marker, p.Dir)
				issues++
				continue
			}
			profileDirs[name] = p.Dir

			status := checker.Check(p.Dir, p.Name, p.AuthMethod)
			icon := green.Sprint("✓")
			authLine := status.Detail
			if !status.Valid {
				icon = red.Sprint("✗")
				issues++
			} else if strings.Contains(status.Detail, "expires in") {
				icon = yellow.Sprint("⚠")
				warnings++
			}
			fmt.Printf("  %s %s%s (%s) — %s\n", icon, name, marker, p.AuthMethod, authLine)

			// Transparency for macOS OAuth: surface the namespaced keychain
			// service so users can inspect it in Keychain Access.
			if runtime.GOOS == "darwin" && p.AuthMethod == "oauth" {
				if svc, kerr := credentials.KeychainService(p.Dir); kerr == nil {
					dim.Printf("      keychain: %s\n", svc)
				}
			}
		}
	}
	fmt.Println()

	// -----------------------------------------------------------------
	// Section 4: ~/.claude vs profiles diff
	// -----------------------------------------------------------------
	bold.Println("~/.claude vs profiles")
	defaultDir, _ := defaultclaude.DefaultDir()
	if !defaultclaude.Exists() {
		dim.Printf("  %s not present — no comparison needed\n", defaultDir)
	} else if len(profileDirs) == 0 {
		dim.Println("  no profiles to compare against")
	} else {
		diffs := defaultclaude.ComputeProfileDiffs(defaultDir, profileDirs, []defaultclaude.Target{
			defaultclaude.TargetSkills,
			defaultclaude.TargetCommands,
			defaultclaude.TargetHooks,
			defaultclaude.TargetAgents,
			defaultclaude.TargetRules,
		})
		if !defaultclaude.HasUnadopted(diffs) {
			green.Println("  ✓ every item in ~/.claude has been adopted by at least one profile")
		} else {
			for _, d := range diffs {
				if len(d.OnlyInDefault) == 0 {
					continue
				}
				yellow.Printf("  ! %s in ~/.claude not in any profile: %s\n", d.Target, strings.Join(d.OnlyInDefault, ", "))
				warnings++
			}
			fmt.Println("    Suggestion: ccpm import default --only skills --all")
		}
	}
	fmt.Println()

	// -----------------------------------------------------------------
	// Section 5: symlink integrity
	// -----------------------------------------------------------------
	bold.Println("Shared symlink integrity")
	symlinkIssues := checkSymlinkIntegrity(profileDirs)
	if len(symlinkIssues) == 0 {
		if len(profileDirs) > 0 {
			green.Println("  ✓ no broken symlinks detected in profile skills")
		} else {
			dim.Println("  (no profiles to check)")
		}
	} else {
		for _, msg := range symlinkIssues {
			yellow.Printf("  ! %s\n", msg)
			warnings++
		}
	}
