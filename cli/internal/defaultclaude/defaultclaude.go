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
// preserved unless force=true.
func copyTreeMerging(src, dst string, force bool) error {
	return filetree.CopyTree(src, dst, !force)
}

// copyDirFiltered copies top-level entries of srcDir into dstDir. When allow
// is nil every entry is copied (equivalent to copyTreeMerging). When allow is
// set only matching top-level entry names are copied; each selected entry is
// walked recursively with the same preserve-existing-unless-force semantics as
// copyTreeMerging.
func copyDirFiltered(srcDir, dstDir string, force bool, allow map[string]bool) error {
	if allow == nil {
		return copyTreeMerging(srcDir, dstDir, force)
	}
	if err := os.MkdirAll(dstDir, config.DirPerm); err != nil {
		return err
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !allow[name] {
			continue
		}
		srcPath := filepath.Join(srcDir, name)
		dstPath := filepath.Join(dstDir, name)

		info, err := os.Stat(srcPath)
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := filetree.CopyTree(srcPath, dstPath, !force); err != nil {
				return err
			}
			continue
		}
		if !force {
			if _, err := os.Stat(dstPath); err == nil {
				continue
			}
		}
		if err := copyFile(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

// importMCP reads MCP server definitions from ~/.claude.json and writes the
// selected ones into a ccpm MCP fragment. Destination is determined by
// opts.MCPScope: "global" writes to ~/.ccpm/share/mcp/global.json; anything
// else falls back to the profile-scoped fragment for opts.ProfileName.
func importMCP(plan *ImportPlan, opts ImportOptions) error {
	if !ClaudeJSONExists() {
		path, _ := ClaudeJSONPath()
		plan.Actions = append(plan.Actions, ImportAction{
			Target:     TargetMCP,
			SourcePath: path,
			Kind:       "skip-missing",
			Note:       "~/.claude.json not found",
		})
		return nil
	}

	entries, err := LoadMCPEntries()
	if err != nil {
		return fmt.Errorf("reading MCP entries: %w", err)
	}
	allow := opts.ItemFilter[TargetMCP]

	scope := opts.MCPScope
	if scope == "" {
		scope = MCPImportScopeProfile
	}
	fragmentName := "global"
	if scope != MCPImportScopeGlobal {
		fragmentName = opts.ProfileName
		if fragmentName == "" {
			return fmt.Errorf("profile-scoped MCP import requires ProfileName")
		}
	}

	selected := make([]MCPEntry, 0, len(entries))
	for _, e := range entries {
		if allow != nil && !allow[e.ID()] {
			continue
		}
		selected = append(selected, e)
	}

	if len(selected) == 0 {
		plan.Actions = append(plan.Actions, ImportAction{
			Target: TargetMCP,
			Kind:   "skip-missing",
			Note:   "no MCP entries selected",
		})
		return nil
	}

	mcpDir, err := share.MCPDir()
	if err != nil {
		return err
	}
	fragPath := filepath.Join(mcpDir, fragmentName+".json")

	if !opts.DryRun {
		if err := share.EnsureDirs(); err != nil {
			return err
		}
		frag, err := settingsmerge.LoadJSON(fragPath)
		if err != nil {
			return fmt.Errorf("loading MCP fragment %s: %w", fragPath, err)
		}
		for _, e := range selected {
			if _, exists := frag[e.Name]; exists && !opts.Force {
				// Preserve existing definitions — matches the "don't clobber
				// without --force" rule used elsewhere in Import.
				continue
			}
			frag[e.Name] = e.Definition
		}
		if err := settingsmerge.WriteJSON(fragPath, frag); err != nil {
			return fmt.Errorf("writing MCP fragment %s: %w", fragPath, err)
		}

		if opts.ProfileName != "" {
			if err := recordMCPInstalls(selected, scope, opts.ProfileName); err != nil {
				return err
			}
		}
	}

	for _, e := range selected {
		plan.Actions = append(plan.Actions, ImportAction{
			Target:     TargetMCP,
			SourcePath: e.Source(),
			TargetPath: fragPath,
			Kind:       "mcp-add",
			Note:       fmt.Sprintf("%s → %s scope", e.Name, scope),
		})
	}
	return nil
}

// recordMCPInstalls updates the ccpm manifest so `ccpm mcp list` shows the
// imported servers alongside ones added via `ccpm mcp add`. Scope+Profiles are
// set to match where the fragment actually lives.
func recordMCPInstalls(entries []MCPEntry, scope, profileName string) error {
	m, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}
	for _, e := range entries {
		if existing := m.Find(e.Name, manifest.KindMCP); existing != nil {
			m.Remove(e.Name, manifest.KindMCP)
		}
		inst := manifest.Install{
			ID:     e.Name,
			Kind:   manifest.KindMCP,
			Source: e.Source(),
		}
		if scope == MCPImportScopeGlobal {
			inst.Scope = manifest.ScopeGlobal
		} else {
			inst.Scope = manifest.ScopeProfile
			inst.Profiles = []string{profileName}
		}
		m.Add(inst)
	}
	return manifest.Save(m)
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
	if err := os.MkdirAll(filepath.Dir(path), config.DirPerm); err != nil {
		return err
	}
	data, err := json.MarshalIndent(fp, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, config.FilePerm); err != nil {
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
			fp.Files[filepath.ToSlash(rel)] = hash
			continue
		}

		if err := hashWalk(root, sub, sub, fp.Files); err != nil {
			return nil, err
		}
	}

	return fp, nil
}

// hashWalk adds every regular file reachable from physDir to files, keyed by
// its logical path relative to root. Symlink-to-directory entries are
// transparently followed (filepath.Walk uses Lstat and would misclassify them
// as files, causing hashFile -> os.Open to fail with EISDIR — same bug class
// as was fixed in CopyTree on 2026-04-17).
//
// logDir tracks the logical path under root so the fingerprint keys stay
// pinned to ~/.claude/<...> even when the walk crosses into a resolved
// symlink target that lives elsewhere on disk.
func hashWalk(root, physDir, logDir string, files map[string]string) error {
	entries, err := os.ReadDir(physDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		physPath := filepath.Join(physDir, e.Name())
		logPath := filepath.Join(logDir, e.Name())

		info, err := os.Lstat(physPath)
		if err != nil {
			return err
		}

		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Stat(physPath)
			if err != nil {
				// Broken symlinks are silently skipped — they can't be
				// hashed and aren't a drift signal on their own.
				continue
			}
			if target.IsDir() {
				resolved, err := filepath.EvalSymlinks(physPath)
				if err != nil {
					return err
				}
				if err := hashWalk(root, resolved, logPath, files); err != nil {
					return err
				}
				continue
			}
			// Symlink to regular file: fall through to the file-hash path
			// below; os.Open will follow the link naturally.
		}

		if info.IsDir() {
			if err := hashWalk(root, physPath, logPath, files); err != nil {
				return err
			}
			continue
		}

		hash, err := hashFile(physPath)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, logPath)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = hash
	}
	return nil
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
