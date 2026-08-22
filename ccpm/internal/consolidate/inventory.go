package consolidate

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
)

// SkillEntry describes a single asset under a skills directory.
type SkillEntry struct {
	Name      string
	IsSymlink bool
	Target    string // resolved symlink target if IsSymlink
	Path      string // absolute path of the entry itself
}

// ProfileSnapshot captures the per-profile bits we audit.
type ProfileSnapshot struct {
	Name             string
	SettingsPath     string
	ClaudeJSONPath   string
	DirectSkills     []SkillEntry
	PluginSkillCount int
	EnabledPlugins   []string
	MCPs             []string
	PluginCacheDir   string
	HasHooks         bool
}

// Snapshot is the structured view consumed by Detect.
type Snapshot struct {
	HomeDir    string
	HostDir    string // ~/.claude
	CCPMDir    string // ~/.ccpm (may be empty if absent)
	AgentsDir  string // ~/.agents (may be empty)

	HostSkills      []SkillEntry
	HostAgents      []SkillEntry
	HostCommands    []SkillEntry
	HostPlugins     []string
	HostMCPs        []string
	HostHooks       map[string]any // PreToolUse/PostToolUse blocks
	HostPermissions []string

	ShareSkills []SkillEntry
	AgentSkills []SkillEntry

	CCPMPresent      bool
	ManifestProfiles []string
	LiveProfiles     []string
	Manifest         *Manifest

	Profiles map[string]ProfileSnapshot
}

// Manifest is a partial decode of ~/.ccpm/installs.json — only fields we
// inspect.
type Manifest struct {
	Version  int             `json:"version"`
	Installs []ManifestEntry `json:"installs"`
}

type ManifestEntry struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Scope    string   `json:"scope"`
	Source   string   `json:"source"`
	Profiles []string `json:"profiles"`
}

// Inventory walks all known scopes and returns a Snapshot. Errors from
// individual missing/unreadable files are tolerated — the caller can act on
// what was found.
func Inventory() (Snapshot, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Snapshot{}, fmt.Errorf("home dir: %w", err)
	}

	snap := Snapshot{
		HomeDir:   home,
		HostDir:   filepath.Join(home, ".claude"),
		CCPMDir:   filepath.Join(home, ".ccpm"),
		AgentsDir: filepath.Join(home, ".agents"),
		Profiles:  map[string]ProfileSnapshot{},
	}

	snap.HostSkills = listSkillDir(filepath.Join(snap.HostDir, "skills"))
	snap.HostAgents = listSkillDir(filepath.Join(snap.HostDir, "agents"))
	snap.HostCommands = listSkillDir(filepath.Join(snap.HostDir, "commands"))

	if hostSettings, ok := readJSON(filepath.Join(snap.HostDir, "settings.json")); ok {
		snap.HostPlugins = sortedKeys(getMapField(hostSettings, "enabledPlugins"))
		snap.HostHooks, _ = hostSettings["hooks"].(map[string]any)
		snap.HostPermissions = stringList(getField(hostSettings, "permissions", "allow"))
	}
	if hostMCP, ok := readJSON(filepath.Join(home, ".claude.json")); ok {
		snap.HostMCPs = sortedKeys(getMapField(hostMCP, "mcpServers"))
	}

	if info, err := os.Stat(snap.CCPMDir); err == nil && info.IsDir() {
		snap.CCPMPresent = true
		snap.ShareSkills = listSkillDir(filepath.Join(snap.CCPMDir, "share", "skills"))
		snap.AgentSkills = listSkillDir(filepath.Join(snap.AgentsDir, "skills"))

		if m := readManifest(filepath.Join(snap.CCPMDir, "installs.json")); m != nil {
			snap.Manifest = m
			snap.ManifestProfiles = manifestProfileSet(m)
		}

		profileRoot := filepath.Join(snap.CCPMDir, "profiles")
		entries, _ := os.ReadDir(profileRoot)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			ps := loadProfile(profileRoot, e.Name())
			snap.Profiles[e.Name()] = ps
			snap.LiveProfiles = append(snap.LiveProfiles, e.Name())
		}
		sort.Strings(snap.LiveProfiles)
	}

	return snap, nil
}

// FilterProfile narrows the snapshot to a single ccpm profile.
func (s Snapshot) FilterProfile(name string) Snapshot {
	if !s.CCPMPresent {
		return s
	}
	if _, ok := s.Profiles[name]; !ok {
		return s
	}
	out := s
	out.Profiles = map[string]ProfileSnapshot{name: s.Profiles[name]}
	out.LiveProfiles = []string{name}
	return out
}

func loadProfile(root, name string) ProfileSnapshot {
	dir := filepath.Join(root, name)
	ps := ProfileSnapshot{
		Name:           name,
		SettingsPath:   filepath.Join(dir, "settings.json"),
		ClaudeJSONPath: filepath.Join(dir, ".claude.json"),
		PluginCacheDir: filepath.Join(dir, "plugins", "cache"),
		DirectSkills:   listSkillDir(filepath.Join(dir, "skills")),
	}
	ps.PluginSkillCount = countSkillMD(ps.PluginCacheDir)

	if d, ok := readJSON(ps.SettingsPath); ok {
		ps.EnabledPlugins = sortedKeys(getMapField(d, "enabledPlugins"))
		_, ps.HasHooks = d["hooks"].(map[string]any)
	}
	if d, ok := readJSON(ps.ClaudeJSONPath); ok {
		ps.MCPs = sortedKeys(getMapField(d, "mcpServers"))
	}
	return ps
}

func listSkillDir(dir string) []SkillEntry {
	out := []SkillEntry{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.Name() == "_sources" {
			continue
		}
		full := filepath.Join(dir, e.Name())
		entry := SkillEntry{Name: e.Name(), Path: full}
		if e.Type()&os.ModeSymlink != 0 {
			entry.IsSymlink = true
			if t, err := os.Readlink(full); err == nil {
				entry.Target = t
			}
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func countSkillMD(root string) int {
	if root == "" {
		return 0
	}
	count := 0
	_ = filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if !d.IsDir() && d.Name() == "SKILL.md" {
			count++
		}
		return nil
	})
	return count
}

func readJSON(path string) (map[string]any, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, false
	}
	return m, true
}

func readManifest(path string) *Manifest {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	return &m
}

func manifestProfileSet(m *Manifest) []string {
	seen := map[string]struct{}{}
	for _, i := range m.Installs {
		for _, p := range i.Profiles {
			seen[p] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

func sortedKeys(m map[string]any) []string {
	return slices.Sorted(maps.Keys(m))
}

func getMapField(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}

func getField(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

func stringList(v any) []string {
	if v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
