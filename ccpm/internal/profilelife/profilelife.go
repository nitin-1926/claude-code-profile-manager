// Package profilelife keeps a profile's side stores in step with its
// directory. Besides <profiles>/<name>/, a profile owns:
//
//   - share/settings/<name>.json        settings fragment
//   - share/settings/<name>.owned.json  owned-keys sidecar
//   - share/mcp/<name>.json             MCP fragment (may hold env tokens)
//   - its name in installs.json         Install.Profiles lists
//
// All of these are keyed by NAME, not by directory, so a command that deletes
// or renames only the directory leaves them behind — and a later profile with
// the same name silently inherits the old one's MCP servers and settings.
// Remove, Rename and Copy change every store in one atomicwrite transaction
// (AGENTS.md invariant #9). Callers hold the config lock.
package profilelife

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/atomicwrite"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/defaultclaude"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/manifest"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
)

// globalMCPName is the global MCP fragment's basename (share/mcp/global.json).
// A profile named "global" shares that path, so its MCP fragment is never
// moved or deleted — doing so would take every global MCP server with it.
const globalMCPName = "global"

type fragments struct {
	settings, owned, mcp string // mcp is "" for a profile named "global"
}

func fragmentsFor(name string) (fragments, error) {
	settingsDir, err := share.SettingsDir()
	if err != nil {
		return fragments{}, err
	}
	mcpDir, err := share.MCPDir()
	if err != nil {
		return fragments{}, err
	}
	f := fragments{
		settings: filepath.Join(settingsDir, name+".json"),
		owned:    filepath.Join(settingsDir, name+".owned.json"),
	}
	if name != globalMCPName {
		f.mcp = filepath.Join(mcpDir, name+".json")
	}
	return f, nil
}

func (f fragments) all() []string {
	out := []string{f.settings, f.owned}
	if f.mcp != "" {
		out = append(out, f.mcp)
	}
	return out
}

// Remove deletes name's fragments and drops name from every manifest entry.
// A profile-scoped entry left with no profiles is removed; the shared store
// entry it pointed at is kept, matching `ccpm <asset> remove --profile`.
func Remove(name string) error {
	f, err := fragmentsFor(name)
	if err != nil {
		return err
	}
	var changes []atomicwrite.FileChange
	for _, p := range f.all() {
		changes = append(changes, atomicwrite.DeleteFile(p))
	}
	mc, err := editManifest(func(m *manifest.Manifest) { dropProfile(m, name) })
	if err != nil {
		return err
	}
	return atomicwrite.Apply(append(changes, mc...))
}

// Rename moves oldName's fragments to newName and rewrites manifest refs. Any
// fragment already sitting under newName (left by an older ccpm's remove) is
// replaced or deleted, so the renamed profile never inherits it.
func Rename(oldName, newName string) error {
	from, err := fragmentsFor(oldName)
	if err != nil {
		return err
	}
	to, err := fragmentsFor(newName)
	if err != nil {
		return err
	}
	var changes []atomicwrite.FileChange
	for _, pair := range [][2]string{{from.settings, to.settings}, {from.owned, to.owned}, {from.mcp, to.mcp}} {
		src, dst := pair[0], pair[1]
		if src == "" || dst == "" {
			continue // "global" MCP fragment: never moved
		}
		data, err := os.ReadFile(src)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			changes = append(changes, atomicwrite.DeleteFile(dst))
		case err != nil:
			return err
		default:
			changes = append(changes, atomicwrite.WriteFile(dst, data, config.FilePerm), atomicwrite.DeleteFile(src))
		}
	}
	mc, err := editManifest(func(m *manifest.Manifest) {
		for i := range m.Installs {
			p := m.Installs[i].Profiles
			if idx := slices.Index(p, oldName); idx >= 0 {
				if slices.Contains(p, newName) {
					m.Installs[i].Profiles = slices.Delete(p, idx, idx+1)
				} else {
					p[idx] = newName
				}
			}
		}
	})
	if err != nil {
		return err
	}
	return atomicwrite.Apply(append(changes, mc...))
}

// Copy gives dst the stores src holds for the selected targets: the settings
// fragment + owned keys (TargetSettings), the MCP fragment (TargetMCP), and
// membership in src's profile-scoped manifest entries of each copied kind.
// Where dst already has a value, dst wins. fresh=true is for a brand-new dst
// (clone, add): anything already under dst's name is stale and discarded
// instead of merged.
func Copy(src, dst string, targets []defaultclaude.Target, fresh bool) error {
	from, err := fragmentsFor(src)
	if err != nil {
		return err
	}
	to, err := fragmentsFor(dst)
	if err != nil {
		return err
	}
	if fresh {
		if err := Remove(dst); err != nil {
			return err
		}
	}

	var changes []atomicwrite.FileChange
	kinds := map[manifest.AssetKind]bool{}
	for _, t := range targets {
		switch t {
		case defaultclaude.TargetSettings:
			c, err := mergeJSONFile(from.settings, to.settings, settingsmerge.DeepMerge)
			if err != nil {
				return err
			}
			changes = append(changes, c...)
			c, err = mergeOwned(from.settings, to.settings, to.owned)
			if err != nil {
				return err
			}
			changes = append(changes, c...)
		case defaultclaude.TargetMCP:
			kinds[manifest.KindMCP] = true
			if from.mcp == "" || to.mcp == "" {
				continue
			}
			c, err := mergeJSONFile(from.mcp, to.mcp, addMissing)
			if err != nil {
				return err
			}
			changes = append(changes, c...)
		default:
			for kind, plural := range manifest.KindPlural {
				if plural == string(t) {
					kinds[kind] = true
				}
			}
		}
	}
	mc, err := editManifest(func(m *manifest.Manifest) {
		for i, inst := range m.Installs {
			if inst.Scope == manifest.ScopeProfile && kinds[inst.Kind] &&
				slices.Contains(inst.Profiles, src) && !slices.Contains(inst.Profiles, dst) {
				m.Installs[i].Profiles = append(inst.Profiles, dst)
			}
		}
	})
	if err != nil {
		return err
	}
	return atomicwrite.Apply(append(changes, mc...))
}

// CopyLinks recreates under dst every symlink in src whose target lies outside
// src — the shared/host assets filetree.CopyTreeSkipEscaping skips, including
// profile-scoped skills that nothing else would re-link. The link itself is
// reproduced (same target) and never followed, so nothing outside the profile
// is read. Dangling links are skipped; existing dst entries are kept unless
// overwrite.
func CopyLinks(src, dst string, overwrite bool) error {
	root, err := filepath.EvalSymlinks(src)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink == 0 || path == src {
			return nil
		}
		target, err := os.Readlink(path)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		resolved, err := filepath.EvalSymlinks(target)
		if err != nil {
			return nil // dangling: skip, as CopyTreeSkipEscaping does
		}
		if rel, err := filepath.Rel(root, resolved); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil // inside src: CopyTree already copied it
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		if _, err := os.Lstat(out); err == nil && !overwrite {
			return nil
		}
		return share.Link(target, out)
	})
}

// mergeJSONFile returns the change that folds src's JSON object into dst's
// via merge(dstBase, overlay). Absent/empty src → no change.
func mergeJSONFile(src, dst string, merge func(base, overlay map[string]interface{}) map[string]interface{}) ([]atomicwrite.FileChange, error) {
	srcData, err := settingsmerge.LoadJSON(src)
	if err != nil || len(srcData) == 0 {
		return nil, err
	}
	dstData, err := settingsmerge.LoadJSON(dst)
	if err != nil {
		return nil, err
	}
	// dst wins: src is the base, dst the overlay.
	out, err := marshal(merge(srcData, dstData))
	if err != nil {
		return nil, err
	}
	return []atomicwrite.FileChange{atomicwrite.WriteFile(dst, out, config.FilePerm)}, nil
}

// addMissing keeps every overlay entry and adds base entries it lacks — a
// per-server union for MCP fragments, where mixing two servers' fields (what
// DeepMerge would do) yields a config neither side wrote.
func addMissing(base, overlay map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

func mergeOwned(srcFrag, dstFrag, dstOwned string) ([]atomicwrite.FileChange, error) {
	srcKeys, err := settingsmerge.LoadOwnedKeys(srcFrag)
	if err != nil || len(srcKeys) == 0 {
		return nil, err
	}
	dstKeys, err := settingsmerge.LoadOwnedKeys(dstFrag)
	if err != nil {
		return nil, err
	}
	for k := range srcKeys {
		dstKeys[k] = struct{}{}
	}
	keys := make([]string, 0, len(dstKeys))
	for k := range dstKeys {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out, err := marshal(settingsmerge.OwnedKeysFile{Keys: keys})
	if err != nil {
		return nil, err
	}
	return []atomicwrite.FileChange{atomicwrite.WriteFile(dstOwned, out, config.FilePerm)}, nil
}

// editManifest applies fn to the manifest and returns the write change, or
// nothing when fn left it unchanged.
func editManifest(fn func(*manifest.Manifest)) ([]atomicwrite.FileChange, error) {
	m, err := manifest.Load()
	if err != nil {
		return nil, err
	}
	before, err := manifest.MarshalBytes(m)
	if err != nil {
		return nil, err
	}
	fn(m)
	after, err := manifest.MarshalBytes(m)
	if err != nil || bytes.Equal(before, after) {
		return nil, err
	}
	path, err := manifest.Path()
	if err != nil {
		return nil, err
	}
	return []atomicwrite.FileChange{atomicwrite.WriteFile(path, append(after, '\n'), config.FilePerm)}, nil
}

func dropProfile(m *manifest.Manifest, name string) {
	kept := m.Installs[:0]
	for _, inst := range m.Installs {
		if i := slices.Index(inst.Profiles, name); i >= 0 {
			inst.Profiles = slices.Delete(inst.Profiles, i, i+1)
			if inst.Scope == manifest.ScopeProfile && len(inst.Profiles) == 0 {
				continue
			}
		}
		kept = append(kept, inst)
	}
	m.Installs = kept
}

func marshal(v interface{}) ([]byte, error) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
