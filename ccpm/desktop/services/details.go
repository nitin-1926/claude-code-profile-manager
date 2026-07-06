//go:build darwin

package services

import (
	"encoding/json"
	"os/exec"
	"sort"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
)

type PermissionView struct {
	Allow []string `json:"allow"`
	Ask   []string `json:"ask"`
	Deny  []string `json:"deny"`
	Mode  string   `json:"mode"`
}

type PluginView struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type EnvVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type McpView struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Sources []string `json:"sources"`
}

type Details struct {
	Profile     string         `json:"profile"`
	Permissions PermissionView `json:"permissions"`
	Plugins     []PluginView   `json:"plugins"`
	Env         []EnvVar       `json:"env"`
	Mcp         []McpView      `json:"mcp"`
}

// DetailsService reads the structured config a profile resolves to (permissions,
// plugins, env, MCP) by reusing the engine's merge + the CLI's mcp list --json.
type DetailsService struct{}

func NewDetails() *DetailsService { return &DetailsService{} }

func emptyDetails(profile string) *Details {
	return &Details{
		Profile:     profile,
		Permissions: PermissionView{Allow: []string{}, Ask: []string{}, Deny: []string{}},
		Plugins:     []PluginView{},
		Env:         []EnvVar{},
		Mcp:         []McpView{},
	}
}

func (s *DetailsService) Get(profile string) (*Details, error) {
	// non-nil slices so the JSON is [] not null (nil slices marshal to null,
	// which would make the frontend's .length/.map throw).
	out := emptyDetails(profile)

	cfg, err := config.Load()
	if err != nil {
		return out, err
	}
	pc, ok := cfg.Profiles[profile]
	if !ok {
		return out, nil
	}

	merged, _ := settingsmerge.ComputeMerged(pc.Dir, profile, "")
	if perms, ok := merged["permissions"].(map[string]interface{}); ok {
		out.Permissions = PermissionView{
			Allow: toStrings(perms["allow"]),
			Ask:   toStrings(perms["ask"]),
			Deny:  toStrings(perms["deny"]),
			Mode:  toString(perms["defaultMode"]),
		}
	}
	if plugs, ok := merged["enabledPlugins"].(map[string]interface{}); ok {
		for name, v := range plugs {
			en, _ := v.(bool)
			out.Plugins = append(out.Plugins, PluginView{Name: name, Enabled: en})
		}
		sort.Slice(out.Plugins, func(i, j int) bool { return out.Plugins[i].Name < out.Plugins[j].Name })
	}

	for k, v := range pc.Env {
		out.Env = append(out.Env, EnvVar{Key: k, Value: v})
	}
	sort.Slice(out.Env, func(i, j int) bool { return out.Env[i].Key < out.Env[j].Key })

	if mcp := readMCP(); mcp != nil {
		out.Mcp = mcp
	}
	return out, nil
}

// readMCP shells `ccpm mcp list --json` (read of a write-tool) and parses it.
func readMCP() []McpView {
	bin := findCCPM()
	if bin == "" {
		return nil
	}
	cmd := exec.Command(bin, "mcp", "list", "--json")
	cmd.Env = append(envWithoutColor(), "NO_COLOR=1")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var list []McpView
	if json.Unmarshal(out, &list) != nil {
		return nil
	}
	return list
}

// SettingKV is one top-level merged settings key and its JSON value (for the
// Settings editor). Big structured keys shown in their own tabs are excluded.
type SettingKV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// SettingsService reads the effective settings a profile resolves to, for editing.
type SettingsService struct{}

func NewSettings() *SettingsService { return &SettingsService{} }

// excludedSettingKeys have dedicated tabs already (Permissions / MCP & Plugins).
var excludedSettingKeys = map[string]bool{
	"permissions":    true,
	"enabledPlugins": true,
	"mcpServers":     true,
}

func (s *SettingsService) Get(profile string) ([]SettingKV, error) {
	cfg, err := config.Load()
	if err != nil {
		return []SettingKV{}, err
	}
	pc, ok := cfg.Profiles[profile]
	if !ok {
		return []SettingKV{}, nil
	}
	merged, _ := settingsmerge.ComputeMerged(pc.Dir, profile, "")
	out := []SettingKV{}
	for k, v := range merged {
		if excludedSettingKeys[k] {
			continue
		}
		b, _ := json.Marshal(v)
		out = append(out, SettingKV{Key: k, Value: string(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func toStrings(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return []string{}
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func toString(v interface{}) string {
	s, _ := v.(string)
	return s
}
