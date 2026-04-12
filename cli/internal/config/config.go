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
)

type ProfileConfig struct {
	Name       string `json:"name"`
	Dir        string `json:"dir"`
	AuthMethod string `json:"auth_method"` // "oauth" or "api_key"
	CreatedAt  string `json:"created_at"`
	LastUsed   string `json:"last_used"`
}

type Config struct {
	Version        string                   `json:"version"`
	DefaultProfile string                   `json:"default_profile"`
	Profiles       map[string]ProfileConfig `json:"profiles"`
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
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", d, err)
		}
	}
	return nil
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

	// Atomic write: write to temp file, then rename
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
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
