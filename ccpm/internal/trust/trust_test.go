package trust

import (
	"os"
	"path/filepath"
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

// TestTrustSymlinkCanonicalization pins the M3 fix: grants are stored
// canonical, and a directory that was trusted but later replaced by a symlink
// to somewhere else must NOT remain trusted.
func TestTrustSymlinkCanonicalization(t *testing.T) {
	isolateHome(t)
	base := t.TempDir()

	real := filepath.Join(base, "real-project")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link-project")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	// Granting via the symlink must trust the canonical target...
	if err := MarkTrusted(link); err != nil {
		t.Fatal(err)
	}
	if !IsTrusted(real) {
		t.Error("canonical path not trusted after granting via symlink")
	}
	if !IsTrusted(link) {
		t.Error("symlinked path not trusted after granting via it")
	}

	// ...and re-pointing the symlink at a different directory must NOT carry
	// the grant along.
	evil := filepath.Join(base, "evil-project")
	if err := os.MkdirAll(evil, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(evil, link); err != nil {
		t.Fatal(err)
	}
	if IsTrusted(link) {
		t.Error("re-pointed symlink inherited trust grant — M3 regression")
	}
	if IsTrusted(evil) {
		t.Error("symlink target gained trust it was never granted")
	}

	// Revocation via the canonical path still works.
	if removed, err := Forget(real); err != nil || !removed {
		t.Errorf("Forget(real) = %v, %v; want removed", removed, err)
	}
	if IsTrusted(real) {
		t.Error("path still trusted after Forget")
	}
}

// TestFilterEnvAlways pins the M4 fix: PATH/loader/interpreter env vars are
// stripped from project layers even when the project is trusted.
func TestFilterEnvAlways(t *testing.T) {
	settings := map[string]interface{}{
		"model": "claude-fable-5",
		"env": map[string]interface{}{
			"PATH":          "/evil/bin:/usr/bin",
			"LD_PRELOAD":    "/evil/lib.so",
			"DYLD_LIBRARY_PATH": "/evil",
			"NODE_OPTIONS":  "--require /evil.js",
			"PYTHONSTARTUP": "/evil.py",
			"BASH_ENV":      "/evil.sh",
			"MY_API_URL":    "https://ok.example.com", // safe — must survive
		},
	}
	filtered, stripped := FilterEnvAlways(settings)
	if len(stripped) != 6 {
		t.Errorf("stripped = %v, want 6 names", stripped)
	}
	env, ok := filtered["env"].(map[string]interface{})
	if !ok {
		t.Fatal("env map missing after filtering")
	}
	if _, ok := env["MY_API_URL"]; !ok {
		t.Error("safe env var was stripped")
	}
	for _, bad := range []string{"PATH", "LD_PRELOAD", "DYLD_LIBRARY_PATH", "NODE_OPTIONS", "PYTHONSTARTUP", "BASH_ENV"} {
		if _, ok := env[bad]; ok {
			t.Errorf("dangerous env var %q survived FilterEnvAlways", bad)
		}
	}
	// Original input must be untouched (shallow-copy semantics).
	origEnv := settings["env"].(map[string]interface{})
	if _, ok := origEnv["PATH"]; !ok {
		t.Error("FilterEnvAlways mutated its input")
	}

	// All-dangerous env collapses to no env key at all.
	onlyBad := map[string]interface{}{"env": map[string]interface{}{"PATH": "/x"}}
	filtered, _ = FilterEnvAlways(onlyBad)
	if _, ok := filtered["env"]; ok {
		t.Error("empty env map should be dropped entirely")
	}

	// No env key: pass-through.
	noEnv := map[string]interface{}{"model": "m"}
	filtered, stripped = FilterEnvAlways(noEnv)
	if len(stripped) != 0 {
		t.Errorf("stripped = %v for settings without env", stripped)
	}
	if _, ok := filtered["model"]; !ok {
		t.Error("settings without env must pass through unchanged")
	}
}
