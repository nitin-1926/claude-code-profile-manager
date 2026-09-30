package profilelife

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/defaultclaude"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/manifest"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
)

func sandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := share.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	return home
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A profile named "global" shares share/mcp/global.json with the global MCP
// layer; removing or renaming it must not take the global servers along.
func TestGlobalMCPFragmentIsNeverMovedOrDeleted(t *testing.T) {
	sandbox(t)
	mcpDir, _ := share.MCPDir()
	globalFrag := filepath.Join(mcpDir, "global.json")
	write(t, globalFrag, `{"everyone":{"command":"x"}}`)

	if err := Rename("global", "renamed"); err != nil {
		t.Fatal(err)
	}
	if err := Remove("global"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(globalFrag); err != nil {
		t.Fatalf("global MCP fragment gone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(mcpDir, "renamed.json")); err == nil {
		t.Fatal("global MCP servers were moved into a profile fragment")
	}
}

func TestCopy_MergesDstWinsAndJoinsProfileScopedEntries(t *testing.T) {
	sandbox(t)
	settingsDir, _ := share.SettingsDir()
	mcpDir, _ := share.MCPDir()
	write(t, filepath.Join(settingsDir, "src.json"), `{"model":"src","theme":"dark"}`)
	write(t, filepath.Join(settingsDir, "src.owned.json"), `{"keys":["theme"]}`)
	write(t, filepath.Join(settingsDir, "dst.json"), `{"model":"dst"}`)
	write(t, filepath.Join(mcpDir, "src.json"), `{"a":{"command":"src-a"},"b":{"command":"src-b"}}`)
	write(t, filepath.Join(mcpDir, "dst.json"), `{"a":{"command":"dst-a"}}`)
	if err := manifest.Save(&manifest.Manifest{Version: "1", Installs: []manifest.Install{
		{ID: "sk", Kind: manifest.KindSkill, Scope: manifest.ScopeProfile, Profiles: []string{"src"}},
		{ID: "ag", Kind: manifest.KindAgent, Scope: manifest.ScopeProfile, Profiles: []string{"src"}},
	}}); err != nil {
		t.Fatal(err)
	}

	targets := []defaultclaude.Target{defaultclaude.TargetSettings, defaultclaude.TargetMCP, defaultclaude.TargetSkills}
	if err := Copy("src", "dst", targets, false); err != nil {
		t.Fatal(err)
	}

	s, _ := settingsmerge.LoadJSON(filepath.Join(settingsDir, "dst.json"))
	if s["model"] != "dst" || s["theme"] != "dark" {
		t.Errorf("settings merge: want dst model kept + src theme added, got %v", s)
	}
	owned, _ := settingsmerge.LoadOwnedKeys(filepath.Join(settingsDir, "dst.json"))
	if _, ok := owned["theme"]; !ok {
		t.Errorf("owned keys not copied: %v", owned)
	}
	mcp, _ := settingsmerge.LoadJSON(filepath.Join(mcpDir, "dst.json"))
	if a, _ := mcp["a"].(map[string]interface{}); a["command"] != "dst-a" || mcp["b"] == nil {
		t.Errorf("MCP merge: want dst's a kept + src's b added, got %v", mcp)
	}
	m, _ := manifest.Load()
	if !slices.Equal(m.Find("sk", manifest.KindSkill).Profiles, []string{"src", "dst"}) {
		t.Errorf("skill entry not joined: %+v", m.Find("sk", manifest.KindSkill))
	}
	if !slices.Equal(m.Find("ag", manifest.KindAgent).Profiles, []string{"src"}) {
		t.Errorf("agent entry joined though agents were not copied: %+v", m.Find("ag", manifest.KindAgent))
	}
}

func TestCopy_FreshDiscardsStaleDstState(t *testing.T) {
	sandbox(t)
	mcpDir, _ := share.MCPDir()
	write(t, filepath.Join(mcpDir, "dst.json"), `{"stale":{"command":"leak"}}`)

	if err := Copy("src", "dst", defaultclaude.AllTargets(), true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(mcpDir, "dst.json")); err == nil {
		t.Fatal("fresh copy kept a stale MCP fragment under the new name")
	}
}

func TestCopyLinks_RecreatesEscapingLinksOnly(t *testing.T) {
	home := sandbox(t)
	store := filepath.Join(home, "store", "my-skill")
	write(t, filepath.Join(store, "SKILL.md"), "x")
	src := filepath.Join(home, "src", "skills")
	dst := filepath.Join(home, "dst", "skills")
	write(t, filepath.Join(src, "local", "SKILL.md"), "y")
	if err := os.Symlink(store, filepath.Join(src, "my-skill")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(src, "local"), filepath.Join(src, "internal-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "gone"), filepath.Join(src, "dangling")); err != nil {
		t.Fatal(err)
	}

	if err := CopyLinks(src, dst, false); err != nil {
		t.Fatal(err)
	}
	if target, err := os.Readlink(filepath.Join(dst, "my-skill")); err != nil || target != store {
		t.Errorf("escaping link not recreated: target=%q err=%v", target, err)
	}
	for _, name := range []string{"internal-link", "dangling"} {
		if _, err := os.Lstat(filepath.Join(dst, name)); err == nil {
			t.Errorf("%s should not be recreated by CopyLinks", name)
		}
	}
}
