package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// envValues returns every value env holds for key, in order.
func envValues(env []string, key string) []string {
	var got []string
	for _, kv := range env {
		if k, v, _ := strings.Cut(kv, "="); k == key {
			got = append(got, v)
		}
	}
	return got
}

func stubClaude(t *testing.T) {
	t.Helper()
	stub := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(stub, []byte("stub"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_BINARY", stub)
}

// A provider profile must launch with its own endpoint and key whatever the
// parent shell exported: Anthropic documents CLAUDE_CODE_USE_* as outranking
// ANTHROPIC_AUTH_TOKEN, and an Anthropic key left in the shell must not travel
// to a third party.
func TestExecEnvProviderProfileIgnoresInheritedRouting(t *testing.T) {
	stubClaude(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-from-shell")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "from-shell")
	t.Setenv("ANTHROPIC_BASE_URL", "https://shell.example")
	t.Setenv("ANTHROPIC_MODEL", "from-shell")
	t.Setenv("ANTHROPIC_CUSTOM_HEADERS", "x-secret: from-shell")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "1")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "from-shell")
	t.Setenv("CCPM_UNTOUCHED_VAR", "kept")

	_, _, env, err := execEnv(t.TempDir(), "ANTHROPIC_AUTH_TOKEN", "provider-key",
		map[string]string{"ANTHROPIC_BASE_URL": "https://provider.example"},
		map[string]string{"CCPM_ONESHOT_VAR": "from-cli"})
	if err != nil {
		t.Fatal(err)
	}

	for key, want := range map[string]string{
		"ANTHROPIC_AUTH_TOKEN": "provider-key",
		"ANTHROPIC_API_KEY":    "",
		"ANTHROPIC_BASE_URL":   "https://provider.example",
		"CCPM_UNTOUCHED_VAR":   "kept",
		"CCPM_ONESHOT_VAR":     "from-cli",
	} {
		if got := envValues(env, key); len(got) != 1 || got[0] != want {
			t.Errorf("%s: got %q, want exactly [%q]", key, got, want)
		}
	}
	for _, key := range []string{"ANTHROPIC_MODEL", "ANTHROPIC_CUSTOM_HEADERS", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_OAUTH_TOKEN"} {
		if got := envValues(env, key); len(got) != 0 {
			t.Errorf("%s inherited into a provider launch: %q", key, got)
		}
	}
}

// An ordinary profile keeps its contract: the key is ANTHROPIC_API_KEY,
// ANTHROPIC_AUTH_TOKEN is untouched, and other inherited variables pass through.
func TestExecEnvOrdinaryProfileUnchanged(t *testing.T) {
	stubClaude(t)
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "gateway")
	t.Setenv("ANTHROPIC_MODEL", "inherited")
	t.Setenv("CLAUDE_CODE_USE_VERTEX", "1")

	_, _, env, err := execEnv(t.TempDir(), "", "sk-ant", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"ANTHROPIC_API_KEY":      "sk-ant",
		"ANTHROPIC_AUTH_TOKEN":   "gateway",
		"ANTHROPIC_MODEL":        "inherited",
		"CLAUDE_CODE_USE_VERTEX": "1",
	} {
		if got := envValues(env, key); len(got) != 1 || got[0] != want {
			t.Errorf("%s: got %q, want exactly [%q]", key, got, want)
		}
	}
}
