package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/defaultclaude"
	"github.com/nitin-1926/ccpm/internal/filetree"
	"github.com/nitin-1926/ccpm/internal/picker"
	"github.com/nitin-1926/ccpm/internal/settingsmerge"
)

var (
	importProfile string
	importAll     bool
	importDryRun  bool
	importOnly    []string
	importForce   bool

	importFromSrc       string
	importFromTarget    string
	importNoShare       bool
	importLiveSymlinks  bool
	importNoLiveSymlink bool
	importSelectAll     bool
	importMCPScope      string
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import assets from external sources into profiles",
}

var importDefaultCmd = &cobra.Command{
	Use:   "default",
	Short: "Import skills, hooks, MCP servers, and settings from ~/.claude into profiles",
	Long: `Copy or merge selected subtrees from the default Claude Code config
directory (~/.claude) into one or all ccpm profiles.

By default, imports skills, commands, rules, hooks, agents, settings, and MCP
servers discovered in ~/.claude.json. Plugins are excluded unless passed
explicitly via --only plugins.

Settings are deep-merged into the profile's settings.json via the same merge
engine used by 'ccpm run'; directory targets are copied file-by-file, preserving
existing profile files unless --force is passed; MCP servers are written into
the appropriate ccpm MCP fragment (~/.ccpm/share/mcp/<scope>.json) and
materialized into settings.json#mcpServers on 'ccpm use' / 'ccpm run'.

Interactive runs drill down to per-item selection for skills, commands, rules,
hooks, agents, and MCP — pick only the entries you want. Pass --select-all to
skip the per-item prompt and import every entry under the selected targets.

Use --live-symlinks with deduped imports (--no-share not set) so top-level
skills/agents/commands that are symlinked directories in ~/.claude stay as
symlinks into the share store (pointing at the resolved path). Edits in the
original tree are then visible in every linked profile without re-import.

Examples:
  ccpm import default --profile work
  ccpm import default --all --dry-run
  ccpm import default --profile personal --only skills,hooks
  ccpm import default --all --only settings --force
  ccpm import default --all --only skills --live-symlinks
  ccpm import default --profile work --only mcp --mcp-scope global
  ccpm import default --all --select-all`,
	RunE: runImportDefault,
}

var importFromProfileCmd = &cobra.Command{
	Use:   "from-profile",
	Short: "Copy assets from one ccpm profile into another",
	Long: `Clone skills, commands, rules, hooks, agents, and (optionally) settings
from a source ccpm profile into a target profile. Useful when spinning up a
new profile that should start with the same loadout as an existing one.

Examples:
  ccpm import from-profile --src work --profile personal
  ccpm import from-profile --src work --profile playground --only skills
  ccpm import from-profile --src work --profile playground --force`,
	RunE: runImportFromProfile,
}

func init() {
	importDefaultCmd.Flags().StringVar(&importProfile, "profile", "", "target profile name")
	importDefaultCmd.Flags().BoolVar(&importAll, "all", false, "import into every profile")
	importDefaultCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "preview without writing")
	importDefaultCmd.Flags().StringSliceVar(&importOnly, "only", nil, "comma-separated targets (skills, commands, rules, hooks, agents, settings, plugins)")
	importDefaultCmd.Flags().BoolVar(&importForce, "force", false, "overwrite existing files in profiles")
	importDefaultCmd.Flags().BoolVar(&importNoShare, "no-share", false, "copy assets directly into the profile instead of symlinking from ~/.ccpm/share")

	importFromProfileCmd.Flags().StringVar(&importFromSrc, "src", "", "source profile to copy from (required)")
	importFromProfileCmd.Flags().StringVar(&importFromTarget, "profile", "", "target profile to copy into (required)")
	importFromProfileCmd.Flags().StringSliceVar(&importOnly, "only", nil, "comma-separated targets (skills, commands, rules, hooks, agents, settings)")
	importFromProfileCmd.Flags().BoolVar(&importForce, "force", false, "overwrite existing files in target profile")

	importCmd.AddCommand(importDefaultCmd)
	importCmd.AddCommand(importFromProfileCmd)
	rootCmd.AddCommand(importCmd)
}

func runImportDefault(cmd *cobra.Command, args []string) error {
	if importProfile == "" && !importAll {
		return fmt.Errorf("specify --profile <name> or --all")
	}
	if importProfile != "" && importAll {
		return fmt.Errorf("use either --profile or --all, not both")
	}

	if !defaultclaude.Exists() {
		src, _ := defaultclaude.DefaultDir()
		return fmt.Errorf("no default Claude config found at %s", src)
	}

	targets, err := defaultclaude.ParseTargets(importOnly)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	var names []string
	if importAll {
		names = config.ProfileNames(cfg)
		if len(names) == 0 {
			return fmt.Errorf("no profiles found — create one with 'ccpm add'")
		}
	} else {
		if _, ok := cfg.Profiles[importProfile]; !ok {
			return fmt.Errorf("profile %q not found", importProfile)
		}
		names = []string{importProfile}
	}

	green := color.New(color.FgGreen, color.Bold)
	cyan := color.New(color.FgCyan)
	dim := color.New(color.Faint)

	for _, name := range names {
		p := cfg.Profiles[name]

		plan, err := defaultclaude.Import(p.Dir, defaultclaude.ImportOptions{
			Targets:     targets,
			DryRun:      importDryRun,
			Force:       importForce,
			Dedupe:      !importNoShare,
			ProfileName: name,
		})
		if err != nil {
			return fmt.Errorf("import into %q: %w", name, err)
		}

		header := fmt.Sprintf("Profile %q", name)
		if importDryRun {
			header += " (dry-run)"
		}
		cyan.Println(header)

		for _, action := range plan.Actions {
			switch action.Kind {
			case "skip-missing":
				dim.Printf("  - %-9s skipped (source missing)\n", action.Target)
			case "merge-settings":
				cyan.Printf("  ~ %-9s merge into settings.json\n", action.Target)
			case "copy":
				green.Printf("  + %-9s %s\n", action.Target, relOrAbs(action.TargetPath))
			case "link":
				green.Printf("  → %-9s %s (shared)\n", action.Target, relOrAbs(action.TargetPath))
			default:
				fmt.Printf("  · %-9s %s\n", action.Target, action.Kind)
			}
		}

		if !importDryRun {
			if err := mergeImportedSettings(p.Dir); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: settings merge failed: %v\n", err)
			}

			if err := settingsmerge.Materialize(p.Dir, name); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: re-materializing settings: %v\n", err)
			}
			if err := settingsmerge.MaterializeMCP(p.Dir, name); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: re-materializing MCP: %v\n", err)
			}
		}
	}

	if !importDryRun {
		snap, err := defaultclaude.Snapshot(defaultclaude.DefaultTargets())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not snapshot ~/.claude: %v\n", err)
		} else if err := defaultclaude.SaveFingerprint(snap); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not save fingerprint: %v\n", err)
		} else {
			dim.Printf("\nFingerprint updated (%d files tracked).\n", len(snap.Files))
		}
	}

	return nil
}

// mergeImportedSettings consumes the staged settings.json.ccpm-import produced
// by Import and deep-merges it into the profile's settings.json. The staged
// file is removed after a successful merge.
func mergeImportedSettings(profileDir string) error {
	staged := filepath.Join(profileDir, "settings.json.ccpm-import")
	if _, err := os.Stat(staged); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	patch, err := settingsmerge.LoadJSON(staged)
	if err != nil {
		return err
	}
	target := filepath.Join(profileDir, "settings.json")
	existing, err := settingsmerge.LoadJSON(target)
	if err != nil {
		return err
	}
	merged := settingsmerge.DeepMerge(existing, patch)
	if err := settingsmerge.WriteJSON(target, merged); err != nil {
		return err
	}
	return os.Remove(staged)
}

func runImportFromProfile(cmd *cobra.Command, args []string) error {
	if importFromSrc == "" || importFromTarget == "" {
		return fmt.Errorf("both --src and --profile are required")
	}
	if importFromSrc == importFromTarget {
		return fmt.Errorf("source and target profiles must differ")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	src, ok := cfg.Profiles[importFromSrc]
	if !ok {
		return fmt.Errorf("source profile %q not found", importFromSrc)
	}
	dst, ok := cfg.Profiles[importFromTarget]
	if !ok {
		return fmt.Errorf("target profile %q not found", importFromTarget)
	}

	targets, err := defaultclaude.ParseTargets(importOnly)
	if err != nil {
		return err
	}

	if err := importFromProfile(src.Dir, dst.Dir, targets, importForce); err != nil {
		return err
	}

	if err := settingsmerge.Materialize(dst.Dir, importFromTarget); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: re-materializing settings: %v\n", err)
	}
	if err := settingsmerge.MaterializeMCP(dst.Dir, importFromTarget); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: re-materializing MCP: %v\n", err)
	}

	color.New(color.FgGreen, color.Bold).Printf("✓ Imported assets from %q into %q\n", importFromSrc, importFromTarget)
	return nil
}

// importFromProfile copies selected targets from srcProfileDir into
// dstProfileDir. Settings are deep-merged; directory targets are walked
// and merged with the "preserve existing unless --force" rule used by
// `ccpm import default`.
func importFromProfile(srcProfileDir, dstProfileDir string, targets []defaultclaude.Target, force bool) error {
	for _, t := range targets {
		srcPath := profileTargetPath(srcProfileDir, t)
		dstPath := profileTargetPath(dstProfileDir, t)

		info, err := os.Stat(srcPath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("stat %s: %w", srcPath, err)
		}

		if t == defaultclaude.TargetSettings {
			if err := mergeProfileSettings(srcPath, dstPath); err != nil {
				return fmt.Errorf("merging settings from %s: %w", srcProfileDir, err)
			}
			continue
		}

		if info.IsDir() {
			if err := copyProfileTree(srcPath, dstPath, force); err != nil {
				return fmt.Errorf("copying %s: %w", srcPath, err)
			}
			continue
		}
	}
	return nil
}

func profileTargetPath(root string, t defaultclaude.Target) string {
	if t == defaultclaude.TargetSettings {
		return filepath.Join(root, "settings.json")
	}
	return filepath.Join(root, string(t))
}

// mergeProfileSettings deep-merges src settings.json into dst. Existing keys
// in dst win so we don't clobber profile-specific overrides.
func mergeProfileSettings(src, dst string) error {
	srcData, err := settingsmerge.LoadJSON(src)
	if err != nil {
		return err
	}
	dstData, err := settingsmerge.LoadJSON(dst)
	if err != nil {
		return err
	}
	merged := settingsmerge.DeepMerge(srcData, dstData)
	return settingsmerge.WriteJSON(dst, merged)
}

func copyProfileTree(src, dst string, force bool) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if _, err := os.Stat(target); err == nil && !force {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func relOrAbs(p string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
			return "~/" + rel
		}
	}
	return p
}
