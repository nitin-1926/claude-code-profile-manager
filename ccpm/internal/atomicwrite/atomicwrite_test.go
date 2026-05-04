package atomicwrite

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestApply_Empty(t *testing.T) {
	if err := Apply(nil); err != nil {
		t.Errorf("Apply(nil) = %v, want nil", err)
	}
	if err := Apply([]FileChange{}); err != nil {
		t.Errorf("Apply([]) = %v, want nil", err)
	}
}

func TestApply_WriteNew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "new.txt")

	err := Apply([]FileChange{WriteFile(path, []byte("hello"), 0o644)})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("contents = %q, want %q", got, "hello")
	}
	info, _ := os.Stat(path)
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", info.Mode().Perm())
	}
}

func TestApply_WriteOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Apply([]FileChange{WriteFile(path, []byte("new"), 0o644)}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Errorf("contents = %q, want %q", got, "new")
	}
}

func TestApply_Delete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("bye"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Apply([]FileChange{DeleteFile(path)}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected %s removed, stat err = %v", path, err)
	}
}

func TestApply_DeleteAbsentIsNoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ghost.txt")
	if err := Apply([]FileChange{DeleteFile(path)}); err != nil {
		t.Errorf("Apply on absent path: %v, want nil", err)
	}
}

func TestApply_RejectsDuplicatePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	err := Apply([]FileChange{
		WriteFile(path, []byte("a"), 0o644),
		WriteFile(path, []byte("b"), 0o644),
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected duplicate-path error, got %v", err)
	}
}

func TestApply_RejectsEmptyPath(t *testing.T) {
	if err := Apply([]FileChange{WriteFile("", nil, 0)}); err == nil {
		t.Error("expected error for empty path")
	}
}

func TestApply_RejectsRelativePath(t *testing.T) {
	if err := Apply([]FileChange{WriteFile("relative.txt", nil, 0)}); err == nil {
		t.Error("expected error for relative path")
	}
}

func TestApply_RefusesSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests need Developer Mode on Windows")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(real, []byte("real"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	err := Apply([]FileChange{WriteFile(link, []byte("clobber"), 0o644)})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("expected symlink-refusal error, got %v", err)
	}

	// Original real file must be untouched.
	got, _ := os.ReadFile(real)
	if string(got) != "real" {
		t.Errorf("real file was modified: %q", got)
	}
}

func TestApply_RefusesDirectoryTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	err := Apply([]FileChange{WriteFile(target, []byte("x"), 0o644)})
	if err == nil || !strings.Contains(err.Error(), "non-regular") {
		t.Errorf("expected non-regular-file error, got %v", err)
	}
}

func TestApply_RollsBackOnSnapshotFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests need Developer Mode on Windows")
	}
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(a, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(filepath.Join(dir, "nowhere"), link); err != nil {
		t.Fatal(err)
	}

	// First change is fine, second hits a symlink and aborts the whole batch.
	err := Apply([]FileChange{
		WriteFile(a, []byte("modified"), 0o644),
		WriteFile(link, []byte("clobber"), 0o644),
	})
	if err == nil {
		t.Fatal("expected error")
	}

	got, _ := os.ReadFile(a)
	if string(got) != "original" {
		t.Errorf("a.txt should be untouched after rollback, got %q", got)
	}

	// No staged tempfiles should be left lying around.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".ccpm-staged-") {
			t.Errorf("staged temp file left behind: %s", e.Name())
		}
	}
}

func TestApply_WriteAndDeleteSucceedTogether(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "keep.txt")
	gone := filepath.Join(dir, "gone.txt")
	fresh := filepath.Join(dir, "fresh.txt")

	os.WriteFile(keep, []byte("v1"), 0o644)
	os.WriteFile(gone, []byte("bye"), 0o644)

	err := Apply([]FileChange{
		WriteFile(keep, []byte("v2"), 0o644),
		DeleteFile(gone),
		WriteFile(fresh, []byte("hi"), 0o644),
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if got, _ := os.ReadFile(keep); string(got) != "v2" {
		t.Errorf("keep = %q, want v2", got)
	}
	if _, err := os.Stat(gone); !os.IsNotExist(err) {
		t.Errorf("gone should be removed: %v", err)
	}
	if got, _ := os.ReadFile(fresh); string(got) != "hi" {
		t.Errorf("fresh = %q, want hi", got)
	}
}

func TestApply_CreatesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests need Developer Mode on Windows")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	link := filepath.Join(dir, "deep", "link.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Apply([]FileChange{SymlinkAt(link, src)}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != src {
		t.Errorf("symlink target = %q, want %q", target, src)
	}
}

func TestApply_ReplacesExistingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests need Developer Mode on Windows")
	}
	dir := t.TempDir()
	src1 := filepath.Join(dir, "src1.txt")
	src2 := filepath.Join(dir, "src2.txt")
	link := filepath.Join(dir, "link.txt")
	os.WriteFile(src1, []byte("a"), 0o644)
	os.WriteFile(src2, []byte("b"), 0o644)
	if err := os.Symlink(src1, link); err != nil {
		t.Fatal(err)
	}

	if err := Apply([]FileChange{SymlinkAt(link, src2)}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, _ := os.Readlink(link)
	if got != src2 {
		t.Errorf("after replace, target = %q, want %q", got, src2)
	}
}

func TestApply_SymlinkRollbackRestoresPriorSymlinkAfterCommitFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests need Developer Mode on Windows")
	}
	dir := t.TempDir()
	src1 := filepath.Join(dir, "src1.txt")
	src2 := filepath.Join(dir, "src2.txt")
	link := filepath.Join(dir, "link.txt")
	os.WriteFile(src1, []byte("a"), 0o644)
	os.WriteFile(src2, []byte("b"), 0o644)
	if err := os.Symlink(src1, link); err != nil {
		t.Fatal(err)
	}

	// Force a commit-time failure on the second change: its parent path is
	// an existing regular file, so MkdirAll inside commitSymlink will fail.
	// The first change (link swap) has already been committed by then —
	// rollback must restore it to point at src1.
	blocker := filepath.Join(dir, "blocker")
	os.WriteFile(blocker, []byte("x"), 0o644)
	bad := filepath.Join(blocker, "child.txt")

	err := Apply([]FileChange{
		SymlinkAt(link, src2),
		SymlinkAt(bad, src1),
	})
	if err == nil {
		t.Fatal("expected commit-time failure")
	}
	got, _ := os.Readlink(link)
	if got != src1 {
		t.Errorf("after rollback, target = %q, want %q (original)", got, src1)
	}
}

func TestApply_WriteRollbackRestoresFileAfterCommitFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests need Developer Mode on Windows")
	}
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	os.WriteFile(a, []byte("original"), 0o644)

	blocker := filepath.Join(dir, "blocker")
	os.WriteFile(blocker, []byte("x"), 0o644)
	bad := filepath.Join(blocker, "child.txt")

	err := Apply([]FileChange{
		WriteFile(a, []byte("new"), 0o644),
		SymlinkAt(bad, a),
	})
	if err == nil {
		t.Fatal("expected commit-time failure")
	}
	got, _ := os.ReadFile(a)
	if string(got) != "original" {
		t.Errorf("after rollback, a.txt = %q, want %q", got, "original")
	}
}

func TestApply_DeleteRemovesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests need Developer Mode on Windows")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	link := filepath.Join(dir, "link.txt")
	os.WriteFile(src, []byte("a"), 0o644)
	os.Symlink(src, link)

	if err := Apply([]FileChange{DeleteFile(link)}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("symlink should be removed: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("src should still exist: %v", err)
	}
}

func TestApply_DefaultModeWhenZero(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix mode bits don't translate cleanly")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := Apply([]FileChange{WriteFile(path, []byte("x"), 0)}); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != DefaultMode {
		t.Errorf("mode = %v, want default %v", info.Mode().Perm(), DefaultMode)
	}
}
