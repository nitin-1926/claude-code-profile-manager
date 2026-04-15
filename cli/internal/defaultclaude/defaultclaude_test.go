package defaultclaude

import (
	"os"
	"path/filepath"
	"testing"
)

func withHome(t *testing.T, home string) {
	t.Helper()
	prev, had := os.LookupEnv("HOME")
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatalf("setenv HOME: %v", err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("HOME", prev)
		} else {
			_ = os.Unsetenv("HOME")
		}
	})
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
}

func seedDefaultTree(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "settings.json"), `{"theme":"dark"}`)
	writeFile(t, filepath.Join(root, "skills", "greet.md"), "# Greet\n")
	writeFile(t, filepath.Join(root, "hooks", "pre.sh"), "#!/bin/sh\necho pre\n")
	writeFile(t, filepath.Join(root, "plugins", "big.bin"), "ignored")
}

func TestParseTargetsDefaults(t *testing.T) {
	got, err := ParseTargets(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(DefaultTargets()) {
		t.Fatalf("expected default set, got %v", got)
	}
}

func TestParseTargetsUnknown(t *testing.T) {
	if _, err := ParseTargets([]string{"bogus"}); err == nil {
		t.Fatal("expected error for unknown target")
	}
}

func TestImportCopiesSelectedTargets(t *testing.T) {
	home := t.TempDir()
	withHome(t, home)
	seedDefaultTree(t, filepath.Join(home, ".claude"))

	profile := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(profile, 0755); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}

	plan, err := Import(profile, ImportOptions{
		Targets: []Target{TargetSkills, TargetHooks, TargetSettings},
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(plan.Actions) != 3 {
		t.Fatalf("expected 3 actions, got %d", len(plan.Actions))
	}

	if _, err := os.Stat(filepath.Join(profile, "skills", "greet.md")); err != nil {
		t.Fatalf("skill not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(profile, "hooks", "pre.sh")); err != nil {
		t.Fatalf("hook not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(profile, "settings.json.ccpm-import")); err != nil {
		t.Fatalf("settings not staged: %v", err)
	}
	if _, err := os.Stat(filepath.Join(profile, "plugins")); !os.IsNotExist(err) {
		t.Fatalf("plugins should not be imported without explicit opt-in")
	}
}

func TestImportDryRunWritesNothing(t *testing.T) {
	home := t.TempDir()
	withHome(t, home)
	seedDefaultTree(t, filepath.Join(home, ".claude"))

	profile := t.TempDir()
	plan, err := Import(profile, ImportOptions{
		Targets: []Target{TargetSkills, TargetSettings},
		DryRun:  true,
	})
	if err != nil {
		t.Fatalf("Import dry-run: %v", err)
	}
	if len(plan.Actions) == 0 {
		t.Fatal("expected plan actions even in dry-run")
	}
	if _, err := os.Stat(filepath.Join(profile, "skills")); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not create files")
	}
	if _, err := os.Stat(filepath.Join(profile, "settings.json.ccpm-import")); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not stage settings")
	}
}

func TestImportPreservesExistingWithoutForce(t *testing.T) {
	home := t.TempDir()
	withHome(t, home)
	seedDefaultTree(t, filepath.Join(home, ".claude"))

	profile := t.TempDir()
	existing := filepath.Join(profile, "skills", "greet.md")
	writeFile(t, existing, "# Kept\n")

	_, err := Import(profile, ImportOptions{Targets: []Target{TargetSkills}})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	data, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "# Kept\n" {
		t.Fatalf("existing file was overwritten: %q", string(data))
	}
}

func TestImportForceOverwrites(t *testing.T) {
	home := t.TempDir()
	withHome(t, home)
	seedDefaultTree(t, filepath.Join(home, ".claude"))

	profile := t.TempDir()
	existing := filepath.Join(profile, "skills", "greet.md")
	writeFile(t, existing, "# Kept\n")

	if _, err := Import(profile, ImportOptions{
		Targets: []Target{TargetSkills},
		Force:   true,
	}); err != nil {
		t.Fatalf("Import --force: %v", err)
	}

	data, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "# Greet\n" {
		t.Fatalf("expected overwrite, got %q", string(data))
	}
}

func TestSnapshotAndCompare(t *testing.T) {
	home := t.TempDir()
	withHome(t, home)
	claudeRoot := filepath.Join(home, ".claude")
	writeFile(t, filepath.Join(claudeRoot, "skills", "a.md"), "A")
	writeFile(t, filepath.Join(claudeRoot, "skills", "b.md"), "B")

	first, err := Snapshot([]Target{TargetSkills})
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(first.Files) != 2 {
		t.Fatalf("expected 2 tracked files, got %d", len(first.Files))
	}

	writeFile(t, filepath.Join(claudeRoot, "skills", "b.md"), "B-updated")
	writeFile(t, filepath.Join(claudeRoot, "skills", "c.md"), "C")
	if err := os.Remove(filepath.Join(claudeRoot, "skills", "a.md")); err != nil {
		t.Fatalf("remove: %v", err)
	}

	second, err := Snapshot([]Target{TargetSkills})
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	d := Compare(first, second)
	if !d.HasChanges() {
		t.Fatal("expected drift")
	}
	if len(d.Added) != 1 || d.Added[0] != "skills/c.md" {
		t.Fatalf("unexpected Added: %v", d.Added)
	}
	if len(d.Removed) != 1 || d.Removed[0] != "skills/a.md" {
		t.Fatalf("unexpected Removed: %v", d.Removed)
	}
	if len(d.Modified) != 1 || d.Modified[0] != "skills/b.md" {
		t.Fatalf("unexpected Modified: %v", d.Modified)
	}
}

func TestFingerprintRoundTrip(t *testing.T) {
	home := t.TempDir()
	withHome(t, home)

	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"x":1}`)

	fp, err := Snapshot([]Target{TargetSettings})
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if err := SaveFingerprint(fp); err != nil {
		t.Fatalf("SaveFingerprint: %v", err)
	}

	loaded, err := LoadFingerprint()
	if err != nil {
		t.Fatalf("LoadFingerprint: %v", err)
	}
	if loaded == nil || len(loaded.Files) != 1 {
		t.Fatalf("unexpected fingerprint: %+v", loaded)
	}
}

func TestLoadFingerprintMissing(t *testing.T) {
	withHome(t, t.TempDir())
	fp, err := LoadFingerprint()
	if err != nil {
		t.Fatalf("LoadFingerprint missing: %v", err)
	}
	if fp != nil {
		t.Fatalf("expected nil fingerprint when none stored, got %+v", fp)
	}
}
