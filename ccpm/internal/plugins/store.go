// Package plugins implements ccpm's external plugin install path: marketplace
// registration, plugin install/remove (global or per-profile), and shared-
// cache garbage collection.
//
// Layout (everything under ~/.ccpm):
//
//	share/plugins/marketplaces/<name>/   shared marketplace clones
//	share/plugins/cache/<marketplace>/<plugin>/<version>/  shared plugin caches
//	share/plugins/marketplaces.json      ccpm's marketplace registry (which
//	                                     marketplaces are present and where
//	                                     they came from)
//
// Per profile, ccpm writes Claude-Code-shaped state files directly so a
// ccpm-managed plugin is indistinguishable from one installed via
// `/plugin install` inside Claude Code:
//
//	<profile>/plugins/marketplaces/<name>            symlink → shared marketplace
//	<profile>/plugins/cache/<marketplace>/<plugin>/<version>  symlink → shared cache
//	<profile>/plugins/known_marketplaces.json        per-profile, ccpm writes it
//	<profile>/plugins/installed_plugins.json         per-profile, ccpm writes it
//	<profile>/settings.json#enabledPlugins.<id>      via ccpm settings fragment
package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/atomicwrite"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

// SharedDir returns ~/.ccpm/share/plugins.
func SharedDir() (string, error) {
	base, err := config.BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "share", "plugins"), nil
}

// MarketplacesDir returns the shared-store directory for marketplace clones.
func MarketplacesDir() (string, error) {
	d, err := SharedDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "marketplaces"), nil
}

// CacheDir returns the shared-store directory for plugin caches.
func CacheDir() (string, error) {
	d, err := SharedDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "cache"), nil
}

// RegistryPath returns the path to the ccpm-owned marketplace registry.
func RegistryPath() (string, error) {
	d, err := SharedDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "marketplaces.json"), nil
}

// EnsureDirs creates every shared-plugins directory ccpm needs.
func EnsureDirs() error {
	for _, fn := range []func() (string, error){SharedDir, MarketplacesDir, CacheDir} {
		d, err := fn()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(d, config.DirPerm); err != nil {
			return fmt.Errorf("creating %s: %w", d, err)
		}
	}
	return nil
}

// MarketplaceSource records where a marketplace was fetched from. For now we
// only model GitHub repositories; the schema mirrors what Claude Code itself
// writes into known_marketplaces.json.
type MarketplaceSource struct {
	Source string `json:"source"`         // "github"
	Repo   string `json:"repo,omitempty"` // "<org>/<repo>" for github
	URL    string `json:"url,omitempty"`  // explicit clone URL (used when Source != "github")
}

// MarketplaceEntry is one row in the ccpm-owned registry.
type MarketplaceEntry struct {
	Name        string            `json:"name"`
	Source      MarketplaceSource `json:"source"`
	LastUpdated string            `json:"last_updated"`
}

// Registry is ~/.ccpm/share/plugins/marketplaces.json. It tracks which
// marketplaces ccpm has cloned into the shared store. Each profile's own
// known_marketplaces.json is derived from this plus that profile's installs.
type Registry struct {
	Version      int                         `json:"version"`
	Marketplaces map[string]MarketplaceEntry `json:"marketplaces"`
}

// LoadRegistry reads the marketplace registry. Missing file yields an empty
// registry with version 1.
func LoadRegistry() (*Registry, error) {
	path, err := RegistryPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Registry{Version: 1, Marketplaces: map[string]MarketplaceEntry{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading registry: %w", err)
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parsing registry: %w", err)
	}
	if reg.Marketplaces == nil {
		reg.Marketplaces = map[string]MarketplaceEntry{}
	}
	return &reg, nil
}

// SaveRegistry writes the registry through atomicwrite so a partial write
// can't corrupt the source-of-truth.
func SaveRegistry(reg *Registry) error {
	path, err := RegistryPath()
	if err != nil {
		return err
	}
	if err := EnsureDirs(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling registry: %w", err)
	}
	return atomicwrite.Apply([]atomicwrite.FileChange{
		atomicwrite.WriteFile(path, append(data, '\n'), config.FilePerm),
	})
}

// MarketplaceNames returns the registered marketplace names sorted
// alphabetically.
func (r *Registry) MarketplaceNames() []string {
	names := make([]string, 0, len(r.Marketplaces))
	for n := range r.Marketplaces {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
