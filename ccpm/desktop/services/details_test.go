//go:build darwin

package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// breakProfileSettings writes an unparseable settings.json into the synthetic
// profile — the shape a hand edit with a trailing comma leaves behind.
func breakProfileSettings(t *testing.T, name string) {
	t.Helper()
	dir := filepath.Join(os.Getenv("HOME"), ".ccpm", "profiles", name)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"model": "opus",}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A malformed settings.json used to render as "no rules / no settings / no
// plugins": the merge error was dropped and the tabs showed an empty profile.
func TestMalformedSettingsIsAnErrorNotAnEmptyProfile(t *testing.T) {
	name := syntheticProfile(t)
	fakeCCPM(t, `echo '[]'`)
	breakProfileSettings(t, name)

	d, err := NewDetails().Get(name)
	if err == nil || !strings.Contains(err.Error(), "settings.json") {
		t.Errorf("Details.Get err = %v, want the settings.json parse error", err)
	}
	assertNoNullArrays(t, d, "plugins", "env", "mcp", "allow", "ask", "deny")

	kv, err := NewSettings().Get(name)
	if err == nil || !strings.Contains(err.Error(), "settings.json") {
		t.Errorf("Settings.Get err = %v, want the settings.json parse error", err)
	}
	if kv == nil {
		t.Error("Settings.Get returned a nil slice alongside its error")
	}

	c, err := NewCascade().Get(name)
	if err == nil || !strings.Contains(err.Error(), "settings.json") {
		t.Errorf("Cascade.Get err = %v, want the settings.json parse error", err)
	}
	if c == nil {
		t.Fatal("Cascade.Get returned a nil DTO alongside its error")
	}
	assertNoNullArrays(t, c, "assets", "settings")
}

// A CLI that is missing, too old for --json, or prints something that is not
// JSON used to leave the MCP list silently empty.
func TestMCPListFailureIsReported(t *testing.T) {
	cases := map[string]string{
		"bad json": `echo 'not json'`,
		"too old":  `echo 'Error: unknown flag: --json' >&2; exit 1`,
	}
	for label, script := range cases {
		t.Run(label, func(t *testing.T) {
			name := syntheticProfile(t)
			fakeCCPM(t, script)
			d, err := NewDetails().Get(name)
			if err == nil || !strings.Contains(err.Error(), "MCP") {
				t.Errorf("err = %v, want an MCP listing error", err)
			}
			assertNoNullArrays(t, d, "plugins", "env", "mcp", "allow", "ask", "deny")
		})
	}
	t.Run("missing", func(t *testing.T) {
		name := syntheticProfile(t)
		t.Setenv("PATH", t.TempDir())
		// findCCPM also probes fixed install paths; skip where one exists.
		if findCCPM() != "" {
			t.Skip("a ccpm binary is installed at a fixed path on this machine")
		}
		if _, err := NewDetails().Get(name); err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("err = %v, want the CLI-not-found error", err)
		}
	})
}
