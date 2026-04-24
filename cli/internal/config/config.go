package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	configVersion = "1"
	ccpmDir       = ".ccpm"
	configFile    = "config.json"

	// FilePerm is the mode used for every ccpm-owned file on disk. User-only
	// read/write (0600) so nothing under ~/.ccpm leaks to other local users
	// on a shared host — config.json reveals profile paths and auth methods,
	// profile fragments can contain `env` entries carrying tokens, etc.
	FilePerm os.FileMode = 0600

	// DirPerm is the mode used for every ccpm-owned directory. 0700 mirrors
	// FilePerm's intent and prevents another local user from listing or
	// traversing profile/vault/share subtrees.
	DirPerm os.FileMode = 0700
)

type ProfileConfig struct {
	Name       string            `json:"name"`
	Dir        string            `json:"dir"`
	AuthMethod string            `json:"auth_method"` // "oauth" or "api_key"
	CreatedAt  string            `json:"created_at"`
	LastUsed   string            `json:"last_used"`
	Env        map[string]string `json:"env,omitempty"`
}

type Settings struct {
	CheckDefaultDrift bool `json:"check_default_drift,omitempty"`
}

type Config struct {
	Version        string                   `json:"version"`
	DefaultProfile string                   `json:"default_profile"`
	Profiles       map[string]ProfileConfig `json:"profiles"`
	Settings       Settings                 `json:"settings,omitempty"`
}

func BaseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ccpmDir), nil
}

func DefaultPath() (string, error) {
	base, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, configFile), nil
}

func ProfilesDir() (string, error) {
	base, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "profiles"), nil
}

func VaultDir() (string, error) {
	base, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "vault"), nil
}

func EnsureDirs() error {
	base, err := BaseDir()
	if err != nil {
		return err
	}
	dirs := []string{
		base,
		filepath.Join(base, "profiles"),
		filepath.Join(base, "vault"),
		filepath.Join(base, "share"),
		filepath.Join(base, "share", "skills"),
		filepath.Join(base, "share", "agents"),
		filepath.Join(base, "share", "commands"),
		filepath.Join(base, "share", "rules"),
		filepath.Join(base, "share", "hooks"),
		filepath.Join(base, "share", "mcp"),
		filepath.Join(base, "share", "settings"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, DirPerm); err != nil {
			return fmt.Errorf("creating directory %s: %w", d, err)
		}
	}
	return nil
}

func ProfileNames(cfg *Config) []string {
	names := make([]string, 0, len(cfg.Profiles))
	for n := range cfg.Profiles {
		names = append(names, n)
	}
	return names
}

func Load() (*Config, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{
			Version:  configVersion,
			Profiles: make(map[string]ProfileConfig),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]ProfileConfig)
	}
	return &cfg, nil
}

func Save(cfg *Config) error {
	path, err := DefaultPath()
	if err != nil {
		return err
	}

	if err := EnsureDirs(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	// Atomic write: write to temp file, then rename. FilePerm is 0600 so the
	// config (which lists every profile path + auth method) is not readable
	// by other local users on shared hosts.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, FilePerm); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("saving config: %w", err)
	}
	return nil
}

func (c *Config) AddProfile(name, dir, authMethod string) {
	c.Profiles[name] = ProfileConfig{
		Name:       name,
		Dir:        dir,
		AuthMethod: authMethod,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		LastUsed:   time.Now().UTC().Format(time.RFC3339),
	}
}

func (c *Config) RemoveProfile(name string) {
	if c.DefaultProfile == name {
		c.DefaultProfile = ""
	}
	delete(c.Profiles, name)
}

func (c *Config) UpdateLastUsed(name string) {
	if p, ok := c.Profiles[name]; ok {
		p.LastUsed = time.Now().UTC().Format(time.RFC3339)
		c.Profiles[name] = p
	}
}
