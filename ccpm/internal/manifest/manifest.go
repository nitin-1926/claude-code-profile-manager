package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/atomicwrite"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

type InstallScope string

const (
	// ScopeGlobal: explicitly registered through `ccpm <asset> add --global`.
	// Auto-propagates to every profile.
	ScopeGlobal InstallScope = "global"
	// ScopeProfile: registered for a specific profile via `ccpm <asset> add`
	// or imported with `ccpm import default --profile <name>`.
	ScopeProfile InstallScope = "profile"
	// ScopeHost: discovered in ~/.claude/<asset>/ at launch and auto-adopted
	// into every profile. Source is always "host:<absolute path>". Distinct
	// from ScopeGlobal so doctor and listings can show provenance, and so
	// users who turn off cascade-auto-adopt can purge them as a group
	// without touching genuinely-global installs.
	ScopeHost InstallScope = "host"
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

// KindPlural maps each directory-backed AssetKind to the subdirectory name
// used under both ~/.claude/ and a profile dir. Single source of truth — the
// cascade scanner, doctor, and asset commands all key directories off this
// instead of repeating the mapping.
var KindPlural = map[AssetKind]string{
	KindSkill:   "skills",
	KindAgent:   "agents",
	KindCommand: "commands",
	KindRule:    "rules",
	KindHook:    "hooks",
}

// DedupableKindsOrdered returns the directory-backed kinds in a stable
// display/scan order.
func DedupableKindsOrdered() []AssetKind {
	return []AssetKind{KindSkill, KindAgent, KindCommand, KindRule, KindHook}
}

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

// Path returns the on-disk location of the manifest file. Exported so callers
// that batch the manifest update with other writes (via atomicwrite) can
// target the same file.
func Path() (string, error) {
	return manifestPath()
}

// MarshalBytes returns the on-disk byte representation of m without touching
// the filesystem. Used by callers that want to bundle the manifest write into
// a larger atomicwrite transaction.
func MarshalBytes(m *Manifest) ([]byte, error) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling manifest: %w", err)
	}
	return data, nil
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

	if err := os.MkdirAll(filepath.Dir(path), config.DirPerm); err != nil {
		return fmt.Errorf("creating manifest directory: %w", err)
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	// Crash-safe, concurrency-safe write via atomicwrite (crypto-random staging
	// name + atomic rename + rollback) instead of a fixed ".tmp" suffix that two
	// concurrent processes would clobber.
	if err := atomicwrite.Apply([]atomicwrite.FileChange{
		atomicwrite.WriteFile(path, data, config.FilePerm),
	}); err != nil {
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

// CascadeInstalls returns the union of Global and Host installs — the set
// that should be linked into every profile at launch. Profile-scoped entries
// are not included; those are owned by their specific profile.
func (m *Manifest) CascadeInstalls() []Install {
	var result []Install
	for _, inst := range m.Installs {
		if inst.Scope == ScopeGlobal || inst.Scope == ScopeHost {
			result = append(result, inst)
		}
	}
	return result
}
