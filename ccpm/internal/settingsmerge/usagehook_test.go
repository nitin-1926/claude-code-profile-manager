package settingsmerge

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
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

// TestEnsureUsageHookKeepsHostSessionEndHooks pins the data-loss fix: the
// fragment owns hooks.SessionEnd, and DeepMerge replaces arrays rather than
// concatenating them, so a fragment carrying only our entry silently erased
// every SessionEnd hook the user had in ~/.claude/settings.json.
func TestEnsureUsageHookKeepsHostSessionEndHooks(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	claudeDir := filepath.Join(tmp, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const hostHook = `{"hooks":{"SessionEnd":[{"matcher":"","hooks":[` +
		`{"type":"command","command":"~/.claude/hooks/brain-capture.mjs"}]}]}}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(hostHook), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := EnsureUsageHook("work"); err != nil {
		t.Fatalf("EnsureUsageHook: %v", err)
	}

	shareDir, err := share.SettingsDir()
	if err != nil {
		t.Fatal(err)
	}
	fragPath := filepath.Join(shareDir, "work.json")
	frag, err := LoadJSON(fragPath)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate what materialize does: host layer merged, then the fragment
	// merged over it, then owned keys re-asserted.
	host, err := loadHostClaudeSettings()
	if err != nil {
		t.Fatal(err)
	}
	merged := DeepMerge(map[string]interface{}{}, host)
	merged = DeepMerge(merged, frag)
	owned, err := LoadOwnedKeys(fragPath)
	if err != nil {
		t.Fatal(err)
	}
	merged = applyOwnedKeys(merged, frag, owned)

	got := commandsInSessionEnd(merged)
	wantBoth := []string{"~/.claude/hooks/brain-capture.mjs", UsageSyncCommand}
	for _, want := range wantBoth {
		if !slices.Contains(got, want) {
			t.Errorf("materialized SessionEnd lost %q — got %v", want, got)
		}
	}

	// A second call must not duplicate either entry.
	if _, err := EnsureUsageHook("work"); err != nil {
		t.Fatalf("second EnsureUsageHook: %v", err)
	}
	frag, _ = LoadJSON(fragPath)
	if got := commandsInSessionEnd(frag); len(got) != 2 {
		t.Errorf("second call changed the fragment: %v", got)
	}
}

// commandsInSessionEnd flattens every hooks.SessionEnd[*].hooks[*].command.
func commandsInSessionEnd(doc map[string]interface{}) []string {
	var out []string
	root, _ := doc["hooks"].(map[string]interface{})
	events, _ := root["SessionEnd"].([]interface{})
	for _, e := range events {
		em, _ := e.(map[string]interface{})
		hooks, _ := em["hooks"].([]interface{})
		for _, h := range hooks {
			if hm, ok := h.(map[string]interface{}); ok {
				if cmd, _ := hm["command"].(string); cmd != "" {
					out = append(out, cmd)
				}
			}
		}
	}
	return out
}
