package defaultclaude

import (
	"os"
	"path/filepath"
	"sort"
)

// ProfileDiff reports, for a given target (skills, commands, hooks, agents,
// rules), which top-level items are only in ~/.claude, only in profiles, and
// which are shared.
type ProfileDiff struct {
	Target           Target
	OnlyInDefault    []string          // items present in ~/.claude but no profile has them
	OnlyInProfiles   map[string][]string // profile name -> items only in that profile
	SharedAdopted    []string            // items present in default AND at least one profile
}

// ComputeProfileDiffs walks ~/.claude and each profileDir for every target in
// `targets` and returns a per-target diff. For file-only targets (settings)
// it is skipped — that doesn't have meaningful "items".
func ComputeProfileDiffs(defaultRoot string, profileDirs map[string]string, targets []Target) []ProfileDiff {
	var out []ProfileDiff
	for _, t := range targets {
		if t == TargetSettings {
			continue
		}
		out = append(out, diffTarget(defaultRoot, profileDirs, t))
	}
	return out
}

func diffTarget(defaultRoot string, profileDirs map[string]string, t Target) ProfileDiff {
	diff := ProfileDiff{
		Target:         t,
		OnlyInProfiles: map[string][]string{},
	}

	defaultItems := listTopLevel(filepath.Join(defaultRoot, string(t)))
	profileItemSets := map[string]map[string]struct{}{}
	for name, dir := range profileDirs {
		items := listTopLevel(filepath.Join(dir, string(t)))
		set := map[string]struct{}{}
		for _, it := range items {
			set[it] = struct{}{}
		}
		profileItemSets[name] = set
	}

	adoptedUnion := map[string]struct{}{}
	for _, set := range profileItemSets {
		for k := range set {
			adoptedUnion[k] = struct{}{}
		}
	}

	defaultSet := map[string]struct{}{}
	for _, it := range defaultItems {
		defaultSet[it] = struct{}{}
		if _, ok := adoptedUnion[it]; ok {
			diff.SharedAdopted = append(diff.SharedAdopted, it)
		} else {
			diff.OnlyInDefault = append(diff.OnlyInDefault, it)
		}
	}

	for name, set := range profileItemSets {
		var extras []string
		for item := range set {
			if _, ok := defaultSet[item]; !ok {
				extras = append(extras, item)
			}
		}
		if len(extras) > 0 {
			sort.Strings(extras)
			diff.OnlyInProfiles[name] = extras
		}
	}

	sort.Strings(diff.OnlyInDefault)
	sort.Strings(diff.SharedAdopted)
	return diff
}

// listTopLevel returns the names of top-level entries (files or dirs) inside
// `dir`. Missing dirs return an empty slice, not an error.
func listTopLevel(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// HasUnadopted returns true when any target has items present in ~/.claude
// that no profile has imported yet.
func HasUnadopted(diffs []ProfileDiff) bool {
	for _, d := range diffs {
		if len(d.OnlyInDefault) > 0 {
			return true
		}
	}
	return false
}
