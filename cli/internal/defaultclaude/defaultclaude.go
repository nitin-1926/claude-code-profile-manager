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
	"github.com/nitin-1926/ccpm/internal/manifest"
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
	TargetPlugins  Target = "plugins"
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
		"plugins":  TargetPlugins,
	}
	result := make([]Target, 0, len(values))
	for _, v := range values {
		t, ok := known[v]
		if !ok {
			return nil, fmt.Errorf("unknown target %q (valid: skills, commands, rules, hooks, agents, settings, plugins)", v)
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
}

// dedupableTargets is the set of directory targets we can safely symlink
// from a shared store. Hooks are excluded because they're typically small
// and frequently edited per-profile; rules/plugins are excluded because
// their semantics vary by tool.
func dedupableTargets() map[Target]bool {
	return map[Target]bool{
		TargetSkills:   true,
		TargetAgents:   true,
		TargetCommands: true,
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
			if opts.Dedupe && dedupableTargets()[t] {
				if !opts.DryRun {
					if err := importDirDeduped(srcPath, dstPath, t, opts); err != nil {
						return plan, fmt.Errorf("dedup-importing %s: %w", srcPath, err)
					}
				}
				plan.Actions = append(plan.Actions, ImportAction{
					Target:     t,
					SourcePath: srcPath,
					TargetPath: dstPath,
					Kind:       "link",
					Note:       "deduped via ~/.ccpm/share/" + string(t),
				})
				continue
			}
			if !opts.DryRun {
				if err := copyTreeMerging(srcPath, dstPath, opts.Force); err != nil {
					return plan, fmt.Errorf("copying %s: %w", srcPath, err)
				}
			}
			plan.Actions = append(plan.Actions, ImportAction{
				Target:     t,
				SourcePath: srcPath,
				TargetPath: dstPath,
				Kind:       "copy",
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

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
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
func importDirDeduped(srcDir, dstProfileDir string, t Target, opts ImportOptions) error {
	shareBase, err := share.Dir()
	if err != nil {
		return err
	}
	storeDir := filepath.Join(shareBase, string(t))
	if err := os.MkdirAll(storeDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(dstProfileDir, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	var m *manifest.Manifest
	if opts.ProfileName != "" && t == TargetSkills {
		m, err = manifest.Load()
		if err != nil {
			return fmt.Errorf("loading manifest: %w", err)
		}
	}

	for _, entry := range entries {
		name := entry.Name()
		srcPath := filepath.Join(srcDir, name)
		storePath := filepath.Join(storeDir, name)
		linkPath := filepath.Join(dstProfileDir, name)

		// Seed the store only if absent or force is set.
		storeExists := false
		if _, err := os.Stat(storePath); err == nil {
			storeExists = true
		}
		if !storeExists || opts.Force {
			if err := os.RemoveAll(storePath); err != nil {
				return err
			}
			if entry.IsDir() {
				if err := copyTreeMerging(srcPath, storePath, true); err != nil {
					return err
				}
			} else {
				if err := copyFile(srcPath, storePath); err != nil {
					return err
				}
			}
		}

		if err := share.Link(storePath, linkPath); err != nil {
			return fmt.Errorf("linking %s: %w", linkPath, err)
		}

		if m != nil {
			inst := m.Find(name, manifest.KindSkill)
			if inst == nil {
				m.Add(manifest.Install{
					ID:       name,
					Kind:     manifest.KindSkill,
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
// preserved unless force=true.
func copyTreeMerging(src, dst string, force bool) error {
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

// Fingerprint is a compact snapshot of the default ~/.claude tree used
// for drift detection between ccpm runs and imports.
type Fingerprint struct {
	Version   string            `json:"version"`
	TakenAt   string            `json:"taken_at"`
	Root      string            `json:"root"`
	LastNudge string            `json:"last_nudge,omitempty"`
	Files     map[string]string `json:"files"` // relative path -> sha256 hex
}

const fingerprintVersion = "1"

func fingerprintPath() (string, error) {
	base, err := config.BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "default-claude-fingerprint.json"), nil
}

// LoadFingerprint returns the stored fingerprint or nil if none exists.
func LoadFingerprint() (*Fingerprint, error) {
	path, err := fingerprintPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading fingerprint: %w", err)
	}
	var fp Fingerprint
	if err := json.Unmarshal(data, &fp); err != nil {
		return nil, fmt.Errorf("parsing fingerprint: %w", err)
	}
	return &fp, nil
}

// SaveFingerprint writes the given fingerprint atomically.
func SaveFingerprint(fp *Fingerprint) error {
	path, err := fingerprintPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(fp, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Snapshot builds a fresh fingerprint of ~/.claude restricted to the given
// targets. Plugins are excluded from hashing (large/binary/noisy) unless
// explicitly requested via targets.
func Snapshot(targets []Target) (*Fingerprint, error) {
	root, err := DefaultDir()
	if err != nil {
		return nil, err
	}

	fp := &Fingerprint{
		Version: fingerprintVersion,
		TakenAt: time.Now().UTC().Format(time.RFC3339),
		Root:    root,
		Files:   map[string]string{},
	}

	if !Exists() {
		return fp, nil
	}

	for _, t := range targets {
		sub := targetPath(root, t)
		info, err := os.Stat(sub)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}

		if !info.IsDir() {
			hash, err := hashFile(sub)
			if err != nil {
				return nil, err
			}
			rel, _ := filepath.Rel(root, sub)
			fp.Files[rel] = hash
			continue
		}

		if err := filepath.Walk(sub, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			hash, err := hashFile(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			fp.Files[rel] = hash
			return nil
		}); err != nil {
			return nil, err
		}
	}

	return fp, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Drift describes the difference between two fingerprints.
type Drift struct {
	Added    []string
	Removed  []string
	Modified []string
}

// HasChanges returns true when any category is non-empty.
func (d Drift) HasChanges() bool {
	return len(d.Added) > 0 || len(d.Removed) > 0 || len(d.Modified) > 0
}

// Compare reports the set of changes between an old and new fingerprint.
// Either argument may be nil; a nil old fingerprint means "everything is new".
func Compare(oldFP, newFP *Fingerprint) Drift {
	d := Drift{}
	oldFiles := map[string]string{}
	if oldFP != nil {
		oldFiles = oldFP.Files
	}
	newFiles := map[string]string{}
	if newFP != nil {
		newFiles = newFP.Files
	}

	for k, newHash := range newFiles {
		oldHash, ok := oldFiles[k]
		if !ok {
			d.Added = append(d.Added, k)
			continue
		}
		if oldHash != newHash {
			d.Modified = append(d.Modified, k)
		}
	}
	for k := range oldFiles {
		if _, ok := newFiles[k]; !ok {
			d.Removed = append(d.Removed, k)
		}
	}

	sort.Strings(d.Added)
	sort.Strings(d.Removed)
	sort.Strings(d.Modified)
	return d
}

// ShouldNudge returns true if drift should be reported to the user now,
// taking the last-nudge timestamp into account. Nudges are debounced to
// once every `interval`.
func ShouldNudge(fp *Fingerprint, interval time.Duration) bool {
	if fp == nil || fp.LastNudge == "" {
		return true
	}
	last, err := time.Parse(time.RFC3339, fp.LastNudge)
	if err != nil {
		return true
	}
	return time.Since(last) >= interval
}

// MarkNudged updates the LastNudge timestamp on the stored fingerprint.
func MarkNudged() error {
	fp, err := LoadFingerprint()
	if err != nil {
		return err
	}
	if fp == nil {
		return nil
	}
	fp.LastNudge = time.Now().UTC().Format(time.RFC3339)
	return SaveFingerprint(fp)
}
