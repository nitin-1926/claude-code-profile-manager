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
	}
	return &m, nil
}

func Save(m *Manifest) error {
	path, err := manifestPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating manifest directory: %w", err)
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("saving manifest: %w", err)
	}
	return nil
}

func (m *Manifest) Add(install Install) {
	install.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	m.Installs = append(m.Installs, install)
}

func (m *Manifest) Remove(id string, kind AssetKind) bool {
	for i, inst := range m.Installs {
		if inst.ID == id && inst.Kind == kind {
			m.Installs = append(m.Installs[:i], m.Installs[i+1:]...)
			return true
		}
	}
	return false
}

func (m *Manifest) Find(id string, kind AssetKind) *Install {
	for i := range m.Installs {
		if m.Installs[i].ID == id && m.Installs[i].Kind == kind {
			return &m.Installs[i]
		}
	}
	return nil
}

func (m *Manifest) ListByKind(kind AssetKind) []Install {
	var result []Install
	for _, inst := range m.Installs {
		if inst.Kind == kind {
			result = append(result, inst)
		}
	}
	return result
}

func (m *Manifest) GlobalInstalls() []Install {
	var result []Install
	for _, inst := range m.Installs {
		if inst.Scope == ScopeGlobal {
			result = append(result, inst)
		}
	}
	return result
}
