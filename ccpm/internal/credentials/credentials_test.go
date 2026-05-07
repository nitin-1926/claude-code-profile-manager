package credentials

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/keystore"
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

// TestCheckOAuthClaudeJSONOnlyIsInvalid asserts that a profile with only the
// .claude.json metadata block — and no actual credential file or keychain
// entry — is reported as INVALID. The metadata only proves "someone logged
// in here at some point"; trusting it caused `ccpm status` to claim profiles
// were authenticated even after their tokens expired.
func TestCheckOAuthClaudeJSONOnlyIsInvalid(t *testing.T) {
	tmp := t.TempDir()
	store := keystore.NewMemoryStore()
	checker := NewChecker(store)

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
	if status.Valid {
		t.Errorf("Should be invalid when only .claude.json exists, got valid: %s", status.Detail)
	}
	if !contains(status.Detail, "test@example.com") {
		t.Errorf("Detail should still surface the last-known email, got: %s", status.Detail)
	}
	if !contains(status.Detail, "auth refresh") {
		t.Errorf("Detail should hint at re-auth, got: %s", status.Detail)
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

// TestCheckOAuthShortLivedAccessTokenWithRefreshIsHealthy guards the listing
// behavior: Claude OAuth issues ~1h access tokens with a refresh token, and
// `claude` refreshes silently on use. So a healthy profile should NOT show a
// near-expiry warning when a refresh token is present, even if the access
// token is minutes from expiring.
func TestCheckOAuthShortLivedAccessTokenWithRefreshIsHealthy(t *testing.T) {
	tmp := t.TempDir()
	store := keystore.NewMemoryStore()
	checker := NewChecker(store)

	// Access token expires in 30 min, refresh token present.
	soon := time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339)
	credsJSON := `{"accessToken":"a","refreshToken":"r","expiresAt":"` + soon + `"}`
	os.WriteFile(filepath.Join(tmp, ".credentials.json"), []byte(credsJSON), 0600)

	status := checker.Check(tmp, "test", "oauth")
	if !status.Valid {
		t.Errorf("Should be valid when refresh token is present, got: %s", status.Detail)
	}
	if contains(status.Detail, "expires in") {
		t.Errorf("Detail should not warn about access-token expiry when refresh is present, got: %s", status.Detail)
	}
	if status.ExpireAt != "" {
		t.Errorf("ExpireAt should be empty so callers don't render an expiry warning, got: %q", status.ExpireAt)
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
