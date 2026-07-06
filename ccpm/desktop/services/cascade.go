//go:build darwin

package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
)

// Layer is which level of the cascade an asset/setting resolves from.
// v1 covers host (~/.claude, cascaded) → global (~/.ccpm/share) → profile.
type Layer string

const (
	LayerHost    Layer = "host"
	LayerGlobal  Layer = "global"
	LayerProfile Layer = "profile"
)

// assetKinds are the symlink-backed asset directories inside a profile.
var assetKinds = []string{"skills", "agents", "commands", "rules", "hooks", "plugins"}

// CascadeAsset is one effective asset plus where it came from.
type CascadeAsset struct {
	Kind       string  `json:"kind"`
	Name       string  `json:"name"`
	Layer      Layer   `json:"layer"`
	Source     string  `json:"source"`               // resolved target (tilde'd)
	ShadowedIn []Layer `json:"shadowedIn,omitempty"`  // other layers that also define this name
}

// CascadeSetting is one top-level effective settings key plus its provenance.
type CascadeSetting struct {
	Key          string  `json:"key"`
	Layer        Layer   `json:"layer"`          // winning layer
	Contributors []Layer `json:"contributors"`   // every layer that sets it
	Value        string  `json:"value"`          // compact JSON of the effective value
	Merged       bool    `json:"merged"`         // object value with >1 contributor
}

// Cascade is the full effective-config picture for a profile.
type Cascade struct {
	Profile  string           `json:"profile"`
	Assets   []CascadeAsset   `json:"assets"`
	Settings []CascadeSetting `json:"settings"`
}

// CascadeService computes the effective config by reusing ccpm's own engine,
// so the view cannot drift from what the CLI resolves.
type CascadeService struct{}

func NewCascade() *CascadeService { return &CascadeService{} }

// Get returns the host→global→profile effective config for a profile.
func (s *CascadeService) Get(profile string) (*Cascade, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	pc, ok := cfg.Profiles[profile]
	if !ok {
		return &Cascade{Profile: profile, Assets: []CascadeAsset{}, Settings: []CascadeSetting{}}, nil
	}

	home, _ := os.UserHomeDir()
	hostRoot := filepath.Join(home, ".claude")
	base, _ := config.BaseDir()
	globalRoot := filepath.Join(base, "share")

	return &Cascade{
		Profile:  profile,
		Assets:   collectAssets(pc.Dir, hostRoot, globalRoot, home),
		Settings: collectSettings(pc.Dir, profile, hostRoot, base),
	}, nil
}

func collectAssets(profileDir, hostRoot, globalRoot, home string) []CascadeAsset {
	out := []CascadeAsset{}
	for _, kind := range assetKinds {
		dir := filepath.Join(profileDir, kind)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if name == "" || name[0] == '.' || name[0] == '_' {
				continue
			}
			full := filepath.Join(dir, name)
			target := full
			if resolved, err := os.Readlink(full); err == nil {
				if !filepath.IsAbs(resolved) {
					resolved = filepath.Join(dir, resolved)
				}
				target = resolved
			}
			layer := classifyLayer(target, hostRoot, globalRoot)
			out = append(out, CascadeAsset{
				Kind:       strings.TrimSuffix(kind, "s"),
				Name:       name,
				Layer:      layer,
				Source:     tilde(target, home),
				ShadowedIn: shadows(kind, name, layer, hostRoot, globalRoot),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func classifyLayer(target, hostRoot, globalRoot string) Layer {
	switch {
	case strings.HasPrefix(target, hostRoot):
		return LayerHost
	case strings.HasPrefix(target, globalRoot):
		return LayerGlobal
	default:
		return LayerProfile
	}
}

// shadows reports OTHER layers that also define name (so the UI can flag overrides).
func shadows(kind, name string, winner Layer, hostRoot, globalRoot string) []Layer {
	var out []Layer
	if winner != LayerHost {
		if exists(filepath.Join(hostRoot, kind, name)) {
			out = append(out, LayerHost)
		}
	}
	if winner != LayerGlobal {
		if exists(filepath.Join(globalRoot, kind, name)) {
			out = append(out, LayerGlobal)
		}
	}
	return out
}

func collectSettings(profileDir, profile, hostRoot, base string) []CascadeSetting {
	// the three v1 layers, lowest → highest precedence
	profileS, _ := settingsmerge.LoadJSON(filepath.Join(profileDir, "settings.json"))
	hostS, _ := settingsmerge.LoadJSON(filepath.Join(hostRoot, "settings.json"))
	shareS, _ := settingsmerge.LoadJSON(filepath.Join(base, "share", "settings", profile+".json"))

	// effective values straight from the engine (single source of truth)
	effective, err := settingsmerge.ComputeMerged(profileDir, profile, "")
	if err != nil || effective == nil {
		effective = map[string]interface{}{}
	}

	keys := map[string]struct{}{}
	for k := range effective {
		keys[k] = struct{}{}
	}

	out := []CascadeSetting{}
	for k := range keys {
		var contributors []Layer
		if _, ok := profileS[k]; ok {
			contributors = append(contributors, LayerProfile)
		}
		if _, ok := hostS[k]; ok {
			contributors = append(contributors, LayerHost)
		}
		if _, ok := shareS[k]; ok {
			contributors = append(contributors, LayerGlobal)
		}
		// winner = highest precedence present (global fragment > host > profile)
		winner := LayerProfile
		for _, l := range contributors {
			if l == LayerHost && winner == LayerProfile {
				winner = LayerHost
			}
			if l == LayerGlobal {
				winner = LayerGlobal
			}
		}
		_, isObj := effective[k].(map[string]interface{})
		out = append(out, CascadeSetting{
			Key:          k,
			Layer:        winner,
			Contributors: contributors,
			Value:        compactJSON(effective[k]),
			Merged:       isObj && len(contributors) > 1,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func tilde(p, home string) string {
	if home != "" && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

func compactJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	s := string(b)
	const max = 160
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}
