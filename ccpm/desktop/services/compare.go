//go:build darwin

package services

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/filetree"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
)

// Row statuses: values are compared, layers are shown but never compared.
const (
	StatusSame      = "same"
	StatusChanged   = "changed"
	StatusOnlyLeft  = "only-left"
	StatusOnlyRight = "only-right"
)

// CompareRow is one name in one category, side by side. An absent side has an
// empty value and layer.
type CompareRow struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Left       string `json:"left"`
	LeftLayer  Layer  `json:"leftLayer"`
	Right      string `json:"right"`
	RightLayer Layer  `json:"rightLayer"`
}

type CompareCategory struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Secret values (env vars) are sent for an explicit reveal; the UI masks
	// them until then.
	Secret bool         `json:"secret"`
	Rows   []CompareRow `json:"rows"`
}

type Comparison struct {
	Left  string `json:"left"`
	Right string `json:"right"`
	// Message says why nothing was compared (an unknown profile).
	Message    string            `json:"message"`
	Categories []CompareCategory `json:"categories"`
}

var compareCategories = []struct{ id, label string }{
	{"skill", "Skills"},
	{"agent", "Agents"},
	{"command", "Commands"},
	{"rule", "Rules"},
	{"hook", "Hooks"},
	{"mcp", "MCP servers"},
	{"plugin", "Plugins"},
	{"permission", "Permissions"},
	{"env", "Env vars"},
	{"setting", "Settings"},
}

// CompareService diffs the effective config of two profiles, built from the
// same engine as the Cascade and Details tabs.
type CompareService struct{}

func NewCompare() *CompareService { return &CompareService{} }

// Compare returns left and right side by side, per category. An unknown
// profile is not an error: it returns no categories and a Message.
func (s *CompareService) Compare(left, right string) (*Comparison, error) {
	out := &Comparison{Left: left, Right: right, Categories: []CompareCategory{}}
	cfg, err := config.Load()
	if err != nil {
		return out, err
	}
	names := []string{left}
	if right != left {
		names = append(names, right)
	}
	var missing []string
	for _, name := range names {
		if _, ok := cfg.Profiles[name]; !ok {
			missing = append(missing, fmt.Sprintf("%q", name))
		}
	}
	if len(missing) > 0 {
		out.Message = "Profile " + strings.Join(missing, " and ") + " not found."
		return out, nil
	}

	l, err := snapshot(left, cfg.Profiles[left])
	if err != nil {
		return out, fmt.Errorf("%s: %w", left, err)
	}
	r := l
	if right != left {
		if r, err = snapshot(right, cfg.Profiles[right]); err != nil {
			return out, fmt.Errorf("%s: %w", right, err)
		}
	}
	for _, c := range compareCategories {
		out.Categories = append(out.Categories, CompareCategory{
			ID:     c.id,
			Label:  c.label,
			Secret: c.id == "env",
			Rows:   diffRows(l[c.id], r[c.id]),
		})
	}
	return out, nil
}

// entry is one side of a row. value is what the UI shows; eq is what is
// compared, and differs from value only where value is redacted.
type entry struct {
	value, eq string
	layer     Layer
	// Assets only: the entry inside the profile dir, and whether it is a
	// symlink. Profile-local copies are compared by content (see sameContent).
	path string
	link bool
}

// snapshot is one profile's effective config as category → name → entry.
func snapshot(profile string, pc config.ProfileConfig) (map[string]map[string]entry, error) {
	home, _ := os.UserHomeDir()
	hostRoot := filepath.Join(home, ".claude")
	base, _ := config.BaseDir()

	out := map[string]map[string]entry{}
	set := func(cat, name string, e entry) {
		if out[cat] == nil {
			out[cat] = map[string]entry{}
		}
		out[cat][name] = e
	}
	put := func(cat, name, value, eq string, layer Layer) {
		set(cat, name, entry{value: value, eq: eq, layer: layer})
	}

	for _, a := range collectAssets(pc.Dir, hostRoot, filepath.Join(base, "share"), home) {
		// plugins/ is Claude Code's plugin store (cache/, installed_plugins.json),
		// not one entry per plugin; enabledPlugins below is the plugin list.
		if a.Kind == "plugin" {
			continue
		}
		p := filepath.Join(pc.Dir, a.Kind+"s", a.Name)
		fi, err := os.Lstat(p)
		link := err == nil && fi.Mode()&os.ModeSymlink != 0
		set(a.Kind, a.Name, entry{value: a.Source, eq: a.Source, layer: a.Layer, path: p, link: link})
	}

	merged, err := settingsmerge.ComputeMerged(pc.Dir, profile, "")
	if err != nil {
		return nil, err
	}
	layers := loadSettingLayers(pc.Dir, profile, hostRoot, base)
	for key, v := range merged {
		switch key {
		case "permissions":
			perms, _ := v.(map[string]interface{})
			for sub, pv := range perms {
				if sub != "allow" && sub != "ask" && sub != "deny" {
					s := plain(pv)
					put("permission", sub, s, s, layers.winner(key, sub))
				}
			}
			// A rule's value is its bucket, so a rule moved from allow to deny
			// reads as changed. Arrays replace wholesale in the merge, so a
			// rule's layer is its bucket's.
			for _, bucket := range []string{"allow", "ask", "deny"} {
				for _, rule := range toStrings(perms[bucket]) {
					val, layer := bucket, layers.winner(key, bucket)
					if prev, ok := out["permission"][rule]; ok {
						val, layer = prev.value+", "+bucket, prev.layer
					}
					put("permission", rule, val, val, layer)
				}
			}
		case "enabledPlugins":
			plugs, _ := v.(map[string]interface{})
			for id, pv := range plugs {
				s := plain(pv)
				put("plugin", id, s, s, layers.winner(key, id))
			}
		case "env":
			vars, _ := v.(map[string]interface{})
			for name, ev := range vars {
				s := plain(ev)
				put("env", name, s, s, layers.winner(key, name))
			}
		default:
			s := fullJSON(v)
			put("setting", key, s, s, layers.winner(key))
		}
	}
	// `ccpm env set` vars ride in the launch environment. A settings env value
	// for the same name is shown instead, assuming Claude Code applies settings
	// env over its inherited environment.
	for name, val := range pc.Env {
		if _, ok := out["env"][name]; !ok {
			put("env", name, val, val, LayerProfile)
		}
	}

	servers, mcpLayers, err := settingsmerge.EffectiveMCP(pc.Dir, profile)
	if err != nil {
		return nil, err
	}
	for name, def := range servers {
		put("mcp", name, mcpSummary(def), fullJSON(def), Layer(mcpLayers[name]))
	}
	return out, nil
}

func diffRows(left, right map[string]entry) []CompareRow {
	names := make([]string, 0, len(left)+len(right))
	for n := range left {
		names = append(names, n)
	}
	for n := range right {
		if _, ok := left[n]; !ok {
			names = append(names, n)
		}
	}
	sort.Strings(names)

	rows := make([]CompareRow, 0, len(names))
	for _, n := range names {
		l, inL := left[n]
		r, inR := right[n]
		status := StatusChanged
		switch {
		case !inR:
			status = StatusOnlyLeft
		case !inL:
			status = StatusOnlyRight
		case l.eq == r.eq:
			status = StatusSame
		case (!l.link || !r.link) && sameContent(l.path, r.path):
			status = StatusSame
		}
		rows = append(rows, CompareRow{
			Name: n, Status: status,
			Left: l.value, LeftLayer: l.layer,
			Right: r.value, RightLayer: r.layer,
		})
	}
	return rows
}

// sameContent reports whether two asset entries hold identical bytes. A
// profile-local copy's path contains its profile's name, so comparing paths
// would call two identical copies different. Used when either side is a
// local copy; two symlinks are compared by their targets.
func sameContent(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	// ponytail: an asset over 16 MiB or 5000 entries is not hashed and reads
	// as changed, so a huge dir cannot stall the tab; raise if real ones hit it.
	const maxBytes, maxEntries = 16 << 20, 5000
	ha, err := filetree.ContentHash(a, maxBytes, maxEntries)
	if err != nil {
		return false
	}
	hb, err := filetree.ContentHash(b, maxBytes, maxEntries)
	return err == nil && ha == hb
}

// mcpSummary describes an MCP server from an allowlist of fields. Definitions
// carry tokens (env, headers, URL queries, args, unknown fields), so values
// are compared whole but only these parts are ever shown: type, command, the
// URL without credentials or query, the arg count, and env/header names.
func mcpSummary(def interface{}) string {
	m, _ := def.(map[string]interface{})
	var parts []string
	for _, k := range []string{"type", "command"} {
		if s, ok := m[k].(string); ok && s != "" {
			parts = append(parts, s)
		}
	}
	if s, ok := m["url"].(string); ok && s != "" {
		parts = append(parts, redactURL(s))
	}
	// ponytail: args hidden wholesale (they carry tokens like --api-key); show
	// the non-secret ones if the count proves too little to go on.
	if args, ok := m["args"].([]interface{}); ok && len(args) == 1 {
		parts = append(parts, "1 arg")
	} else if len(args) > 1 {
		parts = append(parts, fmt.Sprintf("%d args", len(args)))
	}
	for _, k := range []string{"env", "headers"} {
		if names := sortedKeys(m[k]); len(names) > 0 {
			parts = append(parts, k+": "+strings.Join(names, ", "))
		}
	}
	return strings.Join(parts, " · ")
}

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "(unparseable url)"
	}
	hidden := u.RawQuery != "" || u.Fragment != ""
	u.User, u.RawQuery, u.Fragment, u.ForceQuery = nil, "", "", false
	s := u.String()
	if hidden {
		s += "?…"
	}
	return s
}

func sortedKeys(v interface{}) []string {
	m, _ := v.(map[string]interface{})
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// plain renders a string as itself and anything else as JSON.
func plain(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fullJSON(v)
}

// fullJSON is compactJSON without the truncation: values are compared on it,
// and two long values differing past the cut must not read as the same.
func fullJSON(v interface{}) string {
	b, _ := json.Marshal(v) // map keys marshal sorted, so equal values compare equal
	return string(b)
}
