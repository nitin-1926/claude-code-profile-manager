package filetree

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

// TestCopyTreeSelfLoopSymlink guards against infinite recursion when a
// directory contains a symlink pointing back at itself or an ancestor.
// Pre-fix this hung forever; now it must terminate with an error (strict) or
// by skipping the entry (skip-escaping).
func TestCopyTreeSelfLoopSymlink(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(src, filepath.Join(src, "loop")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- CopyTree(src, filepath.Join(tmp, "out-strict"), false) }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("strict CopyTree with self-loop symlink: want error, got nil")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("strict CopyTree hung on self-loop symlink")
	}

	go func() { done <- CopyTreeSkipEscaping(src, filepath.Join(tmp, "out-skip"), false) }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("skip-escaping CopyTree with self-loop symlink: %v", err)
		}
		if _, err := os.Stat(filepath.Join(tmp, "out-skip", "file.txt")); err != nil {
			t.Errorf("regular file not copied alongside skipped loop: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("skip-escaping CopyTree hung on self-loop symlink")
	}
}

// TestCopyTreeNestedSymlinkChecksOriginalRoot pins the root-threading fix: a
// symlink inside a followed subtree that targets a sibling elsewhere in the
// ORIGINAL root is legitimate and must copy (the old code re-derived the root
// from the resolved subtree and falsely treated it as escaping), while a
// nested symlink that truly escapes the original root must still refuse.
func TestCopyTreeNestedSymlinkChecksOriginalRoot(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	for _, d := range []string{"sub", "sibling"} {
		if err := os.MkdirAll(filepath.Join(src, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(src, "sibling", "data.txt"), []byte("sib"), 0o644); err != nil {
		t.Fatal(err)
	}
	// src/dirlink -> src/sub ; src/sub/tosibling -> src/sibling
	if err := os.Symlink(filepath.Join(src, "sub"), filepath.Join(src, "dirlink")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	if err := os.Symlink(filepath.Join(src, "sibling"), filepath.Join(src, "sub", "tosibling")); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(tmp, "out")
	if err := CopyTree(src, dst, false); err != nil {
		t.Fatalf("CopyTree with within-root nested symlink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "dirlink", "tosibling", "data.txt")); err != nil {
		t.Errorf("nested within-root symlink content missing: %v", err)
	}

	// Now a nested symlink that escapes the original root entirely.
	outside := filepath.Join(tmp, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(src, "sub", "escape")); err != nil {
		t.Fatal(err)
	}
	if err := CopyTree(src, filepath.Join(tmp, "out2"), false); err == nil {
		t.Error("CopyTree with nested escaping symlink: want error, got nil")
	}
}
