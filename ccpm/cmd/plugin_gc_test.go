package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/plugins"
)

// gcSandbox isolates HOME, registers one profile per name→installed_plugins
// body, and seeds a shared-cache entry for each "<mkt>/<plugin>/<version>".
func gcSandbox(t *testing.T, profiles map[string]string, cached ...string) map[string]string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range profiles {
		dir := filepath.Join(home, ".ccpm", "profiles", name)
		writePluginsFile(t, dir, body)
		cfg.AddProfile(name, dir, "oauth")
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	for _, key := range cached {
		parts := strings.Split(key, "/")
		p, err := plugins.CachePluginDir(parts[0], parts[1], parts[2])
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
		paths[key] = p
	}
	return paths
}

func TestPluginGCAbortsOnUnparseableProfile(t *testing.T) {
	paths := gcSandbox(t, map[string]string{
		"broken": `{"version": 2, "plugins": {"used@mkt": [ {"version": "1.0.0"`, // truncated write
	}, "mkt/used/1.0.0")

	err := runPluginGC()
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Errorf("runPluginGC() error = %v, want an error naming profile \"broken\"", err)
	}
	if _, statErr := os.Stat(paths["mkt/used/1.0.0"]); statErr != nil {
		t.Fatalf("GC deleted a cache entry while a profile's plugin list was unreadable: %v", statErr)
	}
}

func TestPluginGCKeepsVersionlessEntries(t *testing.T) {
	paths := gcSandbox(t, map[string]string{
		"p": `{"version": 2, "plugins": {"noversion@mkt": [{"scope": "user"}], "old@mkt": [{"version": "1.0.0"}, {"version": "2.0.0"}]}}`,
	}, "mkt/noversion/0.0.0", "mkt/old/1.0.0", "mkt/old/2.0.0", "mkt/orphan/1.0.0")

	if err := runPluginGC(); err != nil {
		t.Fatalf("runPluginGC: %v", err)
	}
	for _, key := range []string{"mkt/noversion/0.0.0", "mkt/old/1.0.0", "mkt/old/2.0.0"} {
		if _, err := os.Stat(paths[key]); err != nil {
			t.Errorf("GC deleted %s, which a profile still references: %v", key, err)
		}
	}
	if _, err := os.Stat(paths["mkt/orphan/1.0.0"]); !os.IsNotExist(err) {
		t.Errorf("unreferenced mkt/orphan/1.0.0 survived GC (err=%v)", err)
	}
}
