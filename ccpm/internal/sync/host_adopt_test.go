package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/manifest"
)

// seedHostAssets writes a small ~/.claude tree with one entry per dedupable
// kind. Returns the (HOME, .claude root) so tests can assert against it.
func seedHostAssets(t *testing.T) (home, claudeRoot string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	claudeRoot = filepath.Join(tmp, ".claude")
	for _, sub := range []string{"skills", "agents", "commands", "rules", "hooks"} {
		entry := filepath.Join(claudeRoot, sub, "matt-"+sub)
		if err := os.MkdirAll(entry, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(entry, "SKILL.md"), []byte("from host"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Hidden file at ~/.claude/skills/.DS_Store should be ignored.
	if err := os.WriteFile(filepath.Join(claudeRoot, "skills", ".DS_Store"), []byte("noise"), 0600); err != nil {
		t.Fatal(err)
	}
	// Plain file at root level — should not affect kind directories.
	if err := os.WriteFile(filepath.Join(claudeRoot, "settings.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	return tmp, claudeRoot
}

// TestScanHostUnadopted_FindsAllDedupableKinds verifies the scanner picks up
// one entry per kind from a freshly-seeded ~/.claude and skips hidden files.
func TestScanHostUnadopted_FindsAllDedupableKinds(t *testing.T) {
	seedHostAssets(t)

	entries, err := scanHostUnadopted(&manifest.Manifest{})
	if err != nil {
		t.Fatalf("scanHostUnadopted: %v", err)
	}

	want := map[manifest.AssetKind]string{
		manifest.KindSkill:   "matt-skills",
		manifest.KindAgent:   "matt-agents",
		manifest.KindCommand: "matt-commands",
		manifest.KindRule:    "matt-rules",
		manifest.KindHook:    "matt-hooks",
	}
	if got := len(entries); got != len(want) {
		t.Fatalf("want %d entries, got %d (%v)", len(want), got, entries)
	}
	for _, e := range entries {
		exp, ok := want[e.Kind]
		if !ok {
			t.Errorf("unexpected kind %q in results", e.Kind)
			continue
		}
		if e.Name != exp {
			t.Errorf("kind %s: got name %q, want %q", e.Kind, e.Name, exp)
		}
	}
}

// TestScanHostUnadopted_SkipsRegistered verifies idempotency — a host entry
// already in the manifest is not returned a second time.
func TestScanHostUnadopted_SkipsRegistered(t *testing.T) {
	seedHostAssets(t)

	m := &manifest.Manifest{}
	m.Add(manifest.Install{
		ID:    "matt-skills",
		Kind:  manifest.KindSkill,
		Scope: manifest.ScopeHost,
	})

	entries, err := scanHostUnadopted(m)
	if err != nil {
		t.Fatalf("scanHostUnadopted: %v", err)
	}
	for _, e := range entries {
		if e.Kind == manifest.KindSkill && e.Name == "matt-skills" {
			t.Errorf("matt-skills should have been skipped (already registered), got %+v", e)
		}
	}
	if len(entries) != 4 {
		t.Errorf("expected 4 remaining entries (4 other kinds), got %d", len(entries))
	}
}

// TestApplyGlobals_AdoptsHostAssets is the end-to-end test: a fresh profile
// + a seeded ~/.claude should produce symlinks under <profile>/<plural>/<name>.
func TestApplyGlobals_AdoptsHostAssets(t *testing.T) {
	home, claudeRoot := seedHostAssets(t)

	base := filepath.Join(home, ".ccpm")
	profileDir := filepath.Join(base, "profiles", "rocketium")
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Cascade ON by default — no config write needed.
	if err := ApplyGlobalsWithOptions(profileDir, "rocketium", Options{QuietAdoption: true}); err != nil {
		t.Fatalf("ApplyGlobalsWithOptions: %v", err)
	}

	for _, sub := range []string{"skills", "agents", "commands", "rules", "hooks"} {
		linkPath := filepath.Join(profileDir, sub, "matt-"+sub)
		hostPath := filepath.Join(claudeRoot, sub, "matt-"+sub)

		info, err := os.Lstat(linkPath)
		if err != nil {
			t.Fatalf("expected link at %s: %v", linkPath, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			// Windows fallback path tolerated; verify the resolved target.
			resolved, err := filepath.EvalSymlinks(linkPath)
			if err != nil {
				t.Fatalf("cannot resolve %s: %v", linkPath, err)
			}
			if resolved != hostPath {
				t.Errorf("%s resolved to %s, want %s", linkPath, resolved, hostPath)
			}
			continue
		}
		target, _ := os.Readlink(linkPath)
		if target != hostPath {
			t.Errorf("symlink at %s points to %s, want %s", linkPath, target, hostPath)
		}
	}

	// Manifest should have one ScopeHost entry per kind.
	m, err := manifest.Load()
	if err != nil {
		t.Fatalf("manifest.Load: %v", err)
	}
	hostCount := 0
	for _, inst := range m.Installs {
		if inst.Scope == manifest.ScopeHost {
			hostCount++
		}
	}
	if hostCount != 5 {
		t.Errorf("expected 5 host entries in manifest, got %d (%v)", hostCount, m.Installs)
	}
}

// TestApplyGlobals_OptOutSuppressesAdoption checks the SkipHostAdoption flag.
// When set, scanning is skipped and no manifest entries appear.
func TestApplyGlobals_OptOutSuppressesAdoption(t *testing.T) {
	home, _ := seedHostAssets(t)

	base := filepath.Join(home, ".ccpm")
	profileDir := filepath.Join(base, "profiles", "rocketium")
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		t.Fatal(err)
	}

	if err := ApplyGlobalsWithOptions(profileDir, "rocketium", Options{
		SkipHostAdoption: true,
		QuietAdoption:    true,
	}); err != nil {
		t.Fatalf("ApplyGlobalsWithOptions: %v", err)
	}

	skillLink := filepath.Join(profileDir, "skills", "matt-skills")
	if _, err := os.Lstat(skillLink); err == nil {
		t.Errorf("expected no skill link at %s (opt-out), but it exists", skillLink)
	}

	m, err := manifest.Load()
	if err != nil {
		t.Fatalf("manifest.Load: %v", err)
	}
	for _, inst := range m.Installs {
		if inst.Scope == manifest.ScopeHost {
			t.Errorf("expected no host entries with opt-out, got %+v", inst)
		}
	}
}

// TestApplyGlobals_PersistentSettingDisablesAdoption checks that the
// cascade_auto_adopt config setting actually disables adoption when set
// to false (mirrors what `ccpm config set cascade_auto_adopt false` does).
func TestApplyGlobals_PersistentSettingDisablesAdoption(t *testing.T) {
	home, _ := seedHostAssets(t)

	// Write a config with the setting explicitly disabled.
	cfgFalse := false
	cfg := &config.Config{
		Version:  "1",
		Profiles: map[string]config.ProfileConfig{},
		Settings: config.Settings{CascadeAutoAdopt: &cfgFalse},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("config.Save: %v", err)
	}

	base := filepath.Join(home, ".ccpm")
	profileDir := filepath.Join(base, "profiles", "rocketium")
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		t.Fatal(err)
	}

	if err := ApplyGlobalsWithOptions(profileDir, "rocketium", Options{QuietAdoption: true}); err != nil {
		t.Fatalf("ApplyGlobalsWithOptions: %v", err)
	}

	skillLink := filepath.Join(profileDir, "skills", "matt-skills")
	if _, err := os.Lstat(skillLink); err == nil {
		t.Errorf("expected no skill link at %s (config disabled), but it exists", skillLink)
	}
}

// TestApplyGlobals_ProfileLocalWinsOverHost checks shadowing precedence: if
// a profile already has a manifest entry with ScopeProfile and the same name
// as a host entry, the host entry must NOT clobber the profile's symlink.
func TestApplyGlobals_ProfileLocalWinsOverHost(t *testing.T) {
	home, _ := seedHostAssets(t)

	base := filepath.Join(home, ".ccpm")
	profileDir := filepath.Join(base, "profiles", "rocketium")
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Pre-register a profile-scoped skill named "matt-skills" (same as the
	// host entry seedHostAssets created). A profile-local source dir is
	// needed so the symlink resolves to something different.
	localSkillSrc := filepath.Join(home, "local-source", "matt-skills")
	if err := os.MkdirAll(localSkillSrc, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localSkillSrc, "SKILL.md"), []byte("from profile"), 0600); err != nil {
		t.Fatal(err)
	}
	// Manifest entry + symlink to profile-local source.
	m := &manifest.Manifest{}
	m.Add(manifest.Install{
		ID:       "matt-skills",
		Kind:     manifest.KindSkill,
		Scope:    manifest.ScopeProfile,
		Source:   localSkillSrc,
		Profiles: []string{"rocketium"},
	})
	if err := manifest.Save(m); err != nil {
		t.Fatal(err)
	}
	profileSkillDir := filepath.Join(profileDir, "skills")
	if err := os.MkdirAll(profileSkillDir, 0700); err != nil {
		t.Fatal(err)
	}
	profileLink := filepath.Join(profileSkillDir, "matt-skills")
	if err := os.Symlink(localSkillSrc, profileLink); err != nil {
		t.Fatal(err)
	}

	// Now run ApplyGlobals — host adoption should NOT replace the profile-local symlink.
	if err := ApplyGlobalsWithOptions(profileDir, "rocketium", Options{QuietAdoption: true}); err != nil {
		t.Fatalf("ApplyGlobalsWithOptions: %v", err)
	}

	target, err := os.Readlink(profileLink)
	if err != nil {
		t.Fatalf("readlink %s: %v", profileLink, err)
	}
	if target != localSkillSrc {
		t.Errorf("profile-local symlink was clobbered: target=%s, want %s", target, localSkillSrc)
	}
}

// TestAdoptHostEntriesReportsProfileAppendAsMutation pins the M16 fix: when an
// already-registered host entry gains a new profile in its Profiles list, the
// function must report the manifest as mutated so callers persist it.
func TestAdoptHostEntriesReportsProfileAppendAsMutation(t *testing.T) {
	_, claudeRoot := seedHostAssets(t)
	profileDir := filepath.Join(t.TempDir(), "profile")
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(claudeRoot, "skills", "matt-skills")
	entry := hostEntry{Kind: manifest.KindSkill, Name: "matt-skills", Src: src}

	m := &manifest.Manifest{}
	m.Add(manifest.Install{
		ID:       "matt-skills",
		Kind:     manifest.KindSkill,
		Scope:    manifest.ScopeHost,
		Source:   "host:" + src,
		Profiles: []string{"other-profile"},
	})

	mutated, err := adoptHostEntries(profileDir, "new-profile", []hostEntry{entry}, m)
	if err != nil {
		t.Fatal(err)
	}
	if !mutated {
		t.Error("Profiles-list append not reported as mutation — Save would be skipped (M16)")
	}
	inst := m.Find("matt-skills", manifest.KindSkill)
	if inst == nil || !containsProfile(inst.Profiles, "new-profile") {
		t.Errorf("profile not appended: %+v", inst)
	}

	// Re-running for the same profile must be a no-op (no spurious mutation).
	mutated, err = adoptHostEntries(profileDir, "new-profile", []hostEntry{entry}, m)
	if err != nil {
		t.Fatal(err)
	}
	if mutated {
		t.Error("idempotent re-adopt reported a mutation")
	}
}

// TestScanHostUnadoptedSkipsUnreadableDir pins the M16 EACCES fix: one
// unreadable kind directory must not abort the scan of the others.
func TestScanHostUnadoptedSkipsUnreadableDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root — chmod 0 is not an access barrier")
	}
	_, claudeRoot := seedHostAssets(t)

	locked := filepath.Join(claudeRoot, "skills")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

	m := &manifest.Manifest{}
	entries, err := scanHostUnadopted(m)
	if err != nil {
		t.Fatalf("scan aborted on unreadable dir: %v", err)
	}
	kinds := map[manifest.AssetKind]bool{}
	for _, e := range entries {
		kinds[e.Kind] = true
	}
	if kinds[manifest.KindSkill] {
		t.Error("unreadable skills dir still yielded entries")
	}
	for _, k := range []manifest.AssetKind{manifest.KindAgent, manifest.KindCommand, manifest.KindRule, manifest.KindHook} {
		if !kinds[k] {
			t.Errorf("kind %s missing — scan stopped early on the unreadable dir", k)
		}
	}
}
