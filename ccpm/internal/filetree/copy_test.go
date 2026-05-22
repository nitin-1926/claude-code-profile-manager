package filetree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyTreeSymlinkToDirectory(t *testing.T) {
	tmp := t.TempDir()
	real := filepath.Join(tmp, "real")
	if err := os.MkdirAll(real, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "SKILL.md"), []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	dst := filepath.Join(tmp, "out")
	if err := CopyTree(link, dst, false); err != nil {
		t.Fatalf("CopyTree: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "ok" {
		t.Fatalf("content = %q", data)
	}
}

// TestCopyTreeEscapingSymlink covers the difference between the strict copier
// (used by `import default`) and the skip-escaping copier (used by `ccpm clone`)
// when a source tree contains a symlink whose target lies OUTSIDE the source —
// exactly what a cascaded/shared asset looks like inside a ccpm profile.
func TestCopyTreeEscapingSymlink(t *testing.T) {
	tmp := t.TempDir()

	// A target that lives outside the source root.
	outside := filepath.Join(tmp, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "HOST.md"), []byte("host"), 0o644); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	// A real file the copy SHOULD always carry.
	if err := os.WriteFile(filepath.Join(src, "own.txt"), []byte("own"), 0o644); err != nil {
		t.Fatal(err)
	}
	// An escaping symlink (points outside src) — like a cascaded host asset.
	if err := os.Symlink(outside, filepath.Join(src, "escaping")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	// Strict CopyTree must REFUSE (preserves the import-default exfil guard).
	if err := CopyTree(src, filepath.Join(tmp, "strict"), false); err == nil {
		t.Fatal("CopyTree should refuse an escaping symlink, got nil")
	}

	// CopyTreeSkipEscaping must SUCCEED, copying real files and skipping the link.
	dst := filepath.Join(tmp, "skip")
	if err := CopyTreeSkipEscaping(src, dst, false); err != nil {
		t.Fatalf("CopyTreeSkipEscaping: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "own.txt")); err != nil || string(data) != "own" {
		t.Fatalf("real file not copied: data=%q err=%v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "escaping")); !os.IsNotExist(err) {
		t.Fatalf("escaping symlink should have been skipped, but something exists at dst/escaping (err=%v)", err)
	}
}

func TestCopyTreeSkipExisting(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(tmp, "dst")
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "a.txt"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := CopyTree(src, dst, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old" {
		t.Fatalf("expected skip, got %q", data)
	}

	if err := CopyTree(src, dst, false); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(dst, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("expected overwrite, got %q", data)
	}
}
