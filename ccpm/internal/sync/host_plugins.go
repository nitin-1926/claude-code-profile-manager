package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/atomicwrite"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/defaultclaude"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/manifest"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/pluginschema"
)

// The installed_plugins.json shapes are owned by internal/pluginschema —
// shared with internal/plugins without creating a build cycle.
type (
	hostPluginEntry = pluginschema.InstalledEntry
	hostPluginDoc   = pluginschema.InstalledDoc
)

// hostKnownMarketplace mirrors the marketplace shape in
// ~/.claude/plugins/known_marketplaces.json. The exact schema has been
// changing across Claude Code releases; we only consume installLocation
// and source, both of which have been stable.
type hostKnownMarketplace struct {
	Source          map[string]any `json:"source"`
	InstallLocation string         `json:"installLocation"`
	LastUpdated     string         `json:"lastUpdated,omitempty"`
}

// scanHostPlugins reads ~/.claude/plugins/installed_plugins.json and returns
// every plugin entry not already in m as KindPlugin (regardless of scope).
// Entries are returned with their original installPath untouched — the
// adopt path rewrites it when materializing into the profile.
func scanHostPlugins(m *manifest.Manifest) ([]hostPluginAdoption, error) {
	hostRoot, err := defaultclaude.DefaultDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(hostRoot, "plugins", "installed_plugins.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var doc hostPluginDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	registered := make(map[string]struct{}, len(m.Installs))
	for _, inst := range m.Installs {
		if inst.Kind == manifest.KindPlugin {
			registered[inst.ID] = struct{}{}
		}
	}

	ids := make([]string, 0, len(doc.Plugins))
	for id := range doc.Plugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var out []hostPluginAdoption
	for _, id := range ids {
		entries := doc.Plugins[id]
		if len(entries) == 0 {
			continue
		}
		if _, ok := registered[id]; ok {
			continue
		}
		// Take the most-recent entry; v2 doc allows historical versions.
		latest := entries[len(entries)-1]
		marketplace, plugin := parsePluginID(id)
		if marketplace == "" || plugin == "" {
			continue
		}
		out = append(out, hostPluginAdoption{
			ID:          id,
			Marketplace: marketplace,
			Plugin:      plugin,
			Version:     latest.Version,
			InstallPath: latest.InstallPath,
			Entry:       latest,
		})
	}
	return out, nil
}

// hostPluginAdoption is a single host plugin we want to surface inside a
// profile. Carries enough context to rewrite the profile-side
// installed_plugins.json + symlink the cache payload.
type hostPluginAdoption struct {
	ID          string // "<plugin>@<marketplace>"
	Marketplace string
	Plugin      string
	Version     string
	InstallPath string // host-side cache path (typically ~/.claude/plugins/cache/<m>/<p>/<v>)
	Entry       hostPluginEntry
}

// adoptHostPlugins materializes each host plugin into the profile by:
//
//  1. Symlinking the cache payload directory into <profile>/plugins/cache/.
//  2. Symlinking the marketplace clone into <profile>/plugins/marketplaces/
//     when known via known_marketplaces.json.
//  3. Merging an installed_plugins.json entry with installPath rewritten to
//     point at the profile-side symlink (matching the convention used by
//     internal/plugins.LinkIntoProfile for ccpm-installed plugins).
//  4. Updating known_marketplaces.json so Claude Code can resolve the
//     marketplace at session time.
//  5. Registering the plugin in the manifest as ScopeHost on first adopt.
//
// All file/symlink mutations land in a single atomicwrite transaction so a
// partial failure can't leave the profile half-installed.
func adoptHostPlugins(profileDir, profileName string, plugs []hostPluginAdoption, m *manifest.Manifest) error {
	if len(plugs) == 0 {
		return nil
	}
	hostRoot, err := defaultclaude.DefaultDir()
	if err != nil {
		return err
	}
	hostKnown, err := loadHostKnownMarketplaces(filepath.Join(hostRoot, "plugins", "known_marketplaces.json"))
	if err != nil {
		return fmt.Errorf("reading host known_marketplaces.json: %w", err)
	}

	installedPath := filepath.Join(profileDir, "plugins", "installed_plugins.json")
	knownPath := filepath.Join(profileDir, "plugins", "known_marketplaces.json")

	installedDoc, err := loadProfileInstalled(installedPath)
	if err != nil {
		return err
	}
	knownDoc, err := loadProfileKnown(knownPath)
	if err != nil {
		return err
	}

	// Track marketplace symlinks to create — keyed so we don't double-add
	// when several plugins share a marketplace.
	mktSymlinks := map[string]string{} // profile-side path -> host-side path

	now := time.Now().UTC().Format(time.RFC3339)

	cacheChanges := make([]atomicwrite.FileChange, 0, len(plugs)*2+2)
	for _, p := range plugs {
		// Skip if the host install path is missing — Claude Code wrote the
		// entry but the payload is gone (uninstalled out of band). The
		// doctor will surface this as a stale host entry.
		if _, err := os.Stat(p.InstallPath); err != nil {
			continue
		}

		profileCachePath := filepath.Join(
			profileDir, "plugins", "cache",
			p.Marketplace, p.Plugin, safeVersion(p.Version),
		)

		// Defer to whatever's already at the destination if it isn't a
		// symlink we control: a real directory there means ccpm's own
		// plugin pipeline (or the user) already installed this plugin
		// locally — we should not clobber it with a host symlink. Doctor
		// will surface this as a shadowing case via the manifest.
		skipPlugin := false
		if info, err := os.Lstat(profileCachePath); err == nil {
			if info.Mode()&os.ModeSymlink == 0 {
				skipPlugin = true
			} else if target, terr := os.Readlink(profileCachePath); terr == nil && target == p.InstallPath {
				// Already linked to the right host path — idempotent skip.
				skipPlugin = true
			}
		}
		if skipPlugin {
			continue
		}

		cacheChanges = append(cacheChanges, atomicwrite.SymlinkAt(profileCachePath, p.InstallPath))

		// Marketplace clone — best-effort. Only if the host knows about it
		// AND the profile-side path isn't already a real directory.
		if hk, ok := hostKnown[p.Marketplace]; ok && hk.InstallLocation != "" {
			profileMktPath := filepath.Join(profileDir, "plugins", "marketplaces", p.Marketplace)
			if info, err := os.Lstat(profileMktPath); err == nil && info.Mode()&os.ModeSymlink == 0 {
				// Existing real directory — don't clobber.
			} else {
				if target, terr := os.Readlink(profileMktPath); terr == nil && target == hk.InstallLocation {
					// Idempotent skip.
				} else {
					mktSymlinks[profileMktPath] = hk.InstallLocation
				}
			}
			knownDoc[p.Marketplace] = profileKnownEntry{
				Source:          hk.Source,
				InstallLocation: filepath.Join(profileDir, "plugins", "marketplaces", p.Marketplace),
				LastUpdated:     hk.LastUpdated,
			}
		}

		// Profile-side installed entry: scope=user, installPath rewritten.
		entry := profileInstalledEntry{
			Scope:        "user",
			InstallPath:  profileCachePath,
			Version:      p.Version,
			InstalledAt:  p.Entry.InstalledAt,
			LastUpdated:  now,
			GitCommitSha: p.Entry.GitCommitSha,
		}
		installedDoc.Plugins[p.ID] = []profileInstalledEntry{entry}

		// Manifest registration — first adopt only.
		if existing := m.Find(p.ID, manifest.KindPlugin); existing == nil {
			m.Add(manifest.Install{
				ID:       p.ID,
				Kind:     manifest.KindPlugin,
				Scope:    manifest.ScopeHost,
				Source:   "host:" + p.InstallPath,
				Profiles: []string{profileName},
			})
		} else if !containsProfile(existing.Profiles, profileName) {
			existing.Profiles = append(existing.Profiles, profileName)
		}
	}

	for prof, host := range mktSymlinks {
		cacheChanges = append(cacheChanges, atomicwrite.SymlinkAt(prof, host))
	}

	// Nothing to do — every plugin was already installed locally or
	// already linked to the right host path. Skip the no-op manifest writes
	// so we don't churn timestamps.
	if len(cacheChanges) == 0 {
		return nil
	}

	installedBytes, err := marshalIndentJSON(installedDoc)
	if err != nil {
		return fmt.Errorf("marshaling installed_plugins.json: %w", err)
	}
	knownBytes, err := marshalIndentJSON(knownDoc)
	if err != nil {
		return fmt.Errorf("marshaling known_marketplaces.json: %w", err)
	}

	cacheChanges = append(cacheChanges,
		atomicwrite.WriteFile(installedPath, installedBytes, config.FilePerm),
		atomicwrite.WriteFile(knownPath, knownBytes, config.FilePerm),
	)

	if err := atomicwrite.Apply(cacheChanges); err != nil {
		return fmt.Errorf("applying host plugin cascade for %q: %w", profileName, err)
	}
	return nil
}

// linkHostPlugin re-runs the plugin cascade for a single existing
// ScopeHost manifest entry — used when a new profile is created and host
// plugins are already in the manifest from a prior cascade.
func linkHostPlugin(profileDir, profileName string, inst manifest.Install) error {
	src := strings.TrimPrefix(inst.Source, "host:")
	if src == "" {
		return nil
	}
	if _, err := os.Stat(src); err != nil {
		return nil
	}
	marketplace, plugin := parsePluginID(inst.ID)
	if marketplace == "" || plugin == "" {
		return nil
	}
	// Reconstruct a single-entry adoption from the manifest-stored data;
	// we don't have the original installedAt/sha so use a fresh stamp.
	return adoptHostPlugins(profileDir, profileName, []hostPluginAdoption{{
		ID:          inst.ID,
		Marketplace: marketplace,
		Plugin:      plugin,
		Version:     versionFromCachePath(src),
		InstallPath: src,
		Entry: hostPluginEntry{
			Scope:       "user",
			InstallPath: src,
			Version:     versionFromCachePath(src),
		},
	}}, &manifest.Manifest{}) // fresh manifest copy: we don't want re-registration here
}

func parsePluginID(id string) (marketplace, plugin string) {
	at := strings.LastIndex(id, "@")
	if at <= 0 || at == len(id)-1 {
		return "", ""
	}
	return id[at+1:], id[:at]
}

// versionFromCachePath extracts the trailing version segment from a cache
// path like /Users/x/.claude/plugins/cache/m/p/0.40.0. Falls back to "" if
// the path doesn't match the expected shape; callers tolerate empty.
func versionFromCachePath(p string) string {
	return filepath.Base(p)
}

func safeVersion(v string) string {
	if v == "" {
		return "0.0.0"
	}
	return v
}

// ---- profile-side I/O helpers (mirror internal/plugins shapes) ----

type profileInstalledEntry struct {
	Scope        string `json:"scope"`
	InstallPath  string `json:"installPath"`
	Version      string `json:"version"`
	InstalledAt  string `json:"installedAt"`
	LastUpdated  string `json:"lastUpdated"`
	GitCommitSha string `json:"gitCommitSha,omitempty"`
}

type profileInstalledDoc struct {
	Version int                                  `json:"version"`
	Plugins map[string][]profileInstalledEntry  `json:"plugins"`
}

type profileKnownEntry struct {
	Source          map[string]any `json:"source"`
	InstallLocation string         `json:"installLocation"`
	LastUpdated     string         `json:"lastUpdated,omitempty"`
}

func loadProfileInstalled(path string) (*profileInstalledDoc, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &profileInstalledDoc{Version: 2, Plugins: map[string][]profileInstalledEntry{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var doc profileInstalledDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if doc.Plugins == nil {
		doc.Plugins = map[string][]profileInstalledEntry{}
	}
	if doc.Version == 0 {
		doc.Version = 2
	}
	return &doc, nil
}

func loadProfileKnown(path string) (map[string]profileKnownEntry, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]profileKnownEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	doc := map[string]profileKnownEntry{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return doc, nil
}

func loadHostKnownMarketplaces(path string) (map[string]hostKnownMarketplace, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]hostKnownMarketplace{}, nil
	}
	if err != nil {
		return nil, err
	}
	doc := map[string]hostKnownMarketplace{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func marshalIndentJSON(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
