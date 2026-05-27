package consolidate

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeFile is a small helper that creates parent dirs and writes contents.
func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// fakeHome builds a minimal fake $HOME with ~/.claude and ~/.ccpm scaffolding
// and returns the home dir. Caller restores HOME via t.Cleanup.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Windows resolves os.UserHomeDir() via %USERPROFILE%, not $HOME.
	t.Setenv("USERPROFILE", home)
	return home
}

func TestInventoryReadsHostScopes(t *testing.T) {
	home := fakeHome(t)
	writeFile(t, filepath.Join(home, ".claude", "skills", "alpha", "SKILL.md"), "stub")
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{
		"enabledPlugins": {"vercel@official": true},
		"permissions": {"allow": ["Bash(ls:*)"]}
	}`)
	writeFile(t, filepath.Join(home, ".claude.json"), `{"mcpServers": {"gitnexus": {}}}`)

	snap, err := Inventory()
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if len(snap.HostSkills) != 1 || snap.HostSkills[0].Name != "alpha" {
		t.Errorf("expected one host skill 'alpha'; got %#v", snap.HostSkills)
	}
	if len(snap.HostPlugins) != 1 || snap.HostPlugins[0] != "vercel@official" {
		t.Errorf("expected vercel plugin; got %v", snap.HostPlugins)
	}
	if len(snap.HostMCPs) != 1 || snap.HostMCPs[0] != "gitnexus" {
		t.Errorf("expected gitnexus mcp; got %v", snap.HostMCPs)
	}
	if snap.CCPMPresent {
		t.Errorf("ccpm should not be detected without ~/.ccpm/")
	}
}

func TestDetectDanglingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests need Developer Mode on Windows")
	}
	home := fakeHome(t)
	skillsDir := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(skillsDir, "ghost")
	if err := os.Symlink(filepath.Join(home, "does-not-exist"), link); err != nil {
		t.Fatal(err)
	}

	snap, err := Inventory()
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	issues := Detect(snap)
	found := false
	for _, i := range issues {
		if i.Category == "dangling" && i.Asset == "ghost" {
			found = true
			if i.AutoFix == nil {
				t.Errorf("dangling symlink should have AutoFix")
			}
		}
	}
	if !found {
		t.Errorf("expected dangling issue for 'ghost'; got %v", issues)
	}
}

func TestDetectGhostManifestEntry(t *testing.T) {
	home := fakeHome(t)
	// Live profile 'work' but manifest references 'rocketium'
	if err := os.MkdirAll(filepath.Join(home, ".ccpm", "profiles", "work"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{Version: 2, Installs: []ManifestEntry{{
		ID:       "tdd",
		Kind:     "skill",
		Scope:    "host",
		Source:   "host:/x",
		Profiles: []string{"work", "rocketium"},
	}}}
	b, _ := json.Marshal(manifest)
	writeFile(t, filepath.Join(home, ".ccpm", "installs.json"), string(b))

	snap, err := Inventory()
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	issues := Detect(snap)
	var got []Issue
	for _, i := range issues {
		if i.Category == "ghost" {
			got = append(got, i)
		}
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 ghost issue; got %d (%v)", len(got), got)
	}
	if got[0].Asset != "tdd" {
		t.Errorf("expected ghost asset=tdd; got %s", got[0].Asset)
	}
}

func TestDetectBrokenEmptyFile(t *testing.T) {
	home := fakeHome(t)
	writeFile(t, filepath.Join(home, ".claude", "skills", "broken-skill", "SKILL.md"), "")
	snap, err := Inventory()
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	issues := Detect(snap)
	found := false
	for _, i := range issues {
		if i.Category == "broken" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected broken issue for empty SKILL.md; got %v", issues)
	}
}

func TestRunDryRunDoesNotApplyAutoFix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests need Developer Mode on Windows")
	}
	home := fakeHome(t)
	skillsDir := filepath.Join(home, ".claude", "skills")
	_ = os.MkdirAll(skillsDir, 0o755)
	link := filepath.Join(skillsDir, "ghost")
	_ = os.Symlink(filepath.Join(home, "missing"), link)

	var buf bytes.Buffer
	if err := Run(Options{DryRun: true, Out: &buf}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Symlink must still exist
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("dry-run should not have removed symlink; err=%v", err)
	}
}

func TestRunFixAppliesAutoFix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests need Developer Mode on Windows")
	}
	home := fakeHome(t)
	skillsDir := filepath.Join(home, ".claude", "skills")
	_ = os.MkdirAll(skillsDir, 0o755)
	link := filepath.Join(skillsDir, "ghost")
	_ = os.Symlink(filepath.Join(home, "missing"), link)

	var buf bytes.Buffer
	if err := Run(Options{Fix: true, Out: &buf}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := os.Lstat(link); err == nil {
		t.Errorf("--fix should have removed dangling symlink")
	}
}

func TestInstallSkillExtractsEmbedded(t *testing.T) {
	home := fakeHome(t)
	if err := InstallSkill(); err != nil {
		t.Fatalf("InstallSkill: %v", err)
	}
	skillMd := filepath.Join(home, ".claude", "skills", "consolidate-claude-assets", "SKILL.md")
	info, err := os.Stat(skillMd)
	if err != nil {
		t.Fatalf("expected SKILL.md at %s: %v", skillMd, err)
	}
	if info.Size() == 0 {
		t.Errorf("extracted SKILL.md is empty")
	}
	// Bundled scripts should be executable
	script := filepath.Join(home, ".claude", "skills", "consolidate-claude-assets", "scripts", "inventory.sh")
	scriptInfo, err := os.Stat(script)
	if err != nil {
		t.Fatalf("expected script at %s: %v", script, err)
	}
	if scriptInfo.Mode().Perm()&0o100 == 0 {
		t.Errorf("script should be executable, got mode=%v", scriptInfo.Mode())
	}
}

func TestInstallSkillRefusesOverwrite(t *testing.T) {
	fakeHome(t)
	if err := InstallSkill(); err != nil {
		t.Fatalf("first InstallSkill: %v", err)
	}
	if err := InstallSkill(); err == nil {
		t.Errorf("second InstallSkill should fail (no overwrite); got nil")
	}
}
