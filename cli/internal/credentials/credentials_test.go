package credentials

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nitin-1926/ccpm/internal/keystore"
)

func TestCheckAPIKey(t *testing.T) {
	store := keystore.NewMemoryStore()
	checker := NewChecker(store)

	// No key stored
	status := checker.Check("/tmp/test", "myprofile", "api_key")
	if status.Valid {
		t.Error("Should be invalid when no API key stored")
	}

	// Store a key
	store.SetAPIKey("myprofile", "sk-ant-api03-abcdef1234567890")

	status = checker.Check("/tmp/test", "myprofile", "api_key")
	if !status.Valid {
		t.Errorf("Should be valid after storing key, got: %s", status.Detail)
	}
	if status.Method != "api_key" {
		t.Errorf("Method = %q, want %q", status.Method, "api_key")
	}
	// Should be masked
	if !contains(status.Detail, "...") {
		t.Error("API key should be masked in detail")
	}
	if contains(status.Detail, "abcdef1234567890") {
		t.Error("Full API key should NOT appear in detail")
	}
}

func TestCheckOAuthWithClaudeJSON(t *testing.T) {
	tmp := t.TempDir()
	store := keystore.NewMemoryStore()
	checker := NewChecker(store)

	// Write a .claude.json with oauthAccount
	claudeJSON := `{
		"oauthAccount": {
			"accountUuid": "abc-123",
			"emailAddress": "test@example.com",
			"displayName": "Test User"
		},
		"userID": "someid"
	}`
	os.WriteFile(filepath.Join(tmp, ".claude.json"), []byte(claudeJSON), 0600)

	status := checker.Check(tmp, "test", "oauth")
	if !status.Valid {
		t.Errorf("Should be valid with .claude.json oauthAccount, got: %s", status.Detail)
	}
	if !contains(status.Detail, "test@example.com") {
		t.Errorf("Detail should contain email, got: %s", status.Detail)
	}
	if !contains(status.Detail, "Test User") {
		t.Errorf("Detail should contain display name, got: %s", status.Detail)
	}
}

func TestCheckOAuthWithCredentialsFile(t *testing.T) {
	tmp := t.TempDir()
	store := keystore.NewMemoryStore()
	checker := NewChecker(store)

	// Write a .credentials.json (Linux/Windows format)
	credsJSON := `{"accessToken":"token123","expiresAt":"2030-12-31T00:00:00Z"}`
	os.WriteFile(filepath.Join(tmp, ".credentials.json"), []byte(credsJSON), 0600)

	status := checker.Check(tmp, "test", "oauth")
	if !status.Valid {
		t.Errorf("Should be valid with .credentials.json, got: %s", status.Detail)
	}
}

func TestCheckOAuthExpired(t *testing.T) {
	tmp := t.TempDir()
	store := keystore.NewMemoryStore()
	checker := NewChecker(store)

	// Write expired credentials
	credsJSON := `{"accessToken":"token123","expiresAt":"2020-01-01T00:00:00Z"}`
	os.WriteFile(filepath.Join(tmp, ".credentials.json"), []byte(credsJSON), 0600)

	status := checker.Check(tmp, "test", "oauth")
	if status.Valid {
		t.Error("Should be invalid for expired token")
	}
	if !contains(status.Detail, "expired") {
		t.Errorf("Detail should mention expiry, got: %s", status.Detail)
	}
}

func TestCheckOAuthNoCredentials(t *testing.T) {
	tmp := t.TempDir()
	store := keystore.NewMemoryStore()
	checker := NewChecker(store)

	status := checker.Check(tmp, "test", "oauth")
	if status.Valid {
		t.Error("Should be invalid when no credentials exist")
	}
}

func TestCheckUnknownMethod(t *testing.T) {
	store := keystore.NewMemoryStore()
	checker := NewChecker(store)

	status := checker.Check("/tmp", "test", "magic")
	if status.Valid {
		t.Error("Unknown auth method should be invalid")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
