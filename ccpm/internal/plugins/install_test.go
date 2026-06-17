package plugins

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// seedMarketplace registers a marketplace named mkt in the shared store with
// one "local"-source plugin (testplug v1.2.3). The clone is a real git repo
// when git is available so HeadSHA resolves.
func seedMarketplace(t *testing.T, mkt, plugin, version string) {
	t.Helper()
	if err := EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	mktDir, err := MarketplacesDir()
	if err != nil {
		t.Fatal(err)
	}
	clone := filepath.Join(mktDir, mkt)

	manifest := map[string]interface{}{
		"name": mkt,
		"plugins": []map[string]interface{}{
			{"name": plugin, "source": "./plugins/" + plugin},
		},
	}
	manifestBytes, _ := json.Marshal(manifest)
	if err := os.MkdirAll(filepath.Join(clone, ".claude-plugin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clone, ".claude-plugin", "marketplace.json"), manifestBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	pluginRoot := filepath.Join(clone, "plugins", plugin)
	if err := os.MkdirAll(filepath.Join(pluginRoot, ".claude-plugin"), 0o700); err != nil {
		t.Fatal(err)
	}
	pluginJSON := []byte(`{"name":"` + plugin + `","version":"` + version + `"}`)
	if err := os.WriteFile(filepath.Join(pluginRoot, ".claude-plugin", "plugin.json"), pluginJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginRoot, "SKILL.md"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Make the clone a real git repo so HeadSHA has something to resolve.
	if _, err := exec.LookPath("git"); err == nil {
		for _, args := range [][]string{
			{"init", "-q"},
			{"-c", "user.email=t@t", "-c", "user.name=t", "add", "."},
			{"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "seed"},
		} {
			cmd := exec.Command("git", append([]string{"-C", clone}, args...)...)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
	}

	reg, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.Marketplaces[mkt] = MarketplaceEntry{
		Name:        mkt,
		Source:      MarketplaceSource{Source: "github", Repo: "org/" + mkt},
		LastUpdated: "2026-06-10T00:00:00Z",
	}
	if err := SaveRegistry(reg); err != nil {
		t.Fatal(err)
	}
}

func TestFetchPluginIntoCacheLocalSourceIdempotent(t *testing.T) {
	isolateHome(t)
	seedMarketplace(t, "mkt", "testplug", "1.2.3")

	mktDir, err := MarketplaceCloneDir("mkt")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := LoadMarketplaceManifest(mktDir)
	if err != nil {
		t.Fatal(err)
	}
	spec := manifest.FindPlugin("testplug")
	if spec == nil {
		t.Fatal("plugin not found in manifest")
	}

	version, sha, err := FetchPluginIntoCache("mkt", *spec, false)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if version != "1.2.3" {
		t.Errorf("version = %q, want 1.2.3", version)
	}
	if _, gitErr := exec.LookPath("git"); gitErr == nil && len(sha) != 40 {
		t.Errorf("commit sha = %q, want full 40-char hash", sha)
	}

	cachePath, err := CachePluginDir("mkt", "testplug", version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cachePath, "SKILL.md")); err != nil {
		t.Errorf("cached content missing: %v", err)
	}

	// Second fetch reuses the cache and reports the same version.
	version2, _, err := FetchPluginIntoCache("mkt", *spec, false)
	if err != nil {
		t.Fatalf("re-fetch: %v", err)
	}
	if version2 != version {
		t.Errorf("idempotent fetch returned %q, want %q", version2, version)
	}

	// No staging turds left behind.
	c, _ := CacheDir()
	entries, _ := os.ReadDir(c)
	for _, e := range entries {
		if len(e.Name()) > 9 && e.Name()[:10] == ".incoming-" {
			t.Errorf("staging dir left behind: %s", e.Name())
		}
	}
}

func TestLinkUnlinkProfileRoundTrip(t *testing.T) {
	isolateHome(t)
	seedMarketplace(t, "mkt", "testplug", "1.2.3")

	mktDir, _ := MarketplaceCloneDir("mkt")
	manifest, _ := LoadMarketplaceManifest(mktDir)
	spec := manifest.FindPlugin("testplug")
	version, sha, err := FetchPluginIntoCache("mkt", *spec, false)
	if err != nil {
		t.Fatal(err)
	}

	profileDir := filepath.Join(t.TempDir(), "profile")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := LinkIntoProfile(profileDir, "mkt", "testplug", version, sha); err != nil {
		t.Fatalf("link: %v", err)
	}

	installedPath := filepath.Join(profileDir, "plugins", "installed_plugins.json")
	doc, err := loadV2Installed(installedPath)
	if err != nil {
		t.Fatal(err)
	}
	entries, ok := doc.Plugins["testplug@mkt"]
	if !ok || len(entries) != 1 {
		t.Fatalf("installed_plugins entry missing: %+v", doc.Plugins)
	}
	if entries[0].Version != version {
		t.Errorf("version = %q, want %q", entries[0].Version, version)
	}
	if entries[0].GitCommitSha != sha {
		t.Errorf("gitCommitSha = %q, want %q (pin must land in installed_plugins.json)", entries[0].GitCommitSha, sha)
	}
	known, err := loadKnownMarketplaces(filepath.Join(profileDir, "plugins", "known_marketplaces.json"))
	if err != nil || len(known) != 1 {
		t.Fatalf("known_marketplaces = %+v, %v", known, err)
	}
	if fi, err := os.Lstat(filepath.Join(profileDir, "plugins", "marketplaces", "mkt")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("marketplace symlink missing: %v", err)
	}

	// Unlink: last plugin from the marketplace also removes the marketplace
	// entry + symlink.
	if err := UnlinkFromProfile(profileDir, "mkt", "testplug"); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	doc, _ = loadV2Installed(installedPath)
	if len(doc.Plugins) != 0 {
		t.Errorf("plugins not removed: %+v", doc.Plugins)
	}
	known, _ = loadKnownMarketplaces(filepath.Join(profileDir, "plugins", "known_marketplaces.json"))
	if len(known) != 0 {
		t.Errorf("marketplace entry should be dropped with its last plugin: %+v", known)
	}
	if _, err := os.Lstat(filepath.Join(profileDir, "plugins", "marketplaces", "mkt")); !os.IsNotExist(err) {
		t.Error("marketplace symlink should be removed with its last plugin")
	}
}

func TestUnlinkKeepsMarketplaceWhileOtherPluginsRemain(t *testing.T) {
	isolateHome(t)
	seedMarketplace(t, "mkt", "plug-a", "1.0.0")

	// Add a second plugin to the same marketplace manifest fixture by linking
	// directly (cache + installed doc are what matter here).
	mktDir, _ := MarketplaceCloneDir("mkt")
	manifest, _ := LoadMarketplaceManifest(mktDir)
	spec := manifest.FindPlugin("plug-a")
	version, sha, err := FetchPluginIntoCache("mkt", *spec, false)
	if err != nil {
		t.Fatal(err)
	}

	profileDir := filepath.Join(t.TempDir(), "profile")
	if err := LinkIntoProfile(profileDir, "mkt", "plug-a", version, sha); err != nil {
		t.Fatal(err)
	}
	// Manually register a second plugin id from the same marketplace.
	installedPath := filepath.Join(profileDir, "plugins", "installed_plugins.json")
	doc, _ := loadV2Installed(installedPath)
	doc.Plugins["plug-b@mkt"] = []installedV2Entry{{Scope: "user", Version: "9.9.9"}}
	data, _ := marshalIndent(doc)
	if err := os.WriteFile(installedPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := UnlinkFromProfile(profileDir, "mkt", "plug-a"); err != nil {
		t.Fatal(err)
	}
	known, _ := loadKnownMarketplaces(filepath.Join(profileDir, "plugins", "known_marketplaces.json"))
	if len(known) != 1 {
		t.Errorf("marketplace entry must survive while plug-b@mkt is installed: %+v", known)
	}
}
