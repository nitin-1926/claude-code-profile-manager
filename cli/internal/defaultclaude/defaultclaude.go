// Package defaultclaude handles reading the default user Claude Code
// configuration tree (~/.claude) for import into ccpm profiles and for
// drift detection against a stored fingerprint.
package defaultclaude

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/filetree"
	"github.com/nitin-1926/ccpm/internal/manifest"
	"github.com/nitin-1926/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/ccpm/internal/share"
)

// Target names the subtrees ccpm knows how to import from ~/.claude.
type Target string

const (
	TargetSkills   Target = "skills"
	TargetCommands Target = "commands"
	TargetRules    Target = "rules"
	TargetHooks    Target = "hooks"
	TargetAgents   Target = "agents"
	TargetSettings Target = "settings"
	TargetMCP      Target = "mcp"
	TargetPlugins  Target = "plugins"
)

// MCP scope values for ImportOptions.MCPScope.
const (
	MCPImportScopeGlobal  = "global"
	MCPImportScopeProfile = "profile"
)

// DefaultTargets returns the safe, opinionated default set (excludes plugins).
func DefaultTargets() []Target {
	return []Target{
		TargetSkills,
		TargetCommands,
		TargetRules,
		TargetHooks,
		TargetAgents,
		TargetSettings,
		TargetMCP,
	}
}

// AllTargets includes plugins; callers must opt in explicitly.
func AllTargets() []Target {
	return append(DefaultTargets(), TargetPlugins)
}

// ParseTargets converts a comma-separated list (e.g. from --only) into Target values.
// Empty input returns DefaultTargets().
func ParseTargets(values []string) ([]Target, error) {
	if len(values) == 0 {
		return DefaultTargets(), nil
	}
	known := map[string]Target{
		"skills":   TargetSkills,
		"commands": TargetCommands,
		"rules":    TargetRules,
		"hooks":    TargetHooks,
		"agents":   TargetAgents,
		"settings": TargetSettings,
		"mcp":      TargetMCP,
		"plugins":  TargetPlugins,
	}
	result := make([]Target, 0, len(values))
	for _, v := range values {
		t, ok := known[v]
		if !ok {
			return nil, fmt.Errorf("unknown target %q (valid: skills, commands, rules, hooks, agents, settings, mcp, plugins)", v)
		}
		result = append(result, t)
	}
	return result, nil
}

// DefaultDir returns ~/.claude — the default user config tree when
// CLAUDE_CONFIG_DIR is not set.
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

// Exists reports whether the default tree is present.
func Exists() bool {
	d, err := DefaultDir()
	if err != nil {
		return false
	}
	info, err := os.Stat(d)
	return err == nil && info.IsDir()
}

// targetPath returns the path of a target inside a config tree.
// For TargetSettings this is "settings.json"; others are directories.
func targetPath(root string, t Target) string {
	switch t {
	case TargetSettings:
		return filepath.Join(root, "settings.json")
	default:
		return filepath.Join(root, string(t))
	}
}

// ImportOptions controls the Import behavior.
type ImportOptions struct {
	Targets []Target
	DryRun  bool
	Force   bool // allow overwriting existing files in the profile (other than settings.json, which always merges)
	// Dedupe controls whether directory targets like skills, agents, and
	// commands go through ~/.ccpm/share/<kind>/<name> with a symlink back
	// into the profile. Enabling this means importing the same asset into
	// two profiles stores the data exactly once.
	Dedupe bool
	// ProfileName is used only for manifest tracking when Dedupe is true.
	// Leave empty to skip manifest updates.
	ProfileName string
	// LiveSymlinks, when used with Dedupe on skills/agents/commands, makes each
	// top-level entry that is a symlink-to-directory become a symlink in the
	// share store (to the resolved absolute path) instead of a file copy, so
	// edits in the original tree are visible through profiles without re-import.
	LiveSymlinks bool
	// ItemFilter restricts imports within a given target to a specific set of
	// top-level entry names. A nil or missing map for a target means "import
	// everything under this target". Keys:
	//   - directory targets (skills/agents/commands/hooks/rules): the top-level
	//     entry name as it appears in ~/.claude/<target>/.
	//   - TargetMCP: the MCPEntry.ID() value (name for user-scope, "name@project"
	//     for project-scope entries).
	ItemFilter map[Target]map[string]bool
	// MCPScope controls where imported MCP servers land:
	//   "global"  -> ~/.ccpm/share/mcp/global.json   (all profiles see them)
	//   "profile" -> ~/.ccpm/share/mcp/<profile>.json (only ProfileName sees them)
	// Empty string defaults to "profile".
	MCPScope string
}

// dedupableTargets is the set of directory targets we can safely symlink
// from a shared store. Plugins remain excluded because their on-disk tree
// is managed by Claude Code itself (~/.claude/plugins/) and ccpm only
// controls activation via the enabledPlugins settings key.
func dedupableTargets() map[Target]bool {
	return map[Target]bool{
		TargetSkills:   true,
		TargetAgents:   true,
		TargetCommands: true,
		TargetRules:    true,
		TargetHooks:    true,
	}
}

// targetToKind maps a dedupable Target to the manifest AssetKind used when
// recording an install. Returns ok=false for targets that aren't dedupable
// or that ccpm doesn't track in the manifest.
func targetToKind(t Target) (manifest.AssetKind, bool) {
	switch t {
	case TargetSkills:
		return manifest.KindSkill, true
	case TargetAgents:
		return manifest.KindAgent, true
	case TargetCommands:
		return manifest.KindCommand, true
	case TargetRules:
		return manifest.KindRule, true
	case TargetHooks:
		return manifest.KindHook, true
	default:
		return "", false
	}
}

// ImportPlan summarizes what Import will do for a single profile.
type ImportPlan struct {
	Profile string
	Actions []ImportAction
}

type ImportAction struct {
	Target     Target
	SourcePath string
	TargetPath string
	Kind       string // "copy", "skip-missing", "merge-settings", "skip-exists"
	Note       string
}

// Import copies the selected subtrees from ~/.claude into the profile directory.
// For settings.json it performs a key-level merge via callers using settingsmerge;
// here we only copy the raw file into a staging path unless opts.Force, and the
// caller is expected to merge separately. Returns a plan of what was (or would be)
// done.
func Import(profileDir string, opts ImportOptions) (*ImportPlan, error) {
	src, err := DefaultDir()
	if err != nil {
		return nil, err
	}
	if !Exists() {
		return nil, fmt.Errorf("default Claude config not found at %s", src)
	}

	plan := &ImportPlan{Profile: filepath.Base(profileDir)}

	for _, t := range opts.Targets {
		if t == TargetMCP {
			if err := importMCP(plan, opts); err != nil {
				return plan, err
			}
			continue
		}

		srcPath := targetPath(src, t)
		dstPath := targetPath(profileDir, t)

		info, err := os.Stat(srcPath)
		if os.IsNotExist(err) {
			plan.Actions = append(plan.Actions, ImportAction{
				Target:     t,
				SourcePath: srcPath,
				Kind:       "skip-missing",
				Note:       "source does not exist",
			})
			continue
		}
		if err != nil {
			return plan, fmt.Errorf("stat %s: %w", srcPath, err)
		}

		if t == TargetSettings {
			plan.Actions = append(plan.Actions, ImportAction{
				Target:     t,
				SourcePath: srcPath,
				TargetPath: dstPath,
				Kind:       "merge-settings",
				Note:       "caller should deep-merge into profile settings.json",
			})
			if !opts.DryRun {
				if err := copyFile(srcPath, dstPath+".ccpm-import"); err != nil {
					return plan, fmt.Errorf("staging settings import: %w", err)
				}
			}
			continue
		}

		// Directory target.
		if info.IsDir() {
			allow := opts.ItemFilter[t]
			if opts.Dedupe && dedupableTargets()[t] {
				if !opts.DryRun {
					if err := importDirDeduped(srcPath, dstPath, t, opts, allow); err != nil {
						return plan, fmt.Errorf("dedup-importing %s: %w", srcPath, err)
					}
				}
				plan.Actions = append(plan.Actions, ImportAction{
					Target:     t,
					SourcePath: srcPath,
					TargetPath: dstPath,
					Kind:       "link",
					Note:       filterNote(allow, "deduped via ~/.ccpm/share/"+string(t)),
				})
				continue
			}
			if !opts.DryRun {
				if err := copyDirFiltered(srcPath, dstPath, opts.Force, allow); err != nil {
					return plan, fmt.Errorf("copying %s: %w", srcPath, err)
				}
			}
			plan.Actions = append(plan.Actions, ImportAction{
				Target:     t,
				SourcePath: srcPath,
				TargetPath: dstPath,
				Kind:       "copy",
				Note:       filterNote(allow, ""),
			})
			continue
		}

		// File target that is not settings.json (e.g. some single-file item).
		if !opts.DryRun {
			if err := copyFile(srcPath, dstPath); err != nil {
				return plan, fmt.Errorf("copying %s: %w", srcPath, err)
			}
		}
		plan.Actions = append(plan.Actions, ImportAction{
			Target:     t,
			SourcePath: srcPath,
			TargetPath: dstPath,
			Kind:       "copy",
		})
	}

	return plan, nil
}

// filterNote annotates a plan action when a non-nil filter was applied so it's
// clear to the user that only a subset was imported.
func filterNote(allow map[string]bool, base string) string {
	if allow == nil {
		return base
	}
	note := fmt.Sprintf("filtered (%d selected)", len(allow))
	if base == "" {
		return note
	}
	return base + "; " + note
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), config.DirPerm); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// importDirDeduped materializes each top-level entry of srcDir into the
// shared store (~/.ccpm/share/<target>/<name>) and replaces the profile's
// copy with a symlink back to the store. Existing shared entries are left
// alone unless opts.Force is set. The manifest is updated so that later
// `ccpm sync` / `ccpm doctor` calls know which installs are profile-scoped.
// If allow is non-nil, only top-level entries whose name is in the set are
// processed; unselected entries are silently skipped.
func importDirDeduped(srcDir, dstProfileDir string, t Target, opts ImportOptions, allow map[string]bool) error {
	shareBase, err := share.Dir()
	if err != nil {
		return err
	}
	storeDir := filepath.Join(shareBase, string(t))
	if err := os.MkdirAll(storeDir, config.DirPerm); err != nil {
		return err
	}
	if err := os.MkdirAll(dstProfileDir, config.DirPerm); err != nil {
		return err
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	kind, kindOK := targetToKind(t)
	var m *manifest.Manifest
	if opts.ProfileName != "" && kindOK {
		m, err = manifest.Load()
		if err != nil {
			return fmt.Errorf("loading manifest: %w", err)
		}
	}

	for _, entry := range entries {
		name := entry.Name()
		if allow != nil && !allow[name] {
			continue
		}
		srcPath := filepath.Join(srcDir, name)
		storePath := filepath.Join(storeDir, name)
		linkPath := filepath.Join(dstProfileDir, name)

		// Seed the store only if absent or force is set.
		storeExists := false
		if _, err := os.Stat(storePath); err == nil {
			storeExists = true
		}
		if !storeExists || opts.Force {
			if _, err := filetree.SeedStoreEntry(srcPath, storePath, opts.LiveSymlinks); err != nil {
				return err
			}
		}

		if err := share.Link(storePath, linkPath); err != nil {
			return fmt.Errorf("linking %s: %w", linkPath, err)
		}

		if m != nil {
			inst := m.Find(name, kind)
			if inst == nil {
				m.Add(manifest.Install{
					ID:       name,
					Kind:     kind,
					Scope:    manifest.ScopeProfile,
					Source:   "default:" + srcPath,
					Profiles: []string{opts.ProfileName},
				})
			} else if !containsString(inst.Profiles, opts.ProfileName) {
				inst.Profiles = append(inst.Profiles, opts.ProfileName)
			}
		}
	}

	if m != nil {
		if err := manifest.Save(m); err != nil {
			return fmt.Errorf("saving manifest: %w", err)
		}
	}
	return nil
}

func containsString(xs []string, target string) bool {
	for _, x := range xs {
		if x == target {
			return true
		}
	}
	return false
}

// copyTreeMerging walks src and copies files into dst. Existing files are
