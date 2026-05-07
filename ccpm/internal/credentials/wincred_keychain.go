//go:build windows

package credentials

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/user"
	"path/filepath"
	"time"

	"github.com/zalando/go-keyring"
)

// Windows port of the macOS keychain handler. Claude Code v2.x on Windows
// stores OAuth tokens in Windows Credential Manager (wincred) under a
// service name namespaced by CLAUDE_CONFIG_DIR — the same shape used on
// macOS, just with a different OS API. go-keyring transparently dispatches
// to wincred on Windows, so the implementation mirrors macos_keychain.go.
//
// THEORETICAL — UNVERIFIED ON A REAL WINDOWS HOST.
//
// The exact service-name and account-name conventions used by Claude Code
// on Windows are assumed identical to macOS. Both targets pass through the
// same upstream JS code path, so this is a reasonable starting point, but
// the first Windows user to exercise `ccpm set-default <oauth>` should
// verify with `cmdkey /list` (or via Credential Manager) that the entry is
// keyed by `Claude Code-credentials-<sha256(abs(CLAUDE_CONFIG_DIR))[:8]>`
// with the OS user as account. Any drift here is a one-line fix.

const claudeKeychainServicePrefix = "Claude Code-credentials"

// MacKeychainOAuth keeps its name across platforms to avoid churn at every
// call site. Treat it as the cross-platform OAuth payload — the "Mac" prefix
// is historical.
type MacKeychainOAuth struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Email        string
	Raw          string
}

type claudeOAuthKeychainPayload struct {
	ClaudeAIOauth struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresAt    int64  `json:"expiresAt"`
		Scopes       []any  `json:"scopes,omitempty"`
		Email        string `json:"email,omitempty"`
	} `json:"claudeAiOauth"`
}

func keychainAccounts() []string {
	out := make([]string, 0, 4)
	if u, err := user.Current(); err == nil && u.Username != "" {
		out = append(out, u.Username)
	}
	out = append(out, "Claude Code", "claude-code", "default")
	return out
}

func KeychainService(profileDir string) (string, error) {
	abs, err := filepath.Abs(profileDir)
	if err != nil {
		return "", fmt.Errorf("resolving profile dir: %w", err)
	}
	sum := sha256.Sum256([]byte(abs))
	return fmt.Sprintf("%s-%s", claudeKeychainServicePrefix, hex.EncodeToString(sum[:])[:8]), nil
}

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
		return nil, fmt.Errorf("reading wincred entry %q: %w", service, err)
	}

	var parsed claudeOAuthKeychainPayload
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("parsing wincred payload: %w", err)
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

func WriteMacKeychainOAuth(profileDir string, raw string) error {
	service, err := KeychainService(profileDir)
	if err != nil {
		return err
	}
	accounts := keychainAccounts()
	primary := accounts[0]
	for _, account := range accounts[1:] {
		_ = keyring.Delete(service, account)
	}
	return keyring.Set(service, primary, raw)
}

func DeleteMacKeychainOAuth(profileDir string) error {
	service, err := KeychainService(profileDir)
	if err != nil {
		return err
	}
	var firstErr error
	for _, account := range keychainAccounts() {
		if err := keyring.Delete(service, account); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func DeleteMacKeychainOAuthDefault(homeClaudeDir string) error {
	service, err := KeychainService(homeClaudeDir)
	if err != nil {
		return err
	}
	var firstErr error
	for _, account := range keychainAccounts() {
		if err := keyring.Delete(service, account); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func readKeychainAnyAccount(service string) (string, error) {
	var lastErr error
	for _, account := range keychainAccounts() {
		v, err := keyring.Get(service, account)
		if err == nil {
			return v, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = keyring.ErrNotFound
	}
	return "", lastErr
}
