package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CacheReference identifies one entry in the shared cache.
type CacheReference struct {
	Marketplace string
	Plugin      string
	Version     string
}

func (r CacheReference) Path() (string, error) {
	return CachePluginDir(r.Marketplace, r.Plugin, r.Version)
}

// EnumerateCache walks the shared cache directory and returns every cached
// (marketplace, plugin, version) triple. Returns an empty slice if the cache
// directory does not exist.
func EnumerateCache() ([]CacheReference, error) {
	cacheDir, err := CacheDir()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		return nil, nil
	}
	var refs []CacheReference
	mkts, err := os.ReadDir(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("reading cache dir: %w", err)
	}
	for _, m := range mkts {
		if !m.IsDir() {
			continue
		}
		plugins, err := os.ReadDir(filepath.Join(cacheDir, m.Name()))
		if err != nil {
			continue
		}
		for _, p := range plugins {
			if !p.IsDir() {
				continue
			}
			versions, err := os.ReadDir(filepath.Join(cacheDir, m.Name(), p.Name()))
			if err != nil {
				continue
			}
			for _, v := range versions {
				if !v.IsDir() {
					continue
				}
				refs = append(refs, CacheReference{
					Marketplace: m.Name(),
					Plugin:      p.Name(),
					Version:     v.Name(),
				})
			}
		}
	}
	return refs, nil
}

// AddProfileReferences records into referenced every shared-cache key the
// profile's installed_plugins.json points at: "<marketplace>/<plugin>/<version>"
// for each entry (not just the newest), and "<marketplace>/<plugin>" for a
// versionless entry, which pins every cached version of that plugin. A read
// or parse failure is returned, never skipped: GC must not delete caches a
// profile it could not read may still be using.
func AddProfileReferences(profileDir string, referenced map[string]bool) error {
	doc, err := loadV2Installed(filepath.Join(profileDir, "plugins", "installed_plugins.json"))
	if err != nil {
		return err
	}
	for id, entries := range doc.Plugins {
		name, mkt, ok := strings.Cut(id, "@")
		if !ok {
			continue // no marketplace segment: never lives in the shared cache
		}
		for _, e := range entries {
			if e.Version == "" {
				referenced[mkt+"/"+name] = true
				continue
			}
			referenced[mkt+"/"+name+"/"+e.Version] = true
		}
	}
	return nil
}

// GarbageCollect removes shared-cache entries that no profile references.
// referenced is the set built by AddProfileReferences: "<marketplace>/<plugin>/<version>"
// triples, plus "<marketplace>/<plugin>" pairs that keep every version of
// a plugin. Returns the list of removed
// references and any error from the first failed removal (subsequent removals
// continue regardless so a single permission error doesn't strand the rest).
func GarbageCollect(referenced map[string]bool) ([]CacheReference, error) {
	refs, err := EnumerateCache()
	if err != nil {
		return nil, err
	}
	var removed []CacheReference
	var firstErr error
	for _, r := range refs {
		plugin := r.Marketplace + "/" + r.Plugin
		if referenced[plugin] || referenced[plugin+"/"+r.Version] {
			continue
		}
		path, err := r.Path()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		removed = append(removed, r)
		// Walk up: if the plugin directory and marketplace directory are now
		// empty after this removal, drop them too so the cache stays tidy.
		pluginDir := filepath.Dir(path)
		_ = removeIfEmpty(pluginDir)
		mktDir := filepath.Dir(pluginDir)
		_ = removeIfEmpty(mktDir)
	}
	return removed, firstErr
}

func removeIfEmpty(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return os.Remove(dir)
	}
	return nil
}
