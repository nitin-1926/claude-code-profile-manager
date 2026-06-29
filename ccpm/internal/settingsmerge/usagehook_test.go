package settingsmerge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureUsageHook(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	settingsDir := filepath.Join(tmp, ".ccpm", "share", "settings")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fragPath := filepath.Join(settingsDir, "work.json")

	// First call writes a SessionEnd hook running the usage-sync command.
	wrote, err := EnsureUsageHook("work")
	if err != nil {
		t.Fatalf("EnsureUsageHook error: %v", err)
	}
	if !wrote {
		t.Fatal("expected first call to write a hook")
	}
	frag, err := LoadJSON(fragPath)
	if err != nil {
		t.Fatal(err)
	}
	if !usageHookPresent(frag) {
		t.Fatalf("usage hook not present after write: %#v", frag["hooks"])
	}

	// Second call is idempotent — must not duplicate the hook.
	wrote, err = EnsureUsageHook("work")
	if err != nil {
		t.Fatalf("second EnsureUsageHook error: %v", err)
	}
	if wrote {
		t.Fatal("expected second call to be a no-op")
	}
	frag, _ = LoadJSON(fragPath)
	hooks := frag["hooks"].(map[string]interface{})
	events := hooks["SessionEnd"].([]interface{})
	if len(events) != 1 {
		t.Fatalf("expected exactly 1 SessionEnd entry, got %d", len(events))
	}

	// The owned-keys file must record hooks.SessionEnd so materialize re-asserts it.
	owned, err := os.ReadFile(ownedKeysPath(fragPath))
	if err != nil {
		t.Fatalf("owned keys file missing: %v", err)
	}
	if !strings.Contains(string(owned), "hooks.SessionEnd") {
		t.Fatalf("owned keys missing hooks.SessionEnd: %s", owned)
	}
}
