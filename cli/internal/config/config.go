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
