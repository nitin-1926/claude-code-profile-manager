package share

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnsureDirs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	os.MkdirAll(filepath.Join(tmp, ".ccpm"), 0755)

	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs() error: %v", err)
	}

	for _, sub := range []string{"share", "share/skills", "share/mcp", "share/settings"} {
		dir := filepath.Join(tmp, ".ccpm", sub)
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("directory %q should exist: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%q should be a directory", dir)
		}
	}
}

func TestLinkAndUnlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests may require elevated privileges on Windows")
	}

	tmp := t.TempDir()

	src := filepath.Join(tmp, "source")
	os.MkdirAll(src, 0755)
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("# Test"), 0644)

	dst := filepath.Join(tmp, "profiles", "work", "skills", "test")

	if err := Link(src, dst); err != nil {
		t.Fatalf("Link() error: %v", err)
	}

	// Verify it's a symlink
	target, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("should be a symlink: %v", err)
	}
	if target != src {
		t.Errorf("symlink target = %q, want %q", target, src)
	}

	// Verify the file is accessible through the link
	data, err := os.ReadFile(filepath.Join(dst, "SKILL.md"))
	if err != nil {
		t.Fatalf("should be able to read through symlink: %v", err)
	}
	if string(data) != "# Test" {
		t.Errorf("content = %q, want '# Test'", string(data))
	}

	// Unlink
	if err := Unlink(dst); err != nil {
		t.Fatalf("Unlink() error: %v", err)
	}

	if _, err := os.Lstat(dst); !os.IsNotExist(err) {
		t.Error("symlink should be removed after Unlink()")
	}
}

func TestLinkIdempotent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests may require elevated privileges on Windows")
	}

	tmp := t.TempDir()

	src := filepath.Join(tmp, "source")
	os.MkdirAll(src, 0755)

	dst := filepath.Join(tmp, "dst")

	if err := Link(src, dst); err != nil {
		t.Fatalf("first Link() error: %v", err)
	}
	if err := Link(src, dst); err != nil {
		t.Fatalf("second Link() should be idempotent: %v", err)
	}
}

func TestIsLinked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests may require elevated privileges on Windows")
	}

	tmp := t.TempDir()

	src := filepath.Join(tmp, "source")
	os.MkdirAll(src, 0755)

	dst := filepath.Join(tmp, "dst")
	os.Symlink(src, dst)

	if !IsLinked(src, dst) {
		t.Error("IsLinked should return true for correct symlink")
	}

	otherSrc := filepath.Join(tmp, "other")
	if IsLinked(otherSrc, dst) {
		t.Error("IsLinked should return false for wrong target")
	}
}

func TestUnlinkNonExistent(t *testing.T) {
	if err := Unlink("/nonexistent/path"); err != nil {
		t.Errorf("Unlink of non-existent path should not error: %v", err)
	}
}

func TestLinkReplacesExisting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests may require elevated privileges on Windows")
	}

	tmp := t.TempDir()

	src1 := filepath.Join(tmp, "source1")
	src2 := filepath.Join(tmp, "source2")
	os.MkdirAll(src1, 0755)
	os.MkdirAll(src2, 0755)

	dst := filepath.Join(tmp, "dst")

	Link(src1, dst)
	Link(src2, dst)

	target, _ := os.Readlink(dst)
	if target != src2 {
		t.Errorf("symlink should point to src2 after replacement, got %q", target)
	}
}
