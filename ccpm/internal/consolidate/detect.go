package consolidate

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Detect inspects a snapshot and returns the issues found. The returned
// issues may carry a non-nil AutoFix when the repair is unambiguous and
// non-destructive (dangling symlink removal, stale plugin cache deletion).
// Other categories require user choice and are surfaced for the slash skill.
func Detect(snap Snapshot) []Issue {
	var issues []Issue
	issues = append(issues, detectDangling(snap)...)
	issues = append(issues, detectRealDirDuplicates(snap)...)
	issues = append(issues, detectGhostManifest(snap)...)
	issues = append(issues, detectBrokenEmptyFiles(snap)...)
	issues = append(issues, detectHookDuplication(snap)...)
	issues = append(issues, detectPermissionIntersection(snap)...)
	issues = append(issues, detectPluginScopeDrift(snap)...)
	issues = append(issues, detectMCPScopeDrift(snap)...)
	issues = append(issues, detectStalePluginCaches(snap)...)
	issues = append(issues, detectBudgetOverflow(snap)...)
	return issues
}

func detectDangling(snap Snapshot) []Issue {
	var out []Issue
	roots := []string{
		filepath.Join(snap.HostDir, "skills"),
		filepath.Join(snap.HostDir, "agents"),
		filepath.Join(snap.HostDir, "commands"),
	}
	if snap.CCPMPresent {
		roots = append(roots,
			filepath.Join(snap.CCPMDir, "profiles"),
			filepath.Join(snap.CCPMDir, "share", "skills"),
		)
	}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || d == nil {
				return nil
			}
			if d.Type()&os.ModeSymlink == 0 {
				return nil
			}
			if _, err := os.Stat(p); err != nil && os.IsNotExist(err) {
				target, _ := os.Readlink(p)
				link := p
				out = append(out, Issue{
					Category: "dangling",
					Severity: SevWarn,
					Scope:    filepath.Dir(p),
					Asset:    filepath.Base(p),
					Detail:   fmt.Sprintf("target %s missing", target),
					AutoFix: func() error {
						return os.Remove(link)
					},
				})
			}
			return nil
		})
	}
	return out
}

func detectRealDirDuplicates(snap Snapshot) []Issue {
	if !snap.CCPMPresent {
		return nil
	}
	scopes := map[string][]SkillEntry{
		"agents": snap.AgentSkills,
		"share":  snap.ShareSkills,
		"host":   snap.HostSkills,
	}
	// Skill name -> list of scopes where it lives as a real (non-symlink) dir
	realDirs := map[string][]string{}
	pathLookup := map[string]map[string]string{} // name -> scope -> path
	for scope, entries := range scopes {
		for _, e := range entries {
			if e.IsSymlink {
				continue
			}
			if !isDir(e.Path) {
				continue
			}
			realDirs[e.Name] = append(realDirs[e.Name], scope)
			if pathLookup[e.Name] == nil {
				pathLookup[e.Name] = map[string]string{}
			}
			pathLookup[e.Name][scope] = e.Path
		}
	}

	var out []Issue
	for name, scopeList := range realDirs {
		if len(scopeList) < 2 {
			continue
		}
		sort.Strings(scopeList)
		paths := pathLookup[name]
		// Compute content fingerprints to label identical vs DIVERGED
		fp := map[string]string{}
		for _, s := range scopeList {
			fp[s] = dirFingerprint(paths[s])
		}
		identical := true
		first := fp[scopeList[0]]
		for _, s := range scopeList[1:] {
			if fp[s] != first {
				identical = false
				break
			}
		}
		detail := "DIVERGED"
		if identical {
			detail = "identical"
		}
		out = append(out, Issue{
			Category: "duplicate",
			Severity: SevWarn,
			Scope:    joinScopes(scopeList),
			Asset:    name,
			Detail:   detail,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Asset < out[j].Asset })
	return out
}

func detectGhostManifest(snap Snapshot) []Issue {
	if !snap.CCPMPresent || snap.Manifest == nil {
		return nil
	}
	live := map[string]struct{}{}
	for _, p := range snap.LiveProfiles {
		live[p] = struct{}{}
	}
	ghosts := map[string]struct{}{}
	for _, p := range snap.ManifestProfiles {
		if _, ok := live[p]; !ok {
			ghosts[p] = struct{}{}
		}
	}
	if len(ghosts) == 0 {
		return nil
	}
	var out []Issue
	for _, install := range snap.Manifest.Installs {
		var ghostRefs []string
		for _, p := range install.Profiles {
			if _, isGhost := ghosts[p]; isGhost {
				ghostRefs = append(ghostRefs, p)
			}
		}
		if len(ghostRefs) > 0 {
			out = append(out, Issue{
				Category: "ghost",
				Severity: SevInfo,
				Scope:    "manifest",
				Asset:    install.ID,
				Detail:   fmt.Sprintf("profiles=%v not in live=%v", ghostRefs, snap.LiveProfiles),
			})
		}
	}
	return out
}

func detectBrokenEmptyFiles(snap Snapshot) []Issue {
	var out []Issue
	roots := []string{filepath.Join(snap.HostDir, "skills")}
	if snap.CCPMPresent {
		roots = append(roots,
			filepath.Join(snap.CCPMDir, "share", "skills"),
			filepath.Join(snap.AgentsDir, "skills"),
		)
	}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || d == nil {
				return nil
			}
			if d.IsDir() || d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			info, err := d.Info()
			if err != nil || info.Size() != 0 {
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			out = append(out, Issue{
				Category: "broken",
				Severity: SevWarn,
				Scope:    root,
				Asset:    rel,
				Detail:   "0-byte file",
			})
			return nil
		})
	}
	return out
}

func detectHookDuplication(snap Snapshot) []Issue {
	if !snap.CCPMPresent || len(snap.HostHooks) == 0 {
		return nil
	}
	var out []Issue
	for name, p := range snap.Profiles {
		if !p.HasHooks {
			continue
		}
		out = append(out, Issue{
			Category: "hook-dupe",
			Severity: SevWarn,
			Scope:    name,
			Asset:    "settings.json:hooks",
			Detail:   "profile hooks block duplicates host hooks; cascade dedupes but bytes redundant",
		})
	}
	return out
}

func detectPermissionIntersection(snap Snapshot) []Issue {
	if !snap.CCPMPresent || len(snap.Profiles) < 2 {
		return nil
	}
	count := map[string]int{}
	for _, p := range snap.Profiles {
		settings, ok := readJSON(p.SettingsPath)
		if !ok {
			continue
		}
		for _, e := range stringList(getField(settings, "permissions", "allow")) {
			count[e]++
		}
	}
	intersection := 0
	for _, c := range count {
		if c >= 2 {
			intersection++
		}
	}
	if intersection == 0 {
		return nil
	}
	return []Issue{{
		Category: "perm-dupe",
		Severity: SevInfo,
		Scope:    "all-profiles",
		Asset:    fmt.Sprintf("%d entries", intersection),
		Detail:   "appearing in 2+ profiles — candidate for host promotion",
	}}
}

func detectPluginScopeDrift(snap Snapshot) []Issue {
	if !snap.CCPMPresent {
		return nil
	}
	host := stringSet(snap.HostPlugins)
	var out []Issue
	for name, p := range snap.Profiles {
		for _, plug := range p.EnabledPlugins {
			if _, hostHas := host[plug]; hostHas {
				out = append(out, Issue{
					Category: "plugin-multi-scope",
					Severity: SevInfo,
					Scope:    fmt.Sprintf("host+%s", name),
					Asset:    plug,
					Detail:   "enabled at both host and profile",
				})
			}
		}
	}
	return out
}

func detectMCPScopeDrift(snap Snapshot) []Issue {
	if !snap.CCPMPresent {
		return nil
	}
	host := stringSet(snap.HostMCPs)
	var out []Issue
	for name, p := range snap.Profiles {
		for _, m := range p.MCPs {
			if _, hostHas := host[m]; hostHas {
				out = append(out, Issue{
					Category: "mcp-multi-scope",
					Severity: SevInfo,
					Scope:    fmt.Sprintf("host+%s", name),
					Asset:    m,
					Detail:   "MCP defined at both host and profile",
				})
			}
		}
	}
	return out
}

func detectStalePluginCaches(snap Snapshot) []Issue {
	if !snap.CCPMPresent {
		return nil
	}
	var out []Issue
	for _, p := range snap.Profiles {
		enabled := stringSet(p.EnabledPlugins)
		// Look at <cache>/<marketplace>/<plugin>/<version>/ — the second segment
		// after cache root is the plugin name (without marketplace suffix).
		root := p.PluginCacheDir
		if root == "" {
			continue
		}
		marketplaces, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, mp := range marketplaces {
			if !mp.IsDir() {
				continue
			}
			plugins, err := os.ReadDir(filepath.Join(root, mp.Name()))
			if err != nil {
				continue
			}
			for _, plug := range plugins {
				if !plug.IsDir() {
					continue
				}
				key := plug.Name() + "@" + mp.Name()
				if _, ok := enabled[key]; ok {
					continue
				}
				cachePath := filepath.Join(root, mp.Name(), plug.Name())
				size := dirSize(cachePath)
				out = append(out, Issue{
					Category: "stale-cache",
					Severity: SevInfo,
					Scope:    p.Name,
					Asset:    key,
					Detail:   fmt.Sprintf("disabled plugin cache, %s", humanSize(size)),
					AutoFix: func() error {
						return os.RemoveAll(cachePath)
					},
				})
			}
		}
	}
	return out
}

func detectBudgetOverflow(snap Snapshot) []Issue {
	const budget = 180
	var out []Issue
	if snap.CCPMPresent {
		for _, p := range snap.Profiles {
			direct := len(p.DirectSkills)
			total := direct + p.PluginSkillCount
			if total > budget {
				out = append(out, Issue{
					Category: "budget",
					Severity: SevWarn,
					Scope:    p.Name,
					Asset:    fmt.Sprintf("%d skills", total),
					Detail:   fmt.Sprintf("over budget by %d (direct=%d plugin=%d)", total-budget, direct, p.PluginSkillCount),
				})
			}
		}
	} else {
		direct := len(snap.HostSkills)
		plugin := countSkillMD(filepath.Join(snap.HostDir, "plugins"))
		total := direct + plugin
		if total > budget {
			out = append(out, Issue{
				Category: "budget",
				Severity: SevWarn,
				Scope:    "host",
				Asset:    fmt.Sprintf("%d skills", total),
				Detail:   fmt.Sprintf("over budget by %d (direct=%d plugin=%d)", total-budget, direct, plugin),
			})
		}
	}
	return out
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func dirFingerprint(path string) string {
	h := sha256.New()
	_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(path, p)
		fmt.Fprintln(h, rel)
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		_, _ = io.Copy(h, f)
		_ = f.Close()
		return nil
	})
	return fmt.Sprintf("%x", h.Sum(nil))
}

func dirSize(path string) int64 {
	var total int64
	_ = filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}

func humanSize(n int64) string {
	const (
		_      = iota
		kb int64 = 1 << (10 * iota)
		mb
		gb
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1fGB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.1fMB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.1fKB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

func stringSet(in []string) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for _, s := range in {
		out[s] = struct{}{}
	}
	return out
}

func joinScopes(scopes []string) string {
	var b strings.Builder
	for i, s := range scopes {
		if i > 0 {
			b.WriteByte('+')
		}
		b.WriteString(s)
	}
	return b.String()
}
