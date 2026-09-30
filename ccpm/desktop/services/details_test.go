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

// The MCP tab listed every profile's profile-scoped servers, and Remove on
// another profile's server deleted ccpm's record of it while it kept running
// there. Driven through the real CLI so the list rows are the real shape.
func TestMCPTabShowsOnlyThisProfilesServers(t *testing.T) {
	name := syntheticProfile(t)
	addSyntheticProfile(t, "beta")
	realCCPM(t)
	mustCCPM(t, "mcp", "add", "only-beta", "--scope", "profile", "--profile", "beta", "--command", "x")
	mustCCPM(t, "mcp", "add", "mine", "--scope", "profile", "--profile", name, "--command", "x")
	mustCCPM(t, "mcp", "add", "everywhere", "--scope", "global", "--command", "x")

	d, err := NewDetails().Get(name)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, m := range d.Mcp {
		got[m.Name] = m.Removable
	}
	want := map[string]bool{"mine": true, "everywhere": false}
	if len(got) != len(want) || got["mine"] != true || got["everywhere"] != false {
		t.Errorf("servers (name → removable) = %v, want %v", got, want)
	}

	if r := NewMutate().RemoveMCP("only-beta", name); r.OK {
		t.Errorf("removing beta's server from %s succeeded: %s", name, r.Output)
	}
	b, err := NewDetails().Get("beta")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range b.Mcp {
		found = found || (m.Name == "only-beta" && m.Removable)
	}
	if !found {
		t.Errorf("beta lost only-beta after a refused remove: %+v", b.Mcp)
	}
	if r := NewMutate().RemoveMCP("mine", name); !r.OK {
		t.Errorf("removing this profile's own server failed: %s", r.Error)
	}
}

// The desktop app ships apart from the CLI, so an older ccpm on PATH — one
// whose remove still deletes another profile's record — must never be asked.
// The row below is `ccpm mcp list --json` output verbatim for a server beta
// added at profile scope.
func TestRemoveMCPRefusesBeforeAskingAnOlderCLI(t *testing.T) {
	name := syntheticProfile(t)
	fakeCCPM(t, `if [ "$2" = list ]; then echo '[{"name":"only-beta","sources":["ccpm-profile"],"profiles":["beta"]}]'; else echo "✓ MCP server removed"; fi`)
	if r := NewMutate().RemoveMCP("only-beta", name); r.OK {
		t.Errorf("RemoveMCP handed beta's server to the CLI for %s: %s", name, r.Output)
	}
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
