package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/trust"
)

// runParallel runs n fresh command trees concurrently, the i-th with args(i).
func runParallel(t *testing.T, n int, newCmd func() *cobra.Command, args func(i int) []string) {
	t.Helper()
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := runCobra(t, newCmd(), args(i)...); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
}

func loadFragmentAndOwned(t *testing.T, profile string) (map[string]interface{}, map[string]struct{}) {
	t.Helper()
	fragPath, err := settingsFragmentPath(profile)
	if err != nil {
		t.Fatal(err)
	}
	frag, err := settingsmerge.LoadJSON(fragPath)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := settingsmerge.LoadOwnedKeys(fragPath)
	if err != nil {
		t.Fatal(err)
	}
	return frag, owned
}

// TestSettingsSetConcurrentKeepsEveryKey: parallel `settings set` used to
// keep 3/30 keys — fragment and owned sidecar were unlocked read-modify-writes.
func TestSettingsSetConcurrentKeepsEveryKey(t *testing.T) {
	sandboxHome(t, "a")
	const n = 30
	runParallel(t, n, newSettingsCmd, func(i int) []string {
		return []string{"set", fmt.Sprintf("k%d", i), "v", "--profile", "a"}
	})
	frag, owned := loadFragmentAndOwned(t, "a")
	for i := 0; i < n; i++ {
		k := fmt.Sprintf("k%d", i)
		if frag[k] != "v" {
			t.Errorf("fragment lost %s", k)
		}
		if _, ok := owned[k]; !ok {
			t.Errorf("owned sidecar lost %s", k)
		}
	}
}

// TestPermissionsAllowConcurrentKeepsEveryRule: parallel `permissions allow`
// used to keep 3/30 rules.
func TestPermissionsAllowConcurrentKeepsEveryRule(t *testing.T) {
	sandboxHome(t, "a")
	const n = 30
	runParallel(t, n, newPermissionsCmd, func(i int) []string {
		return []string{"allow", fmt.Sprintf("Bash(tool%d:*)", i), "--profile", "a"}
	})
	frag, owned := loadFragmentAndOwned(t, "a")
	perms, _ := frag["permissions"].(map[string]interface{})
	allow, _ := perms["allow"].([]interface{})
	if len(allow) != n {
		t.Fatalf("kept %d/%d allow rules: %v", len(allow), n, allow)
	}
	if _, ok := owned["permissions.allow"]; !ok {
		t.Fatalf("permissions.allow not marked owned: %v", owned)
	}
}

// TestPermissionsSurfacesOwnedKeysError: the owned-keys write error used to
// be discarded (`_ = MarkOwned`), reporting success while `ccpm run` would
// later let settings.json shadow the rule.
func TestPermissionsSurfacesOwnedKeysError(t *testing.T) {
	sandboxHome(t, "a")
	fragPath, err := settingsFragmentPath("a")
	if err != nil {
		t.Fatal(err)
	}
	sidecar := strings.TrimSuffix(fragPath, ".json") + ".owned.json"
	if err := os.MkdirAll(filepath.Dir(sidecar), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sidecar, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"allow", "Bash(ls)", "--profile", "a"},
		{"mode", "plan", "--profile", "a"},
	} {
		if err := runCobra(t, newPermissionsCmd(), args...); err == nil {
			t.Errorf("permissions %v succeeded despite an unreadable owned-keys sidecar", args[0])
		}
	}
}

// TestHooksAddConcurrentKeepsEveryHook: same race via `hooks add`.
func TestHooksAddConcurrentKeepsEveryHook(t *testing.T) {
	sandboxHome(t, "a")
	const n = 30
	runParallel(t, n, newHooksCmd, func(i int) []string {
		return []string{"add", "PreToolUse", fmt.Sprintf("echo %d", i), "--profile", "a"}
	})
	frag, owned := loadFragmentAndOwned(t, "a")
	hooks, _ := frag["hooks"].(map[string]interface{})
	entries, _ := hooks["PreToolUse"].([]interface{})
	if len(entries) != n {
		t.Fatalf("kept %d/%d hooks", len(entries), n)
	}
	if _, ok := owned["hooks.PreToolUse"]; !ok {
		t.Fatalf("hooks.PreToolUse not marked owned: %v", owned)
	}
}

// TestTrustAddConcurrentKeepsEveryPath: parallel `trust add` kept 1/20.
func TestTrustAddConcurrentKeepsEveryPath(t *testing.T) {
	sandboxHome(t)
	base := t.TempDir()
	const n = 20
	runParallel(t, n, newTrustCmd, func(i int) []string {
		return []string{"add", filepath.Join(base, fmt.Sprintf("proj%d", i))}
	})
	records, err := trust.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != n {
		t.Fatalf("kept %d/%d trusted paths", len(records), n)
	}
}
