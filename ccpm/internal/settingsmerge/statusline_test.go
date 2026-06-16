package settingsmerge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureDefaultStatusLine(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	settingsDir := filepath.Join(tmp, ".ccpm", "share", "settings")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fragPath := filepath.Join(settingsDir, "work.json")

	// First call on a profile with no statusLine anywhere: writes the default.
	wrote, err := EnsureDefaultStatusLine("work", DefaultStatusLineCommand)
	if err != nil {
		t.Fatalf("EnsureDefaultStatusLine error: %v", err)
	}
	if !wrote {
		t.Fatal("expected first call to write a statusLine")
	}
	frag, err := LoadJSON(fragPath)
	if err != nil {
		t.Fatal(err)
	}
	sl, ok := frag["statusLine"].(map[string]interface{})
	if !ok {
		t.Fatalf("statusLine not written as object: %#v", frag["statusLine"])
	}
	if sl["command"] != DefaultStatusLineCommand || sl["type"] != "command" {
		t.Fatalf("unexpected statusLine shape: %#v", sl)
	}

	// Second call is a no-op (idempotent) — already present.
	wrote, err = EnsureDefaultStatusLine("work", DefaultStatusLineCommand)
	if err != nil {
		t.Fatalf("second EnsureDefaultStatusLine error: %v", err)
	}
	if wrote {
		t.Fatal("expected second call to be a no-op")
	}
}

func TestEnsureDefaultStatusLineRespectsHost(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	if err := os.MkdirAll(filepath.Join(tmp, ".ccpm", "share", "settings"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Host ~/.claude/settings.json already defines a statusLine — ccpm must not
	// shadow the user's global choice.
	hostDir := filepath.Join(tmp, ".claude")
	if err := os.MkdirAll(hostDir, 0o755); err != nil {
		t.Fatal(err)
	}
	host := map[string]interface{}{
		"statusLine": map[string]interface{}{"type": "command", "command": "my-own-statusline"},
	}
	b, _ := json.MarshalIndent(host, "", "  ")
	if err := os.WriteFile(filepath.Join(hostDir, "settings.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}

	wrote, err := EnsureDefaultStatusLine("work", DefaultStatusLineCommand)
	if err != nil {
		t.Fatalf("EnsureDefaultStatusLine error: %v", err)
	}
	if wrote {
		t.Fatal("expected no write when host already has a statusLine")
	}
	if _, err := os.Stat(filepath.Join(tmp, ".ccpm", "share", "settings", "work.json")); !os.IsNotExist(err) {
		t.Fatalf("profile fragment should not have been created; stat err=%v", err)
	}
}
