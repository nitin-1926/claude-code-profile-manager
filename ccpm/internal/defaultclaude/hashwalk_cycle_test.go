package defaultclaude

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSnapshotSurvivesSymlinkCycle: hashWalk followed symlinked directories
// with no cycle guard, so `skills/a/loop -> skills` recursed until the path
// grew too long — hanging doctor, `import default` and the drift nudge on
// `ccpm run`. The cycle must be skipped, with the real files still hashed.
func TestSnapshotSurvivesSymlinkCycle(t *testing.T) {
	home := t.TempDir()
	withHome(t, home)
	skills := filepath.Join(home, ".claude", "skills")
	writeFile(t, filepath.Join(skills, "a", "SKILL.md"), "a")
	if err := os.Symlink(skills, filepath.Join(skills, "a", "loop")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	// A second, non-cyclic link to the same place must still be walked.
	writeFile(t, filepath.Join(home, "elsewhere", "b", "SKILL.md"), "b")
	if err := os.Symlink(filepath.Join(home, "elsewhere", "b"), filepath.Join(skills, "b")); err != nil {
		t.Fatal(err)
	}

	type result struct {
		fp  *Fingerprint
		err error
	}
	done := make(chan result, 1)
	go func() {
		fp, err := Snapshot([]Target{TargetSkills})
		done <- result{fp, err}
	}()
	var r result
	select {
	case r = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Snapshot did not return within 10s: symlink cycle not detected")
	}
	if r.err != nil {
		t.Fatalf("Snapshot: %v", r.err)
	}
	for _, want := range []string{"skills/a/SKILL.md", "skills/b/SKILL.md"} {
		if _, ok := r.fp.Files[want]; !ok {
			t.Errorf("fingerprint missing %s: %v", want, r.fp.Files)
		}
	}
	if len(r.fp.Files) != 2 {
		t.Errorf("fingerprint has %d files, want 2 (cycle walked into?): %v", len(r.fp.Files), r.fp.Files)
	}
}
