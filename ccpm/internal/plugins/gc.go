package plugins

import (
	"fmt"
	"os"
	"path/filepath"
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

// GarbageCollect removes shared-cache entries that no profile references.
// referencedKey is the set of "<marketplace>/<plugin>/<version>" triples that
// are still in use across all profiles. Returns the list of removed
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
		key := r.Marketplace + "/" + r.Plugin + "/" + r.Version
		if referenced[key] {
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
