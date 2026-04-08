package credentials

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

type oauthCreds struct {
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
	credFile := filepath.Join(profileDir, ".credentials.json")
	data, err := os.ReadFile(credFile)
	if err != nil {
		return CredStatus{Valid: false, Method: "oauth", Detail: "no credentials file found"}
	}

	var creds oauthCreds
	if err := json.Unmarshal(data, &creds); err != nil {
		return CredStatus{Valid: false, Method: "oauth", Detail: "credentials file corrupted"}
	}

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
