package share

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// A real file or directory at dst is profile-local content (e.g. a skill
// Claude Code created inside the profile). Link must refuse it with
// ErrNotLink and leave it intact — host adoption used to RemoveAll it.
func TestLinkRefusesRealFileOrDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	src := filepath.Join(tmp, "host", "foo")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}

	realDir := filepath.Join(tmp, "profile", "skills", "foo")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mine := filepath.Join(realDir, "SKILL.md")
	if err := os.WriteFile(mine, []byte("profile-local"), 0o644); err != nil {
		t.Fatal(err)
	}
	realFile := filepath.Join(tmp, "profile", "agents", "bar.md")
	if err := os.MkdirAll(filepath.Dir(realFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(realFile, []byte("profile-local"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, dst := range []string{realDir, realFile} {
		if err := Link(src, dst); !errors.Is(err, ErrNotLink) {
			t.Errorf("Link over real %s: err = %v, want ErrNotLink", dst, err)
		}
	}
	for _, p := range []string{mine, realFile} {
		if got, err := os.ReadFile(p); err != nil || string(got) != "profile-local" {
			t.Fatalf("Link destroyed profile-local %s: %q, %v", p, got, err)
		}
	}
}

// A Windows copy-fallback dir is recorded by path when created, so a later
// Link (after Developer Mode is enabled, or to refresh it) may replace it.
func TestLinkReplacesRecordedCopyFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests may require elevated privileges on Windows")
	}
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	src := filepath.Join(tmp, "store", "foo")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(tmp, "profile", "skills", "foo")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := recordCopyFallback(dst); err != nil {
		t.Fatal(err)
	}

	if err := Link(src, dst); err != nil {
		t.Fatalf("Link over recorded copy-fallback: %v", err)
	}
	if target, err := os.Readlink(dst); err != nil || target != src {
		t.Fatalf("dst not replaced by symlink to src: target=%q err=%v", target, err)
	}
}

// TestLinkAtomicReplaceOfWrongTargetSymlink pins the A3 fix: replacing a
// wrong-target symlink must go through rename (no remove-then-create window)
// and must leave no temp turds behind.
func TestLinkAtomicReplaceOfWrongTargetSymlink(t *testing.T) {
	tmp := t.TempDir()
	srcA := filepath.Join(tmp, "srcA")
	srcB := filepath.Join(tmp, "srcB")
	for _, d := range []string{srcA, srcB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	dst := filepath.Join(tmp, "dst")
	if err := os.Symlink(srcA, dst); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	if err := Link(srcB, dst); err != nil {
		t.Fatalf("Link over wrong-target symlink: %v", err)
	}
	target, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("dst is not a symlink after Link: %v", err)
	}
	if target != srcB {
		t.Errorf("dst -> %q, want %q", target, srcB)
	}

	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".ccpm-link-") {
			t.Errorf("temp link left behind: %s", e.Name())
		}
	}
}
