package filetree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSeedStoreEntryLiveSymlink(t *testing.T) {
	tmp := t.TempDir()
	real := filepath.Join(tmp, "real")
	if err := os.MkdirAll(real, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "SKILL.md"), []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	dst := filepath.Join(tmp, "store")

	seeded, err := SeedStoreEntry(link, dst, true)
	if err != nil {
		t.Fatalf("SeedStoreEntry: %v", err)
	}
	if !seeded {
		t.Fatal("expected live symlink branch")
	}
	fi, err := os.Lstat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("dst is not a symlink: %v", fi.Mode())
	}

	if err := os.WriteFile(filepath.Join(real, "SKILL.md"), []byte("v2"), 0644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "v2" {
		t.Fatalf("live symlink didn't propagate: got %q", data)
	}
}

func TestSeedStoreEntryCopyFallback(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("snap"), 0644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(tmp, "store")

	seeded, err := SeedStoreEntry(src, dst, true)
	if err != nil {
		t.Fatalf("SeedStoreEntry: %v", err)
	}
	if seeded {
		t.Fatal("expected copy branch (src is not a symlink)")
	}
	data, err := os.ReadFile(filepath.Join(dst, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "snap" {
		t.Fatalf("copy content = %q", data)
	}
}

func TestSeedStoreEntryIdempotent(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("one"), 0644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(tmp, "store")

	if _, err := SeedStoreEntry(src, dst, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("two"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedStoreEntry(src, dst, false); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dst, "a.txt"))
	if string(data) != "two" {
		t.Fatalf("second seed didn't overwrite: %q", data)
	}
}
