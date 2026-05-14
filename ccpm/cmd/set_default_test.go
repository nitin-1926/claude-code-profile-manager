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

func TestShouldSaveBackPreservesFresherProfileSlot(t *testing.T) {
	// Profile slot is FRESHER than default (recent `ccpm run X claude /login`).
	// Save-back must NOT overwrite — that would destroy the user's re-login.
	prev := &credentials.MacKeychainOAuth{
		Raw:       `{"claudeAiOauth":{"accessToken":"FRESH","refreshToken":"FRESH-R","expiresAt":2000000000000}}`,
		ExpiresAt: time.UnixMilli(2000000000000),
	}
	cur := &credentials.MacKeychainOAuth{
		Raw:       `{"claudeAiOauth":{"accessToken":"STALE","refreshToken":"STALE-R","expiresAt":1000000000000}}`,
		ExpiresAt: time.UnixMilli(1000000000000),
	}
	if shouldSaveBack(prev, cur) {
		t.Fatal("must not overwrite a fresher profile slot with stale default content")
	}
}

func TestShouldSaveBackSavesFresherDefaultSlot(t *testing.T) {
	// Default slot is FRESHER (drift scenario — plain `claude` rotated tokens
	// while this profile was active). Save-back captures the rotation.
	prev := &credentials.MacKeychainOAuth{
		Raw:       `{"claudeAiOauth":{"accessToken":"STALE","refreshToken":"STALE-R","expiresAt":1000000000000}}`,
		ExpiresAt: time.UnixMilli(1000000000000),
	}
	cur := &credentials.MacKeychainOAuth{
		Raw:       `{"claudeAiOauth":{"accessToken":"FRESH","refreshToken":"FRESH-R","expiresAt":2000000000000}}`,
		ExpiresAt: time.UnixMilli(2000000000000),
	}
	if !shouldSaveBack(prev, cur) {
		t.Fatal("must overwrite a stale profile slot with fresher default content")
	}
}

func TestShouldSaveBackEqualPayloadIsNoop(t *testing.T) {
	raw := `{"claudeAiOauth":{"accessToken":"SAME","refreshToken":"SAME-R","expiresAt":1500000000000}}`
	exp := time.UnixMilli(1500000000000)
	prev := &credentials.MacKeychainOAuth{Raw: raw, ExpiresAt: exp}
	cur := &credentials.MacKeychainOAuth{Raw: raw, ExpiresAt: exp}
	if shouldSaveBack(prev, cur) {
		t.Fatal("identical payloads must short-circuit the save-back")
	}
}

func TestShouldSaveBackEqualExpiryPreservesProfileSlot(t *testing.T) {
	// Same expiresAt but different Raw — could be a benign re-issuance or a
	// genuine drift, but we err on the side of preserving the profile slot
	// since `ccpm run … /login` produces the exact same expiresAt as the
	// default-slot tokens it just replaced. Profile wins ties.
	prev := &credentials.MacKeychainOAuth{
		Raw:       `{"claudeAiOauth":{"accessToken":"PROFILE","refreshToken":"P-R","expiresAt":1500000000000}}`,
		ExpiresAt: time.UnixMilli(1500000000000),
	}
	cur := &credentials.MacKeychainOAuth{
		Raw:       `{"claudeAiOauth":{"accessToken":"DEFAULT","refreshToken":"D-R","expiresAt":1500000000000}}`,
		ExpiresAt: time.UnixMilli(1500000000000),
	}
	if shouldSaveBack(prev, cur) {
		t.Fatal("tie on expiresAt must preserve the profile slot, not overwrite it")
	}
}

func TestShouldSaveBackEmptyProfileSlotIsPopulated(t *testing.T) {
	cur := &credentials.MacKeychainOAuth{
		Raw:       `{"claudeAiOauth":{"accessToken":"X","refreshToken":"Y","expiresAt":2000000000000}}`,
		ExpiresAt: time.UnixMilli(2000000000000),
	}
	if !shouldSaveBack(nil, cur) {
		t.Fatal("a missing profile slot should be populated from default")
	}
}

func TestShouldSaveBackEmptyDefaultSlotIsNoop(t *testing.T) {
	prev := &credentials.MacKeychainOAuth{
		Raw:       `{"claudeAiOauth":{"accessToken":"P","refreshToken":"Q","expiresAt":2000000000000}}`,
		ExpiresAt: time.UnixMilli(2000000000000),
	}
	if shouldSaveBack(prev, nil) {
		t.Fatal("nil cur must short-circuit")
	}
	if shouldSaveBack(prev, &credentials.MacKeychainOAuth{Raw: ""}) {
		t.Fatal("empty-Raw cur must short-circuit")
	}
}
