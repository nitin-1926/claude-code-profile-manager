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

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/claude"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/credentials"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/defaultclaude"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/keystore"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/manifest"
	"golang.org/x/mod/semver"
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
  - Flag platform-specific caveats (e.g. macOS OAuth requires Claude Code 2.1.56+)

doctor is read-only. To audit and repair asset drift across host, profile,
and project scopes (duplicates, dangling symlinks, ghost manifest entries,
budget overflow), run ` + "`ccpm consolidate`" + ` instead.`,
	RunE: runDoctor,
}

// doctorFix, when set via --fix, lets doctor perform safe, idempotent repairs
// (currently: pruning dangling shared-asset symlinks) instead of only
// reporting them. Destructive cross-scope cleanup still belongs to
// `ccpm consolidate --fix`; this is the narrow "my links are broken" repair.
var doctorFix bool

// minMacOSOAuthClaudeVersion is the first Claude Code release that namespaces
// keychain entries by CLAUDE_CONFIG_DIR. Below this, OAuth profiles on macOS
// cannot be isolated and `ccpm auth status` will look wrong.
const minMacOSOAuthClaudeVersion = "2.1.56"

func init() {
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "prune dangling shared-asset symlinks found during the check")
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
	fmt.Printf("  ccpm version: %s\n", rootCmd.Version)
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
	if doctorFix {
		removed, failed := fixBrokenSymlinks(profileDirs)
		if len(removed) == 0 && len(failed) == 0 {
			if len(profileDirs) > 0 {
				green.Println("  ✓ no broken symlinks to prune")
			} else {
				dim.Println("  (no profiles to check)")
			}
		}
		for _, msg := range removed {
			green.Printf("  ✓ pruned %s\n", msg)
		}
		for _, msg := range failed {
			red.Printf("  ✗ could not prune %s\n", msg)
			issues++
		}
	} else {
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
			dim.Println("  Re-run with `ccpm doctor --fix` to prune these dangling links.")
		}
	}
	fmt.Println()

	// -----------------------------------------------------------------
	// Section 5.5: housekeeping (orphans, stale staging, Windows fallback)
	// -----------------------------------------------------------------
	bold.Println("Housekeeping")
	houseFindings := 0
	if base, err := config.BaseDir(); err == nil {
		// Profile dirs on disk with no config entry (e.g. an interrupted add
		// or remove) — invisible to every other command.
		profilesRoot := filepath.Join(base, "profiles")
		if entries, err := os.ReadDir(profilesRoot); err == nil {
			known := map[string]bool{}
			for _, p := range cfg.Profiles {
				known[filepath.Base(p.Dir)] = true
			}
			for _, e := range entries {
				if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
					continue
				}
				if !known[e.Name()] {
					yellow.Printf("  ! orphaned profile directory (not in config.json): %s\n", filepath.Join(profilesRoot, e.Name()))
					warnings++
					houseFindings++
				}
			}
		}

		// Stale atomicwrite staging files — leftovers from a crashed write.
		stale := findStaleStagedFiles(append([]string{base}, profileDirValues(profileDirs)...))
		for _, s := range stale {
			yellow.Printf("  ! stale staging file (crashed write?): %s\n", s)
			warnings++
			houseFindings++
		}

		// Windows copy-fallback breadcrumb: dedup is silently degraded.
		if _, err := os.Stat(filepath.Join(base, ".windows-copy-fallback")); err == nil {
			yellow.Println("  ! shared assets are COPIED, not symlinked (Windows without Developer Mode)")
			dim.Println("    Enable Developer Mode and re-run `ccpm sync` to restore deduplication.")
			warnings++
			houseFindings++
		}
	}
	if houseFindings == 0 {
		green.Println("  ✓ no orphaned profile dirs, stale staging files, or degraded-dedup markers")
	}
	fmt.Println()

	// -----------------------------------------------------------------
	// Section 6: manifest + drift fingerprint
	// -----------------------------------------------------------------
	bold.Println("Shared asset manifest")
	m, err := manifest.Load()
	if err != nil {
		yellow.Printf("  ! could not load manifest: %v\n", err)
		warnings++
	} else {
		hostCount := 0
		globalCount := 0
		profileCount := 0
		for _, inst := range m.Installs {
			switch inst.Scope {
			case manifest.ScopeHost:
				hostCount++
			case manifest.ScopeGlobal:
				globalCount++
			case manifest.ScopeProfile:
				profileCount++
			}
		}
		fmt.Printf("  %d entries tracked in %s (global: %d, host: %d, profile: %d)\n",
			len(m.Installs), manifestDisplayPath(), globalCount, hostCount, profileCount)
	}
	fmt.Println()

	// -----------------------------------------------------------------
	// Section 6.5: host-asset cascade (Project > Profile > Global)
	// -----------------------------------------------------------------
	bold.Println("Host-asset cascade (~/.claude → profiles)")
	if !cfg.Settings.CascadeAutoAdoptEnabled() {
		yellow.Println("  ! cascade_auto_adopt is OFF — host assets will NOT be adopted at launch")
		dim.Println("    Re-enable with: ccpm config set cascade_auto_adopt true")
		warnings++
	} else {
		dim.Println("  cascade_auto_adopt is ON (default)")
	}
	if defaultclaude.Exists() && len(profileDirs) > 0 {
		summary, shadows := defaultclaude.HostCascadeSummary(profileDirs, m)
		if summary == "" {
			green.Println("  ✓ no host assets discovered yet")
		} else {
			fmt.Printf("  %s\n", summary)
		}
		for _, s := range shadows {
			yellow.Printf("  ! shadowed: %q (%s) exists in both ~/.claude and profile %q — profile wins\n",
				s.Name, s.Kind, s.Profile)
			warnings++
		}
	}
	fmt.Println()

	bold.Println("Default ~/.claude drift")
	if !defaultclaude.Exists() {
		dim.Printf("  %s not present — nothing to drift-check\n", defaultDir)
	} else {
		home, _ := os.UserHomeDir()
		if home != "" {
			if _, err := os.Stat(filepath.Join(home, ".claude.json")); err == nil {
				yellow.Printf("  ! ~/.claude.json exists and is global — MCP defined there is NOT isolated per profile\n")
				yellow.Printf("    Prefer 'ccpm mcp add' so fragments are merged into each profile's settings.json\n")
				warnings++
			}
		}

		stored, err := defaultclaude.LoadFingerprint()
		if err != nil {
			yellow.Printf("  ! could not load fingerprint: %v\n", err)
			warnings++
		} else if stored == nil {
			dim.Println("  no fingerprint recorded — run 'ccpm import default' or 'ccpm default fingerprint update'")
		} else {
			current, err := defaultclaude.Snapshot(defaultclaude.DefaultTargets())
			if err != nil {
				yellow.Printf("  ! snapshot failed: %v\n", err)
				warnings++
			} else {
				drift := defaultclaude.Compare(stored, current)
				if drift.HasChanges() {
					yellow.Printf("  ! drift detected: +%d ~%d -%d since %s\n",
						len(drift.Added), len(drift.Modified), len(drift.Removed), stored.TakenAt)
					fmt.Println("    See 'ccpm default fingerprint check' for the full list.")
					warnings++
				} else {
					green.Printf("  ✓ no drift since %s\n", stored.TakenAt)
				}
			}
		}
	}
	fmt.Println()

	// -----------------------------------------------------------------
	// Section 7: drift notifications
	// -----------------------------------------------------------------
	bold.Println("Drift notifications")
	if cfg.Settings.CheckDefaultDrift {
		green.Println("  ✓ enabled — 'ccpm run' and 'ccpm use' will warn on drift")
	} else {
		dim.Println("  off — enable with 'ccpm config set check_default_drift true'")
	}
	fmt.Println()

	// -----------------------------------------------------------------
	// Section 8: notes
	// -----------------------------------------------------------------
	bold.Println("Notes")
	fmt.Println("  · CLAUDE_CONFIG_DIR relocates most Claude paths into a profile, but")
	fmt.Println("    ~/.claude.json stays global — see Anthropic docs for MCP specifics.")
	fmt.Println("  · 'ccpm run' execs claude with CLAUDE_CONFIG_DIR set; 'ccpm use' only")
	fmt.Println("    exports it for the current shell (needs the shell hook).")
	if runtime.GOOS == "windows" {
		fmt.Println("  · On Windows, ccpm falls back to copying when symlinks are not")
		fmt.Println("    available. Enable Developer Mode for true symlink deduplication.")
	}
	fmt.Println()

	// -----------------------------------------------------------------
	// Exit code: only real issues fail; warnings never do. Returning an
	// error lets Cobra's normal exit-code path (cmd/root.go Execute) take
	// over — avoids the previous os.Exit(1) call that bypassed deferred
	// cleanup.
	// -----------------------------------------------------------------
	if issues > 0 {
		red.Printf("✗ %d issue(s), %d warning(s)\n", issues, warnings)
		// exitUnhealthy (4) distinguishes "health check found problems" from
		// "the doctor command itself failed" (1) for scripts.
		return exitWithCode(exitUnhealthy, fmt.Errorf("%d issue(s), %d warning(s)", issues, warnings))
	}
	if warnings > 0 {
		yellow.Printf("! %d warning(s), 0 issues\n", warnings)
		return nil
	}
	green.Println("✓ All checks passed")
	return nil
}

// checkSymlinkIntegrity inspects every dedupable-asset subdirectory in each
// profile and reports any dangling symlinks. Copies are tolerated (Windows
// fallback) — we can't distinguish "copy on purpose" from "stale copy"
// without hashing against the share store every time, which would be
// expensive.
//
// The previous version only looked at skills/, which meant a broken
// agents/commands/rules/hooks symlink would silently slide past doctor.
func checkSymlinkIntegrity(profileDirs map[string]string) []string {
	assetSubdirs := []string{"skills", "agents", "commands", "rules", "hooks"}
	var issues []string
	for name, dir := range profileDirs {
		for _, sub := range assetSubdirs {
			subdir := filepath.Join(dir, sub)
			entries, err := os.ReadDir(subdir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				path := filepath.Join(subdir, e.Name())
				info, err := os.Lstat(path)
				if err != nil {
					issues = append(issues, fmt.Sprintf("%s/%s/%s: cannot stat (%v)", name, sub, e.Name(), err))
					continue
				}
				if info.Mode()&os.ModeSymlink == 0 {
					continue
				}
				if _, err := filepath.EvalSymlinks(path); err != nil {
					issues = append(issues, fmt.Sprintf("%s/%s/%s: broken symlink (%v)", name, sub, e.Name(), err))
				}
			}
		}
	}
	sort.Strings(issues)
	return issues
}

// fixBrokenSymlinks prunes dangling shared-asset symlinks across every profile,
// the repair behind `ccpm doctor --fix`. It only ever removes symbolic links
// whose target no longer resolves — never regular files or copies — so it
// cannot delete real data. Returns the human-readable labels of links removed
// and of any that could not be removed.
func fixBrokenSymlinks(profileDirs map[string]string) (removed, failed []string) {
	assetSubdirs := []string{"skills", "agents", "commands", "rules", "hooks"}
	for name, dir := range profileDirs {
		for _, sub := range assetSubdirs {
			subdir := filepath.Join(dir, sub)
			entries, err := os.ReadDir(subdir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				path := filepath.Join(subdir, e.Name())
				info, err := os.Lstat(path)
				if err != nil || info.Mode()&os.ModeSymlink == 0 {
					continue
				}
				if _, err := filepath.EvalSymlinks(path); err == nil {
					continue // link resolves — leave it alone
				}
				label := fmt.Sprintf("%s/%s/%s", name, sub, e.Name())
				if rmErr := os.Remove(path); rmErr != nil {
					failed = append(failed, fmt.Sprintf("%s (%v)", label, rmErr))
				} else {
					removed = append(removed, label)
				}
			}
		}
	}
	sort.Strings(removed)
	sort.Strings(failed)
	return removed, failed
}

// compareSemver returns -1 / 0 / +1 comparing version strings via
// golang.org/x/mod/semver. Claude's quirky output forms ("v2.1.56",
// "2.1.56 (claude-code)") are normalized first; anything unparseable
// canonicalizes to "v0" (sorts lowest), matching the old lenient behavior.
func compareSemver(a, b string) int {
	return semver.Compare(normalizeSemver(a), normalizeSemver(b))
}

// normalizeSemver reduces a raw version string to a canonical "vX.Y.Z" form
// that x/mod/semver accepts: first whitespace-delimited token, "v" prefix
// ensured, missing minor/patch filled with zeros.
func normalizeSemver(raw string) string {
	tok := strings.TrimSpace(raw)
	if i := strings.IndexByte(tok, ' '); i >= 0 {
		tok = tok[:i]
	}
	tok = "v" + strings.TrimPrefix(tok, "v")
	if !semver.IsValid(tok) {
		// Fill missing components ("v2.1" → "v2.1.0") before giving up.
		for range 2 {
			tok += ".0"
			if semver.IsValid(tok) {
				return tok
			}
		}
		return "v0.0.0"
	}
	return tok
}

// profileDirValues flattens the profile→dir map for path scans.
func profileDirValues(profileDirs map[string]string) []string {
	out := make([]string, 0, len(profileDirs))
	for _, d := range profileDirs {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// findStaleStagedFiles scans the given roots (shallowly, plus one level of
// subdirs) for `.ccpm-staged-*` files — leftovers from a write that crashed
// between stage and rename. atomicwrite normally removes them.
func findStaleStagedFiles(roots []string) []string {
	var stale []string
	seen := map[string]bool{}
	scan := func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if strings.Contains(e.Name(), ".ccpm-staged-") {
				p := filepath.Join(dir, e.Name())
				if !seen[p] {
					seen[p] = true
					stale = append(stale, p)
				}
			}
		}
	}
	for _, root := range roots {
		scan(root)
		if entries, err := os.ReadDir(root); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					scan(filepath.Join(root, e.Name()))
				}
			}
		}
	}
	sort.Strings(stale)
	return stale
}

func manifestDisplayPath() string {
	base, err := config.BaseDir()
	if err != nil {
		return "~/.ccpm/installs.json"
	}
	return filepath.Join(base, "installs.json")
}
