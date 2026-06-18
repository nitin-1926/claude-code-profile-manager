package defaultclaude

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/manifest"
)

// ShadowedAsset describes one (kind, name) that exists in both ~/.claude and
// a profile-local override. The cascade always lets profile-local win, but
// the doctor surfaces these so users aren't surprised when a host update
// silently fails to take effect inside a profile.
type ShadowedAsset struct {
	Kind    string
	Name    string
	Profile string
}

// HostCascadeSummary walks ~/.claude/<asset>/ and every profile's matching
// subdir, returning (a) a one-line summary string of host vs. adopted counts
// per kind, and (b) a list of shadow entries. Moved out of cmd/doctor so the
// walking/counting logic lives next to the other host-state scanners (H10).
func HostCascadeSummary(profileDirs map[string]string, m *manifest.Manifest) (string, []ShadowedAsset) {
	hostRoot, err := DefaultDir()
	if err != nil {
		return "", nil
	}

	// Index the manifest by (kind, id) so we can tell adopted from
	// not-yet-adopted host entries.
	adopted := map[string]bool{}
	if m != nil {
		for _, inst := range m.Installs {
			if inst.Scope == manifest.ScopeHost {
				adopted[string(inst.Kind)+"/"+inst.ID] = true
			}
		}
	}

	type counts struct {
		host    int
		adopted int
	}
	totals := map[manifest.AssetKind]counts{}
	var shadows []ShadowedAsset

	for _, kind := range manifest.DedupableKindsOrdered() {
		plural := manifest.KindPlural[kind]
		hostDir := filepath.Join(hostRoot, plural)
		hostNames := listDirNames(hostDir)
		hostSet := map[string]bool{}
		for _, n := range hostNames {
			if strings.HasPrefix(n, ".") {
				continue
			}
			hostSet[n] = true
			c := totals[kind]
			c.host++
			if adopted[string(kind)+"/"+n] {
				c.adopted++
			}
			totals[kind] = c
		}

		// Per-profile shadowing: a profile entry with the same name as a
		// host entry, but the profile entry was added directly (not via
		// adoption) — we detect this by looking at the manifest scope.
		for prof, profDir := range profileDirs {
			profNames := listDirNames(filepath.Join(profDir, plural))
			for _, n := range profNames {
				if !hostSet[n] {
					continue
				}
				// Same name in host and profile. If the manifest entry
				// for (kind, name) is profile-scoped, it's a shadow.
				if inst := m.Find(n, kind); inst != nil && inst.Scope == manifest.ScopeProfile {
					shadows = append(shadows, ShadowedAsset{
						Kind: string(kind), Name: n, Profile: prof,
					})
				}
			}
		}
	}

	var summary []string
	for _, kind := range manifest.DedupableKindsOrdered() {
		c := totals[kind]
		if c.host == 0 {
			continue
		}
		summary = append(summary, fmt.Sprintf("%s %d/%d adopted", manifest.KindPlural[kind], c.adopted, c.host))
	}
	if len(summary) == 0 {
		return "", shadows
	}
	return strings.Join(summary, ", "), shadows
}

func listDirNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}
