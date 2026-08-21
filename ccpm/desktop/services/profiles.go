//go:build darwin

// Package services holds the Wails-bound Go services the desktop frontend calls.
// READS go straight through ccpm's internal/* engine so the GUI can never drift
// from what the CLI sees. WRITES (later milestones) shell out to the ccpm binary.
package services

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

// AssetCounts is a quick per-profile tally for the overview/list, counted by
// listing the profile's asset directories (symlinks included).
type AssetCounts struct {
	Skills   int `json:"skills"`
	Agents   int `json:"agents"`
	Commands int `json:"commands"`
	Rules    int `json:"rules"`
	Hooks    int `json:"hooks"`
	Plugins  int `json:"plugins"`
}

// Profile is the DTO the frontend renders. It mirrors config.ProfileConfig plus
// derived fields (default flag, asset counts).
type Profile struct {
	Name       string      `json:"name"`
	Dir        string      `json:"dir"`
	AuthMethod string      `json:"authMethod"`
	CreatedAt  string      `json:"createdAt"`
	LastUsed   string      `json:"lastUsed"`
	IsDefault  bool        `json:"isDefault"`
	Counts     AssetCounts `json:"counts"`
}

// Profiles is the bound service for reading profile data.
type Profiles struct{}

// NewProfiles constructs the service (bound once in main.go).
func NewProfiles() *Profiles { return &Profiles{} }

// List returns every registered profile, sorted by last-used (desc) then name.
func (s *Profiles) List() ([]Profile, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	out := make([]Profile, 0, len(cfg.Profiles))
	for name, pc := range cfg.Profiles {
		out = append(out, toDTO(name, pc, cfg.DefaultProfile))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LastUsed != out[j].LastUsed {
			return out[i].LastUsed > out[j].LastUsed // RFC3339 sorts lexically
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Get returns a single profile by name, or nil if it is not registered.
func (s *Profiles) Get(name string) (*Profile, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	pc, ok := cfg.Profiles[name]
	if !ok {
		return nil, nil
	}
	dto := toDTO(name, pc, cfg.DefaultProfile)
	return &dto, nil
}

func toDTO(name string, pc config.ProfileConfig, defaultProfile string) Profile {
	return Profile{
		Name:       name,
		Dir:        pc.Dir,
		AuthMethod: pc.AuthMethod,
		CreatedAt:  pc.CreatedAt,
		LastUsed:   pc.LastUsed,
		IsDefault:  name == defaultProfile,
		Counts:     countAssets(pc.Dir),
	}
}

func countAssets(profileDir string) AssetCounts {
	return AssetCounts{
		Skills:   countDir(filepath.Join(profileDir, "skills")),
		Agents:   countDir(filepath.Join(profileDir, "agents")),
		Commands: countDir(filepath.Join(profileDir, "commands")),
		Rules:    countDir(filepath.Join(profileDir, "rules")),
		Hooks:    countDir(filepath.Join(profileDir, "hooks")),
		Plugins:  countDir(filepath.Join(profileDir, "plugins")),
	}
}

// countDir counts visible entries in dir; missing dir → 0.
func countDir(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.Name() == "" || e.Name()[0] == '.' {
			continue
		}
		n++
	}
	return n
}
