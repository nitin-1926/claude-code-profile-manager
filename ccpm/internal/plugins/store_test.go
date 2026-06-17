package plugins

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func isolateHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	return tmp
}

func TestRegistryRoundTrip(t *testing.T) {
	isolateHome(t)

	// Missing file yields an empty, usable registry.
	reg, err := LoadRegistry()
	if err != nil {
		t.Fatalf("load empty: %v", err)
	}
	if len(reg.Marketplaces) != 0 {
		t.Fatalf("expected empty registry, got %d entries", len(reg.Marketplaces))
	}

	reg.Marketplaces["beta"] = MarketplaceEntry{
		Name:        "beta",
		Source:      MarketplaceSource{Source: "github", Repo: "org/beta"},
		LastUpdated: "2026-06-10T00:00:00Z",
	}
	reg.Marketplaces["alpha"] = MarketplaceEntry{
		Name:        "alpha",
		Source:      MarketplaceSource{Source: "url", URL: "https://example.com/alpha.git"},
		LastUpdated: "2026-06-10T00:00:00Z",
	}
	if err := SaveRegistry(reg); err != nil {
		t.Fatalf("save: %v", err)
	}

	reloaded, err := LoadRegistry()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reflect.DeepEqual(reg.Marketplaces, reloaded.Marketplaces) {
		t.Errorf("round-trip mismatch:\n  saved:  %+v\n  loaded: %+v", reg.Marketplaces, reloaded.Marketplaces)
	}
	if names := reloaded.MarketplaceNames(); !reflect.DeepEqual(names, []string{"alpha", "beta"}) {
		t.Errorf("MarketplaceNames = %v, want sorted [alpha beta]", names)
	}

	// Registry file must not be world-readable (may carry private repo URLs).
	path, err := RegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err == nil {
		if perm := fi.Mode().Perm(); perm&0o077 != 0 {
			t.Errorf("registry perms = %o, want user-only", perm)
		}
	}
}

func TestEnsureDirsCreatesSharedLayout(t *testing.T) {
	home := isolateHome(t)
	if err := EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{
		filepath.Join("share", "plugins", "marketplaces"),
		filepath.Join("share", "plugins", "cache"),
	} {
		p := filepath.Join(home, ".ccpm", sub)
		if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
			t.Errorf("missing shared dir %s: %v", p, err)
		}
	}
}
