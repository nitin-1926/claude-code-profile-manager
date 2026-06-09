package trust

import (
	"testing"
)

// isolateHome points HOME and USERPROFILE at a fresh temp dir so the trust
// list (resolved via os.UserHomeDir()) is isolated on every OS — including
// Windows, where home resolves via %USERPROFILE%, not $HOME. Without this the
// tests read/write the real user home on Windows.
func isolateHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

// TestFilterStripsDangerousKeysWhenUntrusted is the core security property: a
// project that has not been explicitly trusted must not be able to contribute
// hooks/permissions/mcpServers/env/etc. into the merge.
func TestFilterStripsDangerousKeysWhenUntrusted(t *testing.T) {
	isolateHome(t)

	settings := map[string]interface{}{
		"hooks":          map[string]interface{}{"PreToolUse": "rm -rf /"},
		"permissions":    map[string]interface{}{"allow": []string{"*"}},
		"mcpServers":     map[string]interface{}{"evil": "x"},
		"env":            map[string]interface{}{"SECRET": "y"},
		"enabledPlugins": []string{"z"},
		"statusLine":     "danger",
		"model":          "claude-opus-4-8", // safe key — must survive
		"theme":          "dark",            // safe key — must survive
	}

	filtered, stripped := FilterProjectLayer(settings, "/some/untrusted/project")

	for _, dangerous := range DangerousKeys {
		if _, ok := filtered[dangerous]; ok {
			t.Errorf("dangerous key %q survived the untrusted filter", dangerous)
		}
	}
	if _, ok := filtered["model"]; !ok {
		t.Error("safe key \"model\" was incorrectly stripped")
	}
	if _, ok := filtered["theme"]; !ok {
		t.Error("safe key \"theme\" was incorrectly stripped")
	}
	if len(stripped) != len(DangerousKeys) {
		t.Errorf("stripped %d keys, want %d (%v)", len(stripped), len(DangerousKeys), stripped)
	}
}

// TestFilterPassesThroughWhenTrusted verifies an explicitly-trusted project
// keeps all of its keys.
func TestFilterPassesThroughWhenTrusted(t *testing.T) {
	isolateHome(t)

	root := t.TempDir()
	if err := MarkTrusted(root); err != nil {
		t.Fatalf("MarkTrusted: %v", err)
	}
	if !IsTrusted(root) {
		t.Fatal("project should be trusted after MarkTrusted")
	}

	settings := map[string]interface{}{"hooks": "keep-me", "model": "x"}
	filtered, stripped := FilterProjectLayer(settings, root)
	if len(stripped) != 0 {
		t.Errorf("trusted project should strip nothing, stripped %v", stripped)
	}
	if _, ok := filtered["hooks"]; !ok {
		t.Error("trusted project lost its hooks key")
	}
}

// TestEmptyProjectRootIsTrusted: no project context means nothing to strip.
func TestEmptyProjectRootIsTrusted(t *testing.T) {
	isolateHome(t)
	if !IsTrusted("") {
		t.Error("empty projectRoot should be treated as trusted (not-applicable)")
	}
}

// TestMarkTrustedIsIdempotentAndForgettable rounds out the lifecycle.
func TestMarkTrustedIsIdempotentAndForgettable(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()

	if err := MarkTrusted(root); err != nil {
		t.Fatalf("MarkTrusted: %v", err)
	}
	if err := MarkTrusted(root); err != nil {
		t.Fatalf("MarkTrusted (2nd): %v", err)
	}
	all, err := All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 trusted entry after double-add, got %d", len(all))
	}

	removed, err := Forget(root)
	if err != nil || !removed {
		t.Fatalf("Forget: removed=%v err=%v", removed, err)
	}
	if IsTrusted(root) {
		t.Error("project still trusted after Forget")
	}
}
