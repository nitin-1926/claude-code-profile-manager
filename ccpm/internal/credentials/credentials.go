package credentials

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/keystore"
)

type CredStatus struct {
	Valid     bool
	Method   string
	Detail   string
	ExpireAt string
}

type Checker struct {
	Store keystore.Store
}

func NewChecker(store keystore.Store) *Checker {
	return &Checker{Store: store}
}

// oauthAccount is the account info stored in .claude.json on macOS
type oauthAccount struct {
	AccountUuid  string `json:"accountUuid"`
	EmailAddress string `json:"emailAddress"`
	DisplayName  string `json:"displayName"`
}

// claudeJSON is the top-level .claude.json structure
type claudeJSON struct {
	OAuthAccount *oauthAccount `json:"oauthAccount"`
	UserID       string        `json:"userID"`
}

// credentialsJSON is the .credentials.json format used on Linux/Windows
type credentialsJSON struct {
	AccessToken string `json:"accessToken"`
	ExpiresAt   string `json:"expiresAt"`
}

func (c *Checker) Check(profileDir, profileName, authMethod string) CredStatus {
	switch authMethod {
	case "api_key":
		return c.checkAPIKey(profileName)
	case "oauth":
		return c.checkOAuth(profileDir)
	default:
		return CredStatus{Valid: false, Method: authMethod, Detail: "unknown auth method"}
	}
}

func (c *Checker) checkAPIKey(profileName string) CredStatus {
	key, err := c.Store.GetAPIKey(profileName)
	if err != nil || key == "" {
		return CredStatus{Valid: false, Method: "api_key", Detail: "no API key found in keychain"}
	}
	// Mask the key for display
	masked := key[:7] + "..." + key[len(key)-4:]
	return CredStatus{Valid: true, Method: "api_key", Detail: fmt.Sprintf("key: %s", masked)}
}

func (c *Checker) checkOAuth(profileDir string) CredStatus {
	claudeFile := filepath.Join(profileDir, ".claude.json")

	// Strategy 1 (macOS): read the namespaced keychain entry Claude Code
	// writes in v2.1.56+. Gives us the real access token and expiry.
	if runtime.GOOS == "darwin" {
		if kc, err := ReadMacKeychainOAuth(profileDir); err == nil && kc != nil && kc.AccessToken != "" {
			detail := accountDetailFromClaudeJSON(claudeFile)
			if detail == "" {
				detail = kc.Email
			}
			return buildOAuthStatus(detail, kc.ExpiresAt, "keychain")
		}
	}

	// Strategy 2: .credentials.json (Linux/Windows primary method, and a
	// legacy fallback on older macOS Claude Code releases).
	credFile := filepath.Join(profileDir, ".credentials.json")
	if data, err := os.ReadFile(credFile); err == nil {
		var creds credentialsJSON
		if err := json.Unmarshal(data, &creds); err == nil && creds.AccessToken != "" {
			var expiry time.Time
			if creds.ExpiresAt != "" {
				if parsed, perr := time.Parse(time.RFC3339, creds.ExpiresAt); perr == nil {
					expiry = parsed
				}
			}
			return buildOAuthStatus(accountDetailFromClaudeJSON(claudeFile), expiry, "file")
		}
	}

	// Strategy 3: fall back to .claude.json metadata. Lower fidelity — no
	// expiry information — but at least confirms "someone logged in here".
	if data, err := os.ReadFile(claudeFile); err == nil {
		var cj claudeJSON
		if err := json.Unmarshal(data, &cj); err == nil {
			if cj.OAuthAccount != nil {
				email := cj.OAuthAccount.EmailAddress
				name := cj.OAuthAccount.DisplayName
				detail := email
				if name != "" {
					detail = fmt.Sprintf("%s (%s)", email, name)
				}
				return CredStatus{Valid: true, Method: "oauth", Detail: detail}
			}
			if cj.UserID != "" && runtime.GOOS == "darwin" {
				return CredStatus{Valid: true, Method: "oauth", Detail: "authenticated (keychain)"}
			}
		}
	}

	return CredStatus{Valid: false, Method: "oauth", Detail: "not authenticated"}
}

// accountDetailFromClaudeJSON returns the "email (display name)" string for a
// profile when the .claude.json file has oauthAccount metadata. Empty string
// if the file is missing or malformed.
func accountDetailFromClaudeJSON(claudeFile string) string {
	data, err := os.ReadFile(claudeFile)
	if err != nil {
		return ""
	}
	var cj claudeJSON
	if err := json.Unmarshal(data, &cj); err != nil || cj.OAuthAccount == nil {
		return ""
	}
	email := cj.OAuthAccount.EmailAddress
	name := cj.OAuthAccount.DisplayName
	if name != "" && email != "" {
		return fmt.Sprintf("%s (%s)", email, name)
	}
	if email != "" {
		return email
	}
	return name
}

// buildOAuthStatus assembles a CredStatus with a friendly detail line,
// including expiry warnings when the expiry time is known.
func buildOAuthStatus(account string, expiry time.Time, source string) CredStatus {
	detail := account
	if detail == "" {
		detail = "authenticated"
	}
	if !expiry.IsZero() {
		if time.Now().After(expiry) {
			return CredStatus{Valid: false, Method: "oauth", Detail: fmt.Sprintf("%s — token expired", detail), ExpireAt: expiry.Format(time.RFC3339)}
		}
		remaining := time.Until(expiry)
		if remaining < 7*24*time.Hour {
			detail = fmt.Sprintf("%s — expires in %s", detail, remaining.Round(time.Hour))
		}
		return CredStatus{Valid: true, Method: "oauth", Detail: detail, ExpireAt: expiry.Format(time.RFC3339)}
	}
	if source == "keychain" {
		detail = fmt.Sprintf("%s (keychain)", detail)
	}
	return CredStatus{Valid: true, Method: "oauth", Detail: detail}
}
