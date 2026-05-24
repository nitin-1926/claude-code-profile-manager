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
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    string `json:"expiresAt"`
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
	return CredStatus{Valid: true, Method: "api_key", Detail: fmt.Sprintf("key: %s", maskKey(key))}
}

// maskKey produces a display-safe rendering of an API key without ever slicing
// out of bounds. Anthropic keys are long (sk-ant-...), but a malformed or
// fat-fingered entry could be arbitrarily short; slicing key[:7]/key[len-4:]
// unconditionally would panic and brick `ccpm status`/`list`/`auth status`.
func maskKey(key string) string {
	if len(key) <= 8 {
		// Too short to reveal a head and tail without overlap — mask entirely.
		return "****"
	}
	return key[:7] + "..." + key[len(key)-4:]
}

func (c *Checker) checkOAuth(profileDir string) CredStatus {
	claudeFile := filepath.Join(profileDir, ".claude.json")

	// Strategy 1 (macOS + Windows): read the namespaced OS-credential entry
	// Claude Code writes in v2.1.56+. Gives us the real access token and
	// expiry. Linux still falls through to .credentials.json until a
	// libsecret-backed handler lands.
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		if kc, err := ReadMacKeychainOAuth(profileDir); err == nil && kc != nil && kc.AccessToken != "" {
			detail := accountDetailFromClaudeJSON(claudeFile)
			if detail == "" {
				detail = kc.Email
			}
			return buildOAuthStatus(detail, kc.ExpiresAt, kc.RefreshToken != "", "keychain")
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
			return buildOAuthStatus(accountDetailFromClaudeJSON(claudeFile), expiry, creds.RefreshToken != "", "file")
		}
	}

	// Strategy 3: .claude.json metadata is *only* a hint about who last
	// logged in; it is NOT proof that current credentials are valid. If we
	// reach this branch on macOS the keychain entry is missing — the profile
	// needs a re-login, regardless of how nicely-formatted the metadata is.
	// Older code marked the profile Valid here, which is why `ccpm status`
	// could claim a profile was authenticated while `claude` itself prompted
	// for re-auth.
	if data, err := os.ReadFile(claudeFile); err == nil {
		var cj claudeJSON
		if err := json.Unmarshal(data, &cj); err == nil && cj.OAuthAccount != nil {
			email := cj.OAuthAccount.EmailAddress
			name := cj.OAuthAccount.DisplayName
			who := email
			if name != "" && email != "" {
				who = fmt.Sprintf("%s (%s)", email, name)
			} else if name != "" {
				who = name
			}
			return CredStatus{Valid: false, Method: "oauth", Detail: fmt.Sprintf("%s — credentials missing, run `ccpm auth refresh`", who)}
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

// buildOAuthStatus assembles a CredStatus with a friendly detail line.
//
// Claude OAuth issues short-lived (~1h) access tokens paired with a
// long-lived refresh token; `claude` itself silently refreshes on use. So a
// freshly-logged-in profile still shows ~1h remaining on the access token,
// and that's not a problem as long as the refresh token is around. Callers
// pass hasRefresh=true when a refresh token is present in the keychain or
// .credentials.json — in that case we don't surface the access-token expiry
// at all, because it would constantly cry "expires in 1h" for healthy
// profiles. Without a refresh token we fall back to access-token-based
// reporting (warn when close, fail when expired).
func buildOAuthStatus(account string, expiry time.Time, hasRefresh bool, source string) CredStatus {
	detail := account
	if detail == "" {
		detail = "authenticated"
	}
	expireAt := ""
	if !expiry.IsZero() {
		expireAt = expiry.Format(time.RFC3339)
	}

	if hasRefresh {
		// Deliberately omit ExpireAt: callers like `ccpm list` paint a yellow
		// "expiring soon" warning whenever ExpireAt is within 7 days, and
		// access tokens are nearly always within 7 days. The refresh token is
		// what keeps the profile alive, and `claude` rotates it transparently.
		if source == "keychain" {
			detail = fmt.Sprintf("%s (keychain)", detail)
		}
		return CredStatus{Valid: true, Method: "oauth", Detail: detail}
	}

	if !expiry.IsZero() {
		if time.Now().After(expiry) {
			return CredStatus{Valid: false, Method: "oauth", Detail: fmt.Sprintf("%s — token expired", detail), ExpireAt: expireAt}
		}
		remaining := time.Until(expiry)
		if remaining < 7*24*time.Hour {
			detail = fmt.Sprintf("%s — expires in %s", detail, remaining.Round(time.Hour))
		}
		return CredStatus{Valid: true, Method: "oauth", Detail: detail, ExpireAt: expireAt}
	}
	if source == "keychain" {
		detail = fmt.Sprintf("%s (keychain)", detail)
	}
	return CredStatus{Valid: true, Method: "oauth", Detail: detail}
}
