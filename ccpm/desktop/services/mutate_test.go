//go:build darwin

package services

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
)

// A value starting with "-" was parsed by the CLI as a flag — setting a
// numeric setting to -1 failed as "unknown shorthand flag", which outdatedCLI
// then misreported as "CLI too old". Positionals from the UI now follow `--`.
// Driven through the real CLI, which is also the check that every subcommand
// these calls use accepts the separator.
func TestUserValuesStartingWithADashAreNotFlags(t *testing.T) {
	name := syntheticProfile(t)
	realCCPM(t)
	m := NewMutate()
	dir := filepath.Join(os.Getenv("HOME"), ".ccpm", "profiles", name)

	ok := func(label string, r CmdResult) {
		t.Helper()
		if !r.OK {
			t.Errorf("%s: %s", label, r.Error)
		}
	}
	ok("settings set -1", m.SetSetting("cleanupPeriodDays", "-1", name))
	ok("permissions allow -rule", m.AddPermission("allow", "-rule", name))
	merged, err := settingsmerge.ComputeMerged(dir, name, "")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := merged["cleanupPeriodDays"].(float64); v != -1 {
		t.Errorf("cleanupPeriodDays = %v, want -1", merged["cleanupPeriodDays"])
	}
	perms, _ := merged["permissions"].(map[string]interface{})
	if allow, _ := perms["allow"].([]interface{}); !slices.Contains(allow, interface{}("-rule")) {
		t.Errorf("allow = %v, want -rule in it", perms["allow"])
	}
	ok("permissions remove -rule", m.RemovePermission("-rule", name))

	// The rest of the separated calls, with ordinary values: each subcommand
	// must accept `--` rather than count it as an argument.
	ok("permissions mode", m.SetPermissionMode("plan", name))
	ok("env set", m.SetEnv("CCPM_TEST=1", name))
	ok("env unset", m.UnsetEnv("CCPM_TEST", name))
	ok("mcp add stdio", m.AddStdioMCP("srv", "-weird-command", name))
	ok("mcp add http", m.AddHTTPMCP("web", "https://example.com/mcp", name))
	ok("mcp remove", m.RemoveMCP("srv", name))

	skill := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: demo\ndescription: d\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ok("skill add", m.AddAsset("skill", skill, name))
	ok("skill remove", m.RemoveAsset("skill", "demo", name))

	// Plugins need a marketplace this sandbox does not have; what matters is
	// that the separator is not rejected as a flag or an extra argument.
	for _, r := range []CmdResult{m.TogglePlugin("p@m", true, name), m.InstallPlugin("p@m", name), m.RemovePlugin("p@m", name)} {
		if strings.Contains(r.Output, "accepts 1 arg") || outdatedCLI(r.Output) {
			t.Errorf("plugin call rejected its arguments: %s", r.Output)
		}
	}
}
