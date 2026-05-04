package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/atomicwrite"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/filetree"
)

func copyTree(src, dst string) error {
	return filetree.CopyTree(src, dst, false)
}

// AddMarketplaceOptions parameterizes RegisterMarketplace.
type AddMarketplaceOptions struct {
	// Repo is the GitHub "<org>/<repo>" slug. Required when URL is empty.
	Repo string
	// URL is an explicit clone URL. Wins over Repo when set.
	URL string
	// Name overrides the marketplace name read from marketplace.json. Only
	// useful when the upstream manifest is absent or wrong.
	Name string
	// SSH forces SSH clone; default is HTTPS.
	SSH bool
}

// RegisterMarketplace clones the marketplace into the shared store, parses
// its manifest to determine the canonical name, and adds it to the registry.
// Returns the resolved name on success.
func RegisterMarketplace(opts AddMarketplaceOptions) (string, error) {
	if opts.Repo == "" && opts.URL == "" {
		return "", fmt.Errorf("AddMarketplaceOptions: Repo or URL required")
	}
	if err := EnsureDirs(); err != nil {
		return "", err
	}

	url := opts.URL
	if url == "" {
		url = GitHubRepoURL(opts.Repo, opts.SSH)
	} else {
		url = normalizeGitURL(url, opts.SSH)
	}

	// Clone into a temp directory first so a partial clone never pollutes
	// the registry. Once we know the canonical name, atomic-rename into the
	// final shared-store path.
	mktDir, err := MarketplacesDir()
	if err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(mktDir, ".incoming-")
	if err != nil {
		return "", fmt.Errorf("creating staging dir: %w", err)
	}
	cloneDest := filepath.Join(tmp, "clone")
	if err := CloneRepo(url, cloneDest, "", opts.SSH); err != nil {
		os.RemoveAll(tmp)
		return "", err
	}

	manifest, err := LoadMarketplaceManifest(cloneDest)
	if err != nil {
		os.RemoveAll(tmp)
		return "", fmt.Errorf("loading marketplace manifest: %w", err)
	}
	name := opts.Name
	if name == "" {
		name = manifest.Name
	}
	if name == "" {
		os.RemoveAll(tmp)
		return "", fmt.Errorf("marketplace manifest missing name and none provided")
	}

	finalDest := filepath.Join(mktDir, name)
	if _, err := os.Stat(finalDest); err == nil {
		// Existing clone — replace it. Move the old one aside so we can
		// atomically swap; if the rename fails we restore the original.
		backup := finalDest + ".old-" + time.Now().UTC().Format("20060102150405")
		if err := os.Rename(finalDest, backup); err != nil {
			os.RemoveAll(tmp)
			return "", fmt.Errorf("moving existing marketplace aside: %w", err)
		}
		if err := os.Rename(cloneDest, finalDest); err != nil {
			_ = os.Rename(backup, finalDest)
			os.RemoveAll(tmp)
			return "", fmt.Errorf("installing new clone: %w", err)
		}
		os.RemoveAll(backup)
	} else {
		if err := os.Rename(cloneDest, finalDest); err != nil {
			os.RemoveAll(tmp)
			return "", fmt.Errorf("installing new clone: %w", err)
		}
	}
	os.RemoveAll(tmp)

	reg, err := LoadRegistry()
	if err != nil {
		return "", err
	}
	source := MarketplaceSource{Source: "github"}
	if opts.Repo != "" {
		source.Repo = opts.Repo
	} else {
		source.Source = "url"
		source.URL = opts.URL
	}
	reg.Marketplaces[name] = MarketplaceEntry{
		Name:        name,
		Source:      source,
		LastUpdated: time.Now().UTC().Format(time.RFC3339),
	}
	if err := SaveRegistry(reg); err != nil {
		return "", err
	}
	return name, nil
}

// RemoveMarketplace deletes the marketplace clone from the shared store and
// drops it from the registry. Caller is responsible for first removing any
// installed plugins that reference the marketplace.
func RemoveMarketplace(name string) error {
	reg, err := LoadRegistry()
	if err != nil {
		return err
	}
	if _, ok := reg.Marketplaces[name]; !ok {
		return fmt.Errorf("marketplace %q not registered", name)
	}
	mktDir, err := MarketplacesDir()
	if err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(mktDir, name)); err != nil {
		return fmt.Errorf("removing marketplace clone: %w", err)
	}
	delete(reg.Marketplaces, name)
	return SaveRegistry(reg)
}

// MarketplaceCloneDir returns the on-disk path of a registered marketplace's
// clone. Returns an error if the marketplace isn't registered.
func MarketplaceCloneDir(name string) (string, error) {
	reg, err := LoadRegistry()
	if err != nil {
		return "", err
	}
	if _, ok := reg.Marketplaces[name]; !ok {
		return "", fmt.Errorf("marketplace %q not registered (run `ccpm plugin marketplace add`)", name)
	}
	mktDir, err := MarketplacesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(mktDir, name), nil
}

// CachePluginDir returns the shared-cache path for a plugin install.
func CachePluginDir(marketplace, pluginName, version string) (string, error) {
	c, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(c, marketplace, pluginName, version), nil
}

// FetchPluginIntoCache resolves a marketplace plugin entry's source and
// materializes its files into the shared cache. The cache dir is created if
// missing; an existing cache dir is reused (FetchPluginIntoCache is
// idempotent). Returns the version string read from the resulting plugin.json.
func FetchPluginIntoCache(marketplace string, spec MarketplacePluginSpec, ssh bool) (string, error) {
	src, err := spec.ResolveSource()
	if err != nil {
		return "", err
	}
	mktDir, err := MarketplaceCloneDir(marketplace)
	if err != nil {
		return "", err
	}

	// Stage the plugin contents into a temp directory, read its plugin.json
	// for the version, then atomic-rename into the cache dir.
	c, err := CacheDir()
	if err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(c, ".incoming-")
	if err != nil {
		return "", fmt.Errorf("staging dir: %w", err)
	}
	stageContent := filepath.Join(stage, "content")

	switch src.Kind {
	case "local":
		from := filepath.Join(mktDir, src.Path)
		if err := copyTree(from, stageContent); err != nil {
			os.RemoveAll(stage)
			return "", fmt.Errorf("copying local plugin %q: %w", src.Path, err)
		}
	case "git-subdir":
		repoStage := filepath.Join(stage, "repo")
		if err := CloneRepo(src.URL, repoStage, src.Ref, ssh); err != nil {
			os.RemoveAll(stage)
			return "", err
		}
		if src.SHA != "" {
			if err := CheckoutSHA(repoStage, src.SHA); err != nil {
				os.RemoveAll(stage)
				return "", err
			}
		}
		from := filepath.Join(repoStage, src.Path)
		if err := copyTree(from, stageContent); err != nil {
			os.RemoveAll(stage)
			return "", fmt.Errorf("copying subdir %q: %w", src.Path, err)
		}
	case "url":
		if err := CloneRepo(src.URL, stageContent, "", ssh); err != nil {
			os.RemoveAll(stage)
			return "", err
		}
		if src.SHA != "" {
			if err := CheckoutSHA(stageContent, src.SHA); err != nil {
				os.RemoveAll(stage)
				return "", err
			}
		}
	case "github":
		url := GitHubRepoURL(src.Repo, ssh)
		if err := CloneRepo(url, stageContent, src.Ref, ssh); err != nil {
			os.RemoveAll(stage)
			return "", err
		}
		if src.SHA != "" {
			if err := CheckoutSHA(stageContent, src.SHA); err != nil {
				os.RemoveAll(stage)
				return "", err
			}
		}
	default:
		os.RemoveAll(stage)
		return "", fmt.Errorf("unsupported source kind %q", src.Kind)
	}

	version, err := readPluginVersion(stageContent)
	if err != nil {
		os.RemoveAll(stage)
		return "", err
	}
	if version == "" {
		version = "0.0.0"
	}

	finalDest, err := CachePluginDir(marketplace, spec.Name, version)
	if err != nil {
		os.RemoveAll(stage)
		return "", err
	}
	if _, err := os.Stat(finalDest); err == nil {
		// Already cached at this version — discard the staging clone.
		os.RemoveAll(stage)
		return version, nil
	}
	if err := os.MkdirAll(filepath.Dir(finalDest), config.DirPerm); err != nil {
		os.RemoveAll(stage)
		return "", fmt.Errorf("creating cache dir: %w", err)
	}
	if err := os.Rename(stageContent, finalDest); err != nil {
		os.RemoveAll(stage)
		return "", fmt.Errorf("installing plugin into cache: %w", err)
	}
	os.RemoveAll(stage)
	return version, nil
}

func readPluginVersion(pluginRoot string) (string, error) {
	path := filepath.Join(pluginRoot, ".claude-plugin", "plugin.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	var pm struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pm); err != nil {
		return "", fmt.Errorf("parsing %s: %w", path, err)
	}
	return pm.Version, nil
}

// LinkIntoProfile creates the per-profile state for an installed plugin:
//
//   - Symlink <profile>/plugins/marketplaces/<marketplace> → shared marketplace
//     (if not already present)
//   - Symlink <profile>/plugins/cache/<marketplace>/<plugin>/<version> → shared cache
//   - Update <profile>/plugins/installed_plugins.json (v2 schema)
//   - Update <profile>/plugins/known_marketplaces.json
//
// The four file/symlink mutations go through a single atomicwrite transaction
// so a failure mid-link can't leave the profile half-installed.
func LinkIntoProfile(profileDir, marketplace, pluginName, version string) error {
	cachePath, err := CachePluginDir(marketplace, pluginName, version)
	if err != nil {
		return err
	}
	mktClone, err := MarketplaceCloneDir(marketplace)
	if err != nil {
		return err
	}

	profileMktSymlink := filepath.Join(profileDir, "plugins", "marketplaces", marketplace)
	profileCacheSymlink := filepath.Join(profileDir, "plugins", "cache", marketplace, pluginName, version)
	installedPath := filepath.Join(profileDir, "plugins", "installed_plugins.json")
	knownPath := filepath.Join(profileDir, "plugins", "known_marketplaces.json")

	installedDoc, err := loadV2Installed(installedPath)
	if err != nil {
		return err
	}
	knownDoc, err := loadKnownMarketplaces(knownPath)
	if err != nil {
		return err
	}

	id := pluginName + "@" + marketplace
	now := time.Now().UTC().Format(time.RFC3339)
	entry := installedV2Entry{
		Scope:        "user",
		InstallPath:  profileCacheSymlink,
		Version:      version,
		InstalledAt:  now,
		LastUpdated:  now,
	}
	installedDoc.Plugins[id] = []installedV2Entry{entry}

	knownDoc[marketplace] = knownMarketplaceEntry{
		Source: MarketplaceSource{
			Source: "github",
			Repo:   "",
		},
		InstallLocation: profileMktSymlink,
		LastUpdated:     now,
	}
	// Carry forward the registered source if we know it; falling back to a
	// minimal record if the registry is missing.
	if reg, regErr := LoadRegistry(); regErr == nil {
		if e, ok := reg.Marketplaces[marketplace]; ok {
			km := knownDoc[marketplace]
			km.Source = e.Source
			knownDoc[marketplace] = km
		}
	}

	installedBytes, err := marshalIndent(installedDoc)
	if err != nil {
		return fmt.Errorf("marshaling installed_plugins.json: %w", err)
	}
	knownBytes, err := marshalIndent(knownDoc)
	if err != nil {
		return fmt.Errorf("marshaling known_marketplaces.json: %w", err)
	}

	changes := []atomicwrite.FileChange{
		atomicwrite.SymlinkAt(profileMktSymlink, mktClone),
		atomicwrite.SymlinkAt(profileCacheSymlink, cachePath),
		atomicwrite.WriteFile(installedPath, installedBytes, config.FilePerm),
		atomicwrite.WriteFile(knownPath, knownBytes, config.FilePerm),
	}
	return atomicwrite.Apply(changes)
}

// UnlinkFromProfile reverses LinkIntoProfile. The symlinks are removed and
// the plugin entry is dropped from installed_plugins.json. The marketplace
// entry in known_marketplaces.json is preserved if any other plugin from the
// same marketplace is still installed; otherwise removed too.
func UnlinkFromProfile(profileDir, marketplace, pluginName string) error {
	id := pluginName + "@" + marketplace

	installedPath := filepath.Join(profileDir, "plugins", "installed_plugins.json")
	knownPath := filepath.Join(profileDir, "plugins", "known_marketplaces.json")

	installedDoc, err := loadV2Installed(installedPath)
	if err != nil {
		return err
	}
	knownDoc, err := loadKnownMarketplaces(knownPath)
	if err != nil {
		return err
	}

	// Capture the cache symlink path from the entry so we know what to remove.
	var cacheSymlink string
	if entries, ok := installedDoc.Plugins[id]; ok && len(entries) > 0 {
		cacheSymlink = entries[len(entries)-1].InstallPath
	}
	delete(installedDoc.Plugins, id)

	stillUsed := false
	for otherID := range installedDoc.Plugins {
		if strings.HasSuffix(otherID, "@"+marketplace) {
			stillUsed = true
			break
		}
	}
	if !stillUsed {
		delete(knownDoc, marketplace)
	}

	installedBytes, err := marshalIndent(installedDoc)
	if err != nil {
		return err
	}
	knownBytes, err := marshalIndent(knownDoc)
	if err != nil {
		return err
	}

	changes := []atomicwrite.FileChange{
		atomicwrite.WriteFile(installedPath, installedBytes, config.FilePerm),
		atomicwrite.WriteFile(knownPath, knownBytes, config.FilePerm),
	}
	if cacheSymlink != "" {
		changes = append(changes, atomicwrite.DeleteFile(cacheSymlink))
	}
	if !stillUsed {
		profileMktSymlink := filepath.Join(profileDir, "plugins", "marketplaces", marketplace)
		changes = append(changes, atomicwrite.DeleteFile(profileMktSymlink))
	}
	return atomicwrite.Apply(changes)
}

// ----- on-disk helper types -----

type installedV2Entry struct {
	Scope        string `json:"scope"`
	InstallPath  string `json:"installPath"`
	Version      string `json:"version"`
	InstalledAt  string `json:"installedAt"`
	LastUpdated  string `json:"lastUpdated"`
	GitCommitSha string `json:"gitCommitSha,omitempty"`
}

type installedV2Doc struct {
	Version int                            `json:"version"`
	Plugins map[string][]installedV2Entry `json:"plugins"`
}

func loadV2Installed(path string) (*installedV2Doc, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &installedV2Doc{Version: 2, Plugins: map[string][]installedV2Entry{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var doc installedV2Doc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if doc.Plugins == nil {
		doc.Plugins = map[string][]installedV2Entry{}
	}
	if doc.Version == 0 {
		doc.Version = 2
	}
	return &doc, nil
}

type knownMarketplaceEntry struct {
	Source          MarketplaceSource `json:"source"`
	InstallLocation string            `json:"installLocation"`
	LastUpdated     string            `json:"lastUpdated"`
}

func loadKnownMarketplaces(path string) (map[string]knownMarketplaceEntry, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]knownMarketplaceEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	doc := map[string]knownMarketplaceEntry{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return doc, nil
}

func marshalIndent(v interface{}) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
