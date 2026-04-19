package defaultclaude

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func withHome(t *testing.T, home string) {
	t.Helper()
	// os.UserHomeDir() reads different env vars per platform:
	//   * Unix/macOS: $HOME
	//   * Windows: %USERPROFILE% (falls back to %HOMEDRIVE%%HOMEPATH%)
	// t.Setenv handles restore + parallel-safety and fails the test if
	// the env can't be set.
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}
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

func TestImportDedupeLiveSymlinks(t *testing.T) {
	home := t.TempDir()
	withHome(t, home)
	ccpmBase := filepath.Join(home, ".ccpm")
	if err := os.MkdirAll(ccpmBase, 0755); err != nil {
		t.Fatal(err)
	}

	claudeSkills := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(claudeSkills, 0755); err != nil {
		t.Fatal(err)
	}
	realSkill := filepath.Join(t.TempDir(), "myskill")
	if err := os.MkdirAll(realSkill, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(realSkill, "SKILL.md"), "live-body\n")
	link := filepath.Join(claudeSkills, "myskill")
	if err := os.Symlink(realSkill, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	profile := filepath.Join(t.TempDir(), "prof")
	if err := os.MkdirAll(profile, 0755); err != nil {
		t.Fatal(err)
	}

	if _, err := Import(profile, ImportOptions{
		Targets:      []Target{TargetSkills},
		Dedupe:       true,
		ProfileName:  "prof",
		LiveSymlinks: true,
	}); err != nil {
		t.Fatalf("Import: %v", err)
	}

	storePath := filepath.Join(ccpmBase, "share", "skills", "myskill")
	fi, err := os.Lstat(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("expected store path to be a symlink")
	}
	gotResolved, err := filepath.EvalSymlinks(storePath)
	if err != nil {
		t.Fatal(err)
	}
	wantResolved, err := filepath.EvalSymlinks(realSkill)
	if err != nil {
		t.Fatal(err)
	}
	if gotResolved != wantResolved {
		t.Fatalf("store resolves to %q, want %q", gotResolved, wantResolved)
	}

	profSkill := filepath.Join(profile, "skills", "myskill", "SKILL.md")
	data, err := os.ReadFile(profSkill)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "live-body\n" {
		t.Fatalf("via profile chain read %q", data)
	}

	writeFile(t, filepath.Join(realSkill, "SKILL.md"), "updated\n")
	data2, err := os.ReadFile(profSkill)
	if err != nil {
		t.Fatal(err)
	}
	if string(data2) != "updated\n" {
		t.Fatalf("after live edit read %q", data2)
	}
}

func TestImportItemFilterSkills(t *testing.T) {
	home := t.TempDir()
	withHome(t, home)
	claudeSkills := filepath.Join(home, ".claude", "skills")
	writeFile(t, filepath.Join(claudeSkills, "keep", "SKILL.md"), "keep-body\n")
	writeFile(t, filepath.Join(claudeSkills, "drop", "SKILL.md"), "drop-body\n")

	profile := filepath.Join(t.TempDir(), "p")
	if err := os.MkdirAll(profile, 0755); err != nil {
		t.Fatal(err)
	}

	_, err := Import(profile, ImportOptions{
		Targets: []Target{TargetSkills},
		ItemFilter: map[Target]map[string]bool{
			TargetSkills: {"keep": true},
		},
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if _, err := os.Stat(filepath.Join(profile, "skills", "keep", "SKILL.md")); err != nil {
		t.Fatalf("selected skill missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(profile, "skills", "drop")); !os.IsNotExist(err) {
		t.Fatal("filtered-out skill should not have been copied")
	}
}

func TestImportMCPFromClaudeJSON(t *testing.T) {
	home := t.TempDir()
	withHome(t, home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(home, ".claude.json"), `{
		"mcpServers": {
			"gitnexus": {"type": "stdio", "command": "npx", "args": ["-y", "gitnexus"]}
		},
		"projects": {
			"/some/project": {
				"mcpServers": {
					"playwright": {"type": "stdio", "command": "npx", "args": ["@playwright/mcp"]}
				}
			}
		}
	}`)

	profile := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(profile, 0755); err != nil {
		t.Fatal(err)
	}

	_, err := Import(profile, ImportOptions{
		Targets:     []Target{TargetMCP},
		ProfileName: "work",
		MCPScope:    MCPImportScopeGlobal,
	})
	if err != nil {
		t.Fatalf("Import MCP: %v", err)
	}

	fragPath := filepath.Join(home, ".ccpm", "share", "mcp", "global.json")
	data, err := os.ReadFile(fragPath)
	if err != nil {
		t.Fatalf("reading fragment: %v", err)
	}
	body := string(data)
	if !contains(body, "gitnexus") || !contains(body, "playwright") {
		t.Fatalf("fragment missing entries: %s", body)
	}
}

func TestImportMCPFilter(t *testing.T) {
	home := t.TempDir()
	withHome(t, home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(home, ".claude.json"), `{
		"mcpServers": {
			"gitnexus":   {"type": "stdio", "command": "npx"},
			"playwright": {"type": "stdio", "command": "npx"}
		}
	}`)

	profile := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(profile, 0755); err != nil {
		t.Fatal(err)
	}

	_, err := Import(profile, ImportOptions{
		Targets:     []Target{TargetMCP},
		ProfileName: "work",
		MCPScope:    MCPImportScopeProfile,
		ItemFilter: map[Target]map[string]bool{
			TargetMCP: {"gitnexus": true},
		},
	})
	if err != nil {
		t.Fatalf("Import MCP filter: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".ccpm", "share", "mcp", "work.json"))
	if err != nil {
		t.Fatalf("reading fragment: %v", err)
	}
	body := string(data)
	if !contains(body, "gitnexus") {
		t.Fatalf("expected gitnexus in fragment, got %s", body)
	}
	if contains(body, "playwright") {
		t.Fatalf("did not expect playwright in fragment, got %s", body)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSnapshotFollowsSymlinkedDirectory(t *testing.T) {
	home := t.TempDir()
	withHome(t, home)

	claudeSkills := filepath.Join(home, ".claude", "skills")
	writeFile(t, filepath.Join(claudeSkills, "plain", "SKILL.md"), "plain-body")

	realSkill := filepath.Join(t.TempDir(), "external")
	writeFile(t, filepath.Join(realSkill, "SKILL.md"), "linked-body")
	writeFile(t, filepath.Join(realSkill, "nested", "note.md"), "nested-body")

	link := filepath.Join(claudeSkills, "linked")
	if err := os.Symlink(realSkill, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	fp, err := Snapshot([]Target{TargetSkills})
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	want := []string{
		"skills/plain/SKILL.md",
		"skills/linked/SKILL.md",
		"skills/linked/nested/note.md",
	}
	for _, k := range want {
		if _, ok := fp.Files[k]; !ok {
			t.Fatalf("missing fingerprint key %q; got %v", k, fp.Files)
		}
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
