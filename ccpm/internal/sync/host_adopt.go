package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/defaultclaude"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/manifest"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
)

// hostKindSpec is the canonical kind→subdirectory mapping, owned by the
// manifest package (the same names apply under ~/.claude/ and profile dirs).
// kindDirs in sync.go stays separate because it additionally resolves the
// share-store root — only the subdir names are shared.
var hostKindSpec = manifest.KindPlural

// hostEntry is one item discovered in ~/.claude/<asset>/ that is not yet
// represented in the ccpm manifest. The scanner returns these so callers can
// log them, register them, and link them into profiles.
type hostEntry struct {
	Kind manifest.AssetKind
	Name string // top-level entry name as it appears in ~/.claude/<plural>/
	Src  string // absolute path inside ~/.claude/<plural>/<name>
}

// scanHostUnadopted walks ~/.claude/<plural>/ for every dedupable kind and
// returns entries that are not already present in m as KindSkill/Agent/...
// regardless of scope. Anything already registered (by ccpm add, by import,
// or by a prior auto-adopt) is skipped so the function is idempotent.
//
// A nil or empty result means "nothing to adopt" — no error path.
func scanHostUnadopted(m *manifest.Manifest) ([]hostEntry, error) {
	hostRoot, err := defaultclaude.DefaultDir()
	if err != nil {
		return nil, err
	}
	if !defaultclaude.Exists() {
		return nil, nil
	}

	// Build a fast lookup of (kind, name) → already in manifest. We compare
	// by exact name match: the manifest stores logical IDs (no extension for
	// directory assets), and the host top-level entries are themselves
	// directory or file names, so an exact-string match is the right key.
	registered := make(map[string]struct{}, len(m.Installs))
	for _, inst := range m.Installs {
		registered[manifestKey(inst.Kind, inst.ID)] = struct{}{}
	}

	kinds := sortedKinds(hostKindSpec)
	var out []hostEntry
	for _, k := range kinds {
		dir := filepath.Join(hostRoot, hostKindSpec[k])
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			// A single unreadable host dir (EACCES after a stray sudo, a
			// mount hiccup) must not abort the whole cascade — warn and keep
			// scanning the other kinds.
			fmt.Fprintf(os.Stderr, "Warning: skipping host scan of %s: %v\n", dir, err)
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if name == "" || strings.HasPrefix(name, ".") {
				// Hidden files (.DS_Store, .gitkeep) are noise, never assets.
				continue
			}
			if _, ok := registered[manifestKey(k, name)]; ok {
				continue
			}
			out = append(out, hostEntry{
				Kind: k,
				Name: name,
				Src:  filepath.Join(dir, name),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// adoptHostEntries registers each entry in the manifest as ScopeHost and
// links it into the given profile directory. Symlinks point directly at the
// host path (`~/.claude/<plural>/<name>`) — we deliberately do not route
// host entries through the share store, because the host already is the
// canonical location and a copy under ~/.ccpm/share would (a) double the
// disk footprint and (b) drift from the host whenever an external tool
// updates the original.
//
// The manifest write happens once at the end after every link succeeds, so
// a partial failure does not leave the manifest claiming entries that were
// never linked.
//
// Returns whether the manifest was mutated (new entry added OR an existing
// entry's Profiles list appended). Callers must persist when mutated is true —
// deciding from len(entries) alone loses Profiles-list appends (M16).
func adoptHostEntries(profileDir, profileName string, entries []hostEntry, m *manifest.Manifest) (mutated bool, err error) {
	if len(entries) == 0 {
		return false, nil
	}
	for _, e := range entries {
		subdir, ok := hostKindSpec[e.Kind]
		if !ok {
			continue
		}
		dst := filepath.Join(profileDir, subdir, e.Name)
		if err := share.Link(e.Src, dst); err != nil {
			return mutated, fmt.Errorf("linking host %s %q into %q: %w", e.Kind, e.Name, profileName, err)
		}

		// Only register on first adopt — subsequent profiles re-link the
		// same manifest entry through linkCascadeEntry below.
		if existing := m.Find(e.Name, e.Kind); existing == nil {
			m.Add(manifest.Install{
				ID:       e.Name,
				Kind:     e.Kind,
				Scope:    manifest.ScopeHost,
				Source:   "host:" + e.Src,
				Profiles: []string{profileName},
			})
			mutated = true
		} else {
			if !containsProfile(existing.Profiles, profileName) {
				existing.Profiles = append(existing.Profiles, profileName)
				mutated = true
			}
		}
	}
	return mutated, nil
}

// linkHostEntry re-links a single host-scoped manifest entry into a profile.
// Used by ApplyGlobals's cascade loop so a host entry that was already
// adopted by profile A automatically gets a symlink in newly-created profile
// B without re-scanning ~/.claude.
func linkHostEntry(profileDir string, inst manifest.Install) error {
	subdir, ok := hostKindSpec[inst.Kind]
	if !ok {
		return nil
	}
	src := strings.TrimPrefix(inst.Source, "host:")
	if src == "" {
		return fmt.Errorf("host install %q has no source path", inst.ID)
	}
	if _, err := os.Stat(src); os.IsNotExist(err) {
		// Source vanished from ~/.claude (user deleted it). Skip silently;
		// the doctor host-assets section will surface this as a stale entry.
		return nil
	}
	dst := filepath.Join(profileDir, subdir, inst.ID)
	return share.Link(src, dst)
}

func manifestKey(kind manifest.AssetKind, id string) string {
	return string(kind) + "\x00" + id
}

func sortedKinds(m map[manifest.AssetKind]string) []manifest.AssetKind {
	out := make([]manifest.AssetKind, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func containsProfile(xs []string, target string) bool {
	for _, x := range xs {
		if x == target {
			return true
		}
	}
	return false
}

// adoptionWarningOnce makes sure each (profile, scan-result) combination
// emits a stderr summary at most once per process. Repeated `ccpm run`
// invocations within the same long-lived process (rare but possible in
// integration tests) would otherwise spam.
var adoptionWarningOnce sync.Map // key: profileName + ":" + count

// reportAdoption prints a compact stderr line summarizing a successful
// auto-adopt, broken down by kind. Empty entries skip the print.
func reportAdoption(profileName string, entries []hostEntry) {
	if len(entries) == 0 {
		return
	}
	key := fmt.Sprintf("%s:%d:%d", profileName, len(entries), time.Now().Unix()/60)
	if _, loaded := adoptionWarningOnce.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	counts := map[manifest.AssetKind]int{}
	for _, e := range entries {
		counts[e.Kind]++
	}
	parts := make([]string, 0, len(counts))
	for _, k := range sortedKinds(hostKindSpec) {
		if n := counts[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d", string(k)+"s", n))
		}
	}
	fmt.Fprintf(os.Stderr,
		"ccpm: adopted %d items from ~/.claude into profile %q (%s) — disable with `ccpm config set cascade_auto_adopt false`\n",
		len(entries), profileName, strings.Join(parts, " "),
	)
}

