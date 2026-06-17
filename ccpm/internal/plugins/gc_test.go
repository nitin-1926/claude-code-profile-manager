package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

func seedCacheEntry(t *testing.T, mkt, plugin, version string) string {
	t.Helper()
	path, err := CachePluginDir(mkt, plugin, version)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "plugin.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGarbageCollectKeepsReferencedRemovesOrphans(t *testing.T) {
	isolateHome(t)
	if err := EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	kept := seedCacheEntry(t, "mkt", "keepme", "1.0.0")
	orphan := seedCacheEntry(t, "mkt", "dropme", "2.0.0")
	orphanOtherMkt := seedCacheEntry(t, "other", "gone", "0.1.0")

	refs, err := EnumerateCache()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 3 {
		t.Fatalf("enumerate = %d entries, want 3: %+v", len(refs), refs)
	}

	removed, err := GarbageCollect(map[string]bool{"mkt/keepme/1.0.0": true})
	if err != nil {
		t.Fatalf("gc: %v", err)
	}
	if len(removed) != 2 {
		t.Errorf("removed %d entries, want 2: %+v", len(removed), removed)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("referenced entry was removed: %v", err)
	}
	for _, p := range []string{orphan, orphanOtherMkt} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("orphan %s survived gc", p)
		}
	}
	// Empty parent dirs of removed entries are tidied; the marketplace dir of
	// the kept entry must survive.
	if _, err := os.Stat(filepath.Dir(filepath.Dir(orphanOtherMkt))); !os.IsNotExist(err) {
		t.Error("empty marketplace dir not tidied after gc")
	}
	if _, err := os.Stat(filepath.Dir(filepath.Dir(kept))); err != nil {
		t.Errorf("kept entry's marketplace dir vanished: %v", err)
	}
}

func TestGarbageCollectEmptyCacheIsNoop(t *testing.T) {
	isolateHome(t)
	removed, err := GarbageCollect(map[string]bool{})
	if err != nil {
		t.Fatalf("gc on missing cache dir: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("removed %+v from an empty cache", removed)
	}
}
