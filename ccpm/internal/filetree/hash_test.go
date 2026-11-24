package filetree

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func mustHash(t *testing.T, p string) string {
	t.Helper()
	h, err := ContentHash(p, 1<<20, 1000)
	if err != nil {
		t.Fatalf("ContentHash(%s): %v", p, err)
	}
	return h
}

func TestContentHash(t *testing.T) {
	tmp := t.TempDir()
	base := map[string]string{"SKILL.md": "hello", "scripts/run.sh": "echo hi"}
	writeTree(t, filepath.Join(tmp, "a"), base)
	writeTree(t, filepath.Join(tmp, "b"), base)
	writeTree(t, filepath.Join(tmp, "content"), map[string]string{"SKILL.md": "hello", "scripts/run.sh": "echo bye"})
	writeTree(t, filepath.Join(tmp, "renamed"), map[string]string{"SKILL.md": "hello", "scripts/go.sh": "echo hi"})
	writeTree(t, filepath.Join(tmp, "extra"), map[string]string{"SKILL.md": "hello", "scripts/run.sh": "echo hi", "x": ""})

	a := mustHash(t, filepath.Join(tmp, "a"))
	if b := mustHash(t, filepath.Join(tmp, "b")); a != b {
		t.Error("identical trees hash differently")
	}
	for _, d := range []string{"content", "renamed", "extra"} {
		if mustHash(t, filepath.Join(tmp, d)) == a {
			t.Errorf("tree %q hashes like the original", d)
		}
	}

	// A symlink to the tree hashes as the tree itself.
	if err := os.Symlink(filepath.Join(tmp, "a"), filepath.Join(tmp, "link")); err != nil {
		t.Logf("symlink not supported, skipping the symlinked-root check: %v", err)
	} else if mustHash(t, filepath.Join(tmp, "link")) != a {
		t.Error("a symlinked root is not followed")
	}

	// Files: same bytes hash equal, different bytes differ.
	f1, f2, f3 := filepath.Join(tmp, "a", "SKILL.md"), filepath.Join(tmp, "b", "SKILL.md"), filepath.Join(tmp, "a", "scripts", "run.sh")
	if mustHash(t, f1) != mustHash(t, f2) || mustHash(t, f1) == mustHash(t, f3) {
		t.Error("file hashes do not follow content")
	}
}

// A symlink inside the tree is hashed by its target string: changing what it
// points at outside the tree must not change the hash (nothing outside is read).
func TestContentHashDoesNotFollowInnerSymlinks(t *testing.T) {
	tmp := t.TempDir()
	outside := filepath.Join(tmp, "outside.txt")
	writeTree(t, tmp, map[string]string{"outside.txt": "v1", "tree/SKILL.md": "x"})
	if err := os.Symlink(outside, filepath.Join(tmp, "tree", "ref")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	before := mustHash(t, filepath.Join(tmp, "tree"))
	if err := os.WriteFile(outside, []byte("v2, longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	if mustHash(t, filepath.Join(tmp, "tree")) != before {
		t.Error("hash changed with content outside the tree: an inner symlink was followed")
	}
}

func TestContentHashBudget(t *testing.T) {
	tmp := t.TempDir()
	writeTree(t, tmp, map[string]string{"a": "12345", "b": "12345", "c": ""})
	if _, err := ContentHash(tmp, 9, 100); !errors.Is(err, ErrTooLarge) {
		t.Errorf("byte budget: err = %v, want ErrTooLarge", err)
	}
	if _, err := ContentHash(tmp, 100, 3); !errors.Is(err, ErrTooLarge) {
		t.Errorf("entry budget: err = %v, want ErrTooLarge", err)
	}
	if _, err := ContentHash(tmp, 10, 4); err != nil {
		t.Errorf("exactly at budget: %v", err)
	}
}
