// Package consolidate audits and (selectively) repairs Claude Code asset
// drift across host, profile, and project scopes.
//
// The companion skill at internal/consolidate/skill/consolidate-claude-assets
// drives the rich interactive flow inside Claude Code. The Go CLI surfaced
// via `ccpm consolidate` is a thin orchestrator: it prints a structured
// audit report and can apply a small set of low-risk auto-fixes
// (dangling symlinks, stale plugin caches) when invoked with --fix. For
// proposal-heavy categories (duplicates, plugin/skill overlap, budget
// overflow) it points the user at the slash skill.
package consolidate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fatih/color"
)

// Severity tags are used to sort + filter issues for display.
type Severity int

const (
	SevInfo Severity = iota
	SevWarn
	SevError
)

func (s Severity) String() string {
	switch s {
	case SevError:
		return "error"
	case SevWarn:
		return "warn"
	default:
		return "info"
	}
}

// Issue is a single detected anomaly in the asset cascade.
type Issue struct {
	Category string
	Severity Severity
	Scope    string
	Asset    string
	Detail   string

	// AutoFix is non-nil when the consolidate Go CLI knows how to repair the
	// issue without user input beyond a top-level --fix flag. Categories that
	// require per-issue user choice leave this nil; the slash skill handles
	// them.
	AutoFix func() error
}

// Options controls Run behavior.
type Options struct {
	// DryRun forces inventory + detection only. No fixes attempted regardless
	// of Fix flag. Equivalent to omitting --fix.
	DryRun bool

	// Fix opts in to the small set of low-risk auto-fixes (dangling symlinks,
	// stale caches). Per-issue confirmation is still printed before applying.
	Fix bool

	// Profile narrows detection to a single ccpm profile. Empty = all profiles
	// + host scope.
	Profile string

	// Out is the writer for human-readable output. Defaults to os.Stdout.
	Out io.Writer
}

// Run is the top-level entry point for `ccpm consolidate` (excluding the
// --install-skill mode which is dispatched separately).
func Run(opts Options) error {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}

	snap, err := Inventory()
	if err != nil {
		return fmt.Errorf("inventory: %w", err)
	}

	if opts.Profile != "" {
		snap = snap.FilterProfile(opts.Profile)
	}

	issues := Detect(snap)
	printSummary(opts.Out, snap, issues)

	if opts.DryRun || !opts.Fix {
		printIssues(opts.Out, issues)
		printGuidance(opts.Out, issues)
		return nil
	}

	// --fix mode: apply only the autoFixable issues; leave the rest for the
	// slash skill.
	autoFixable := filterAutoFixable(issues)
	if len(autoFixable) == 0 {
		fmt.Fprintln(opts.Out, "No auto-fixable issues. Run /consolidate-claude-assets in Claude Code for interactive proposals.")
		printIssues(opts.Out, issues)
		return nil
	}

	applied, failed := applyAutoFixes(opts.Out, autoFixable)
	printGuidance(opts.Out, filterRemaining(issues))
	if failed > 0 {
		return fmt.Errorf("applied %d fix(es), %d failed — re-run `ccpm consolidate` to see what remains", applied, failed)
	}
	return nil
}

func filterAutoFixable(issues []Issue) []Issue {
	var out []Issue
	for _, i := range issues {
		if i.AutoFix != nil {
			out = append(out, i)
		}
	}
	return out
}

func filterRemaining(issues []Issue) []Issue {
	var out []Issue
	for _, i := range issues {
		if i.AutoFix == nil {
			out = append(out, i)
		}
	}
	return out
}

func printSummary(out io.Writer, snap Snapshot, issues []Issue) {
	bold := color.New(color.Bold)
	dim := color.New(color.Faint)

	bold.Fprintln(out, "Inventory")
	fmt.Fprintf(out, "  host skills:    %d\n", len(snap.HostSkills))
	fmt.Fprintf(out, "  host plugins:   %d enabled\n", len(snap.HostPlugins))
	fmt.Fprintf(out, "  host MCPs:      %d\n", len(snap.HostMCPs))
	if snap.CCPMPresent {
		fmt.Fprintf(out, "  ccpm profiles:  %d\n", len(snap.Profiles))
		dim.Fprintf(out, "    %s\n", strings.Join(profileNames(snap), ", "))
	}
	fmt.Fprintln(out)

	bold.Fprintln(out, "Issues")
	if len(issues) == 0 {
		color.New(color.FgGreen).Fprintln(out, "  ✓ no issues detected")
	} else {
		counts := map[string]int{}
		for _, i := range issues {
			counts[i.Severity.String()]++
		}
		sevs := []string{"error", "warn", "info"}
		for _, s := range sevs {
			if counts[s] > 0 {
				fmt.Fprintf(out, "  %s: %d\n", s, counts[s])
			}
		}
	}
	fmt.Fprintln(out)
}

func profileNames(snap Snapshot) []string {
	names := make([]string, 0, len(snap.Profiles))
	for n := range snap.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func printIssues(out io.Writer, issues []Issue) {
	if len(issues) == 0 {
		return
	}
	bold := color.New(color.Bold)
	bold.Fprintln(out, "Detected issues")
	// Sort by severity then category for stable output
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Severity != issues[j].Severity {
			return issues[i].Severity > issues[j].Severity
		}
		return issues[i].Category < issues[j].Category
	})
	for _, i := range issues {
		marker := "  •"
		if i.AutoFix != nil {
			marker = "  ⚙"
		}
		fmt.Fprintf(out, "%s [%s] %s | %s | %s — %s\n",
			marker, i.Severity, i.Category, i.Scope, i.Asset, i.Detail)
	}
	fmt.Fprintln(out)
	color.New(color.Faint).Fprintln(out, "  ⚙ = auto-fixable with --fix")
}

func printGuidance(out io.Writer, remaining []Issue) {
	if len(remaining) == 0 {
		return
	}
	bold := color.New(color.Bold)
	bold.Fprintln(out, "Next steps")
	fmt.Fprintln(out, "  Run /consolidate-claude-assets in Claude Code for an interactive proposal flow")
	fmt.Fprintln(out, "  on the remaining issues. The skill walks each category and applies fixes")
	fmt.Fprintln(out, "  only after per-issue confirmation. Backups happen automatically.")
	fmt.Fprintln(out)
	if !skillInstalled() {
		fmt.Fprintln(out, "  Install the skill first:  ccpm consolidate --install-skill")
	}
}

func skillInstalled() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".claude", "skills", "consolidate-claude-assets", "SKILL.md"))
	return err == nil
}

// applyAutoFixes runs every auto-fixable issue's fix and reports how many
// applied vs failed; individual failures are printed but never abort the run.
func applyAutoFixes(out io.Writer, autoFixable []Issue) (applied, failed int) {
	bold := color.New(color.Bold)
	green := color.New(color.FgGreen, color.Bold)
	red := color.New(color.FgRed, color.Bold)

	bold.Fprintln(out, "Auto-fixes")
	for _, i := range autoFixable {
		fmt.Fprintf(out, "  applying: %s | %s | %s\n", i.Category, i.Asset, i.Detail)
		if err := i.AutoFix(); err != nil {
			red.Fprintf(out, "    ✗ %v\n", err)
			failed++
			continue
		}
		green.Fprintln(out, "    ✓ applied")
		applied++
	}
	fmt.Fprintln(out)
	return applied, failed
}
