package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExecEnvReplacesInheritedKeys is the regression for a launch that ran on
// the wrong account. execEnv used to append ccpm's values after os.Environ()
// without removing the inherited ones, and syscall.Exec passes the slice
// through verbatim. Node (npm-installed claude) reads the FIRST occurrence, so
// `ccpm run B` from a shell where `ccpm use A` had exported CLAUDE_CONFIG_DIR
// launched on A's directory, and an exported ANTHROPIC_API_KEY beat the
// profile's key. Every key ccpm sets must appear exactly once, with ccpm's
// value.
func TestExecEnvReplacesInheritedKeys(t *testing.T) {
	stub := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(stub, []byte("stub"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_BINARY", stub)
	t.Setenv("CLAUDE_CONFIG_DIR", "/profiles/A")
	t.Setenv("ANTHROPIC_API_KEY", "sk-from-shell")
	t.Setenv("CCPM_PROFILE_VAR", "from-shell")
	t.Setenv("CCPM_ONESHOT_VAR", "from-shell")
	t.Setenv("CCPM_UNTOUCHED_VAR", "kept")

	profileDir := t.TempDir()
	_, abs, env, err := execEnv(profileDir, "sk-profile",
		map[string]string{"CCPM_PROFILE_VAR": "from-profile", "CCPM_ONESHOT_VAR": "from-profile"},
		map[string]string{"CCPM_ONESHOT_VAR": "from-cli"})
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"CLAUDE_CONFIG_DIR":  abs,
		"ANTHROPIC_API_KEY":  "sk-profile",
		"CCPM_PROFILE_VAR":   "from-profile",
		"CCPM_ONESHOT_VAR":   "from-cli",
		"CCPM_UNTOUCHED_VAR": "kept",
	}
	for key, val := range want {
		var got []string
		for _, kv := range env {
			if k, v, _ := strings.Cut(kv, "="); k == key {
				got = append(got, v)
			}
		}
		if len(got) != 1 || got[0] != val {
			t.Errorf("%s: got %q, want exactly [%q]", key, got, val)
		}
	}
}
