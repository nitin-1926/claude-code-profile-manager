package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/credentials"
)

func TestWriteAPIKeyEnvPreservesOtherKeys(t *testing.T) {
	tmp := t.TempDir()

	existing := map[string]interface{}{
		"theme": "dark",
		"env": map[string]interface{}{
			"FOO": "bar",
		},
	}
	raw, _ := json.Marshal(existing)
	if err := os.WriteFile(filepath.Join(tmp, "settings.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}

	if err := writeAPIKeyEnv(tmp, "sk-test-123"); err != nil {
		t.Fatalf("writeAPIKeyEnv: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmp, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out["theme"] != "dark" {
		t.Errorf("theme lost: %v", out["theme"])
	}
	env, _ := out["env"].(map[string]interface{})
	if env["FOO"] != "bar" {
		t.Errorf("pre-existing env key clobbered: %v", env)
	}
	if env["ANTHROPIC_API_KEY"] != "sk-test-123" {
		t.Errorf("API key not written: %v", env["ANTHROPIC_API_KEY"])
	}
}

func TestWriteAPIKeyEnvCreatesFile(t *testing.T) {
	tmp := t.TempDir()

	if err := writeAPIKeyEnv(tmp, "sk-xyz"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(tmp, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	env, _ := out["env"].(map[string]interface{})
	if env["ANTHROPIC_API_KEY"] != "sk-xyz" {
		t.Errorf("API key missing: %v", out)
	}
}

func TestClearAPIKeyEnvStripsOnlyThatKey(t *testing.T) {
	tmp := t.TempDir()
	claude := filepath.Join(tmp, ".claude")
	if err := os.MkdirAll(claude, 0755); err != nil {
		t.Fatal(err)
	}
	existing := map[string]interface{}{
		"theme": "dark",
		"env": map[string]interface{}{
			"ANTHROPIC_API_KEY": "leaked",
			"FOO":               "bar",
		},
	}
	raw, _ := json.Marshal(existing)
	if err := os.WriteFile(filepath.Join(claude, "settings.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp) // os.UserHomeDir reads USERPROFILE on Windows

	if err := clearAPIKeyEnv(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(claude, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out["theme"] != "dark" {
		t.Errorf("theme lost")
	}
	env, _ := out["env"].(map[string]interface{})
	if _, present := env["ANTHROPIC_API_KEY"]; present {
		t.Errorf("API key still present: %v", env)
	}
	if env["FOO"] != "bar" {
		t.Errorf("other env key lost: %v", env)
	}
}

func TestClearAPIKeyEnvDropsEmptyEnvBlock(t *testing.T) {
	tmp := t.TempDir()
	claude := filepath.Join(tmp, ".claude")
	if err := os.MkdirAll(claude, 0755); err != nil {
		t.Fatal(err)
	}
	existing := map[string]interface{}{
		"env": map[string]interface{}{
			"ANTHROPIC_API_KEY": "x",
		},
	}
	raw, _ := json.Marshal(existing)
	if err := os.WriteFile(filepath.Join(claude, "settings.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp) // os.UserHomeDir reads USERPROFILE on Windows

	if err := clearAPIKeyEnv(); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(claude, "settings.json"))
	var out map[string]interface{}
	_ = json.Unmarshal(data, &out)
	if _, has := out["env"]; has {
		t.Errorf("empty env block should have been removed: %v", out)
	}
}

func TestClearAPIKeyEnvIsNoopIfMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp) // os.UserHomeDir reads USERPROFILE on Windows
	if err := clearAPIKeyEnv(); err != nil {
		t.Fatalf("clearAPIKeyEnv on missing file: %v", err)
	}
}

func TestSyncOAuthIdentityToDefaultRewritesIdentityKeys(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	profileDir := filepath.Join(tmp, "profile")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}
	srcRaw, _ := json.Marshal(map[string]interface{}{
		"oauthAccount": map[string]interface{}{
			"emailAddress":     "cin@example.com",
			"organizationName": "CIN Org",
		},
		"userID":      "cin-user-id",
		"projects":    map[string]interface{}{"/foo": "bar"}, // must NOT leak into ~/.claude.json
	})
	if err := os.WriteFile(filepath.Join(profileDir, ".claude.json"), srcRaw, 0600); err != nil {
		t.Fatal(err)
	}

	// Pre-existing ~/.claude.json with a stale identity and unrelated state.
	homeRaw, _ := json.Marshal(map[string]interface{}{
		"oauthAccount": map[string]interface{}{
			"emailAddress":     "labs@example.com",
			"organizationName": "Labs Org",
		},
		"userID":         "labs-user-id",
		"customApiKeyResponses": map[string]interface{}{"approved": []string{"x"}},
	})
	if err := os.WriteFile(filepath.Join(tmp, ".claude.json"), homeRaw, 0600); err != nil {
		t.Fatal(err)
	}

	if err := syncOAuthIdentityToDefault(profileDir); err != nil {
		t.Fatalf("syncOAuthIdentityToDefault: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(tmp, ".claude.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatal(err)
	}
	oa, _ := out["oauthAccount"].(map[string]interface{})
	if oa["emailAddress"] != "cin@example.com" {
		t.Errorf("emailAddress not synced: %v", oa)
	}
	if oa["organizationName"] != "CIN Org" {
		t.Errorf("organizationName not synced: %v", oa)
	}
	if out["userID"] != "cin-user-id" {
		t.Errorf("userID not synced: %v", out["userID"])
	}
	// Unrelated keys must be preserved.
	if _, has := out["customApiKeyResponses"]; !has {
		t.Errorf("unrelated key was dropped: %v", out)
	}
	// Profile-only keys must NOT bleed in.
	if _, has := out["projects"]; has {
		t.Errorf("non-identity key leaked into ~/.claude.json: %v", out)
	}
}

func TestSyncOAuthIdentityToDefaultIsNoopWhenSourceMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	profileDir := filepath.Join(tmp, "profile")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}
	// No .claude.json in profileDir.
	if err := syncOAuthIdentityToDefault(profileDir); err != nil {
		t.Fatalf("expected no-op, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".claude.json")); !os.IsNotExist(err) {
		t.Errorf("expected ~/.claude.json untouched, got err=%v", err)
	}
}
