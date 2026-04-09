package credentials

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/nitin-1926/ccpm/internal/keystore"
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
	// Strategy 1: Check .claude.json for oauthAccount (macOS primary method)
	claudeFile := filepath.Join(profileDir, ".claude.json")
	if data, err := os.ReadFile(claudeFile); err == nil {
		var cj claudeJSON
		if err := json.Unmarshal(data, &cj); err == nil && cj.OAuthAccount != nil {
			email := cj.OAuthAccount.EmailAddress
			name := cj.OAuthAccount.DisplayName
			detail := fmt.Sprintf("%s (%s)", email, name)
			if name == "" {
				detail = email
			}
			return CredStatus{Valid: true, Method: "oauth", Detail: detail}
		}
	}

	// Strategy 2: Check .credentials.json (Linux/Windows primary method)
	credFile := filepath.Join(profileDir, ".credentials.json")
	if data, err := os.ReadFile(credFile); err == nil {
		var creds credentialsJSON
		if err := json.Unmarshal(data, &creds); err == nil && creds.AccessToken != "" {
			if creds.ExpiresAt != "" {
				expiry, err := time.Parse(time.RFC3339, creds.ExpiresAt)
				if err == nil {
					if time.Now().After(expiry) {
						return CredStatus{Valid: false, Method: "oauth", Detail: "token expired", ExpireAt: creds.ExpiresAt}
					}
					remaining := time.Until(expiry)
					if remaining < 7*24*time.Hour {
						return CredStatus{Valid: true, Method: "oauth", Detail: fmt.Sprintf("expires in %s", remaining.Round(time.Hour)), ExpireAt: creds.ExpiresAt}
					}
				}
			}
			return CredStatus{Valid: true, Method: "oauth", Detail: "authenticated", ExpireAt: creds.ExpiresAt}
		}
	}

	// Strategy 3: Check if userID exists in .claude.json (weaker signal but still valid)
	if data, err := os.ReadFile(claudeFile); err == nil {
		var cj claudeJSON
		if err := json.Unmarshal(data, &cj); err == nil && cj.UserID != "" {
			if runtime.GOOS == "darwin" {
				return CredStatus{Valid: true, Method: "oauth", Detail: "authenticated (keychain)"}
			}
		}
	}

	return CredStatus{Valid: false, Method: "oauth", Detail: "not authenticated"}
}
