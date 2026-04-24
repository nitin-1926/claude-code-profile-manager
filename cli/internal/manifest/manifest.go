package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nitin-1926/ccpm/internal/config"
)

type InstallScope string

const (
	ScopeGlobal  InstallScope = "global"
	ScopeProfile InstallScope = "profile"
)

type AssetKind string

const (
	KindSkill   AssetKind = "skill"
	KindMCP     AssetKind = "mcp"
	KindSetting AssetKind = "setting"
	KindAgent   AssetKind = "agent"
	KindCommand AssetKind = "command"
	KindRule    AssetKind = "rule"
	KindHook    AssetKind = "hook"
	KindPlugin  AssetKind = "plugin"
)

type Install struct {
	ID        string       `json:"id"`
	Kind      AssetKind    `json:"kind"`
	Scope     InstallScope `json:"scope"`
	Source    string       `json:"source,omitempty"`
	Profiles  []string     `json:"profiles,omitempty"`
	CreatedAt string       `json:"created_at"`
}

type Manifest struct {
	Version  string    `json:"version"`
	Installs []Install `json:"installs"`
}

const manifestVersion = "1"

func manifestPath() (string, error) {
	base, err := config.BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "installs.json"), nil
}

func Load() (*Manifest, error) {
	path, err := manifestPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Manifest{Version: manifestVersion}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
