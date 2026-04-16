//go:build darwin

package credentials

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/zalando/go-keyring"
)

// Claude Code v2.1.56+ writes OAuth tokens into the macOS login keychain
// under a service name namespaced by CLAUDE_CONFIG_DIR, allowing multiple
// profiles to hold independent tokens side by side.
//
//	service = "Claude Code-credentials-<sha256(abs(CLAUDE_CONFIG_DIR))[:8]>"
//	account = "<OS user>" (claude uses the current user; go-keyring needs a non-empty account)
//
// We read the entry via go-keyring (which wraps Security framework calls) so
// the existing keychain permissions flow works.

const (
	claudeKeychainServicePrefix = "Claude Code-credentials"
)

// KeychainService returns the expected macOS keychain service name for the
// given CLAUDE_CONFIG_DIR. The directory is absolutized before hashing so
// ccpm and Claude Code always agree on the namespace.
func KeychainService(profileDir string) (string, error) {
	abs, err := filepath.Abs(profileDir)
	if err != nil {
		return "", fmt.Errorf("resolving profile dir: %w", err)
	}
	sum := sha256.Sum256([]byte(abs))
	return fmt.Sprintf("%s-%s", claudeKeychainServicePrefix, hex.EncodeToString(sum[:])[:8]), nil
}

// claudeOAuthKeychainPayload mirrors the JSON Claude Code serializes into
// the keychain secret.
type claudeOAuthKeychainPayload struct {
	ClaudeAIOauth struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		// ExpiresAt is epoch milliseconds in claude's payload.
		ExpiresAt int64  `json:"expiresAt"`
		Scopes    []any  `json:"scopes,omitempty"`
		Email     string `json:"email,omitempty"`
	} `json:"claudeAiOauth"`
}

// MacKeychainOAuth is the parsed, high-level view of Claude Code's keychain entry.
type MacKeychainOAuth struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Email        string
	Raw          string // raw JSON as stored, for backup round-trips
}

// commonKeychainAccounts are the possible account values Claude Code uses on
// macOS. Different versions have flipped between the active user name, the
// literal "Claude Code", and the empty string, so we try each in order.
var commonKeychainAccounts = []string{
	"Claude Code",
	"claude-code",
	"default",
}

// ReadMacKeychainOAuth reads Claude Code's namespaced keychain entry for the
// given profile directory. Returns (nil, nil) if the entry is absent. Returns
// an error for any other failure (parse error, permission denied, etc.).
func ReadMacKeychainOAuth(profileDir string) (*MacKeychainOAuth, error) {
	service, err := KeychainService(profileDir)
	if err != nil {
		return nil, err
	}

	raw, err := readKeychainAnyAccount(service)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading keychain entry %q: %w", service, err)
	}

	var parsed claudeOAuthKeychainPayload
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("parsing keychain payload: %w", err)
	}

	out := &MacKeychainOAuth{
		AccessToken:  parsed.ClaudeAIOauth.AccessToken,
		RefreshToken: parsed.ClaudeAIOauth.RefreshToken,
		Email:        parsed.ClaudeAIOauth.Email,
		Raw:          raw,
	}
	if parsed.ClaudeAIOauth.ExpiresAt > 0 {
		out.ExpiresAt = time.UnixMilli(parsed.ClaudeAIOauth.ExpiresAt)
	}
	return out, nil
}

// WriteMacKeychainOAuth writes a raw JSON payload back into the namespaced
// keychain entry for the given profile dir. Used for `ccpm auth restore`.
func WriteMacKeychainOAuth(profileDir string, raw string) error {
	service, err := KeychainService(profileDir)
	if err != nil {
		return err
	}
	// Prefer the first account we already stored under; if none exists, pick
	// the first default so restores work even on a fresh machine.
	account := commonKeychainAccounts[0]
	if existing, which, err := readKeychainAnyAccountWithName(service); err == nil && existing != "" {
		account = which
	}
	return keyring.Set(service, account, raw)
}

// DeleteMacKeychainOAuth removes the namespaced keychain entry for a profile.
// Used during `ccpm remove`. Returns nil if the entry is absent.
func DeleteMacKeychainOAuth(profileDir string) error {
	service, err := KeychainService(profileDir)
	if err != nil {
		return err
	}
	var firstErr error
	for _, account := range commonKeychainAccounts {
		if err := keyring.Delete(service, account); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func readKeychainAnyAccount(service string) (string, error) {
	raw, _, err := readKeychainAnyAccountWithName(service)
	return raw, err
}

func readKeychainAnyAccountWithName(service string) (string, string, error) {
	var lastErr error
	for _, account := range commonKeychainAccounts {
		v, err := keyring.Get(service, account)
		if err == nil {
			return v, account, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = keyring.ErrNotFound
	}
	return "", "", lastErr
}
