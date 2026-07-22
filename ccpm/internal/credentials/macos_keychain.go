//go:build darwin

package credentials

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
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

// keychainAccounts returns the candidate account values Claude Code may use
// on macOS, ordered by current likelihood. Claude Code v2.x writes the entry
// under the OS user name (verified against `security dump-keychain`), so that
// must be tried first; the others remain as fallbacks for legacy entries or
// future drift. The list is built per-call because the OS user is only known
// at runtime.
func keychainAccounts() []string {
	out := make([]string, 0, 4)
	if u, err := user.Current(); err == nil && u.Username != "" {
		out = append(out, u.Username)
	}
	out = append(out, "Claude Code", "claude-code", "default")
	return out
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
// keychain entry for the given profile dir. Used for `ccpm auth restore`,
// `ccpm set-default`, and `ccpm rename` (to migrate the entry across the
// dir-hash namespace change).
//
// We deliberately bypass zalando/go-keyring on the write path: that library
// wraps every macOS keychain value with a `go-keyring-base64:<base64>` prefix
// to defend against binary data and macOS truncation quirks. Claude Code
// reads the keychain with the macOS Security framework directly and sees
// only the literal stored bytes, so a go-keyring-wrapped payload fails to
// parse as JSON and the user is prompted to re-login. Shelling out to the
// `security` CLI gives us byte-identical, prefix-free storage that matches
// what `claude` itself writes during initial login.
//
// We always write under the current OS user (the account Claude Code reads
// from). To avoid a stale entry under a different account name shadowing the
// fresh write, every other known account name under the same service is
// deleted first. Without this cleanup, a previous broken `set-default` that
// wrote under "Claude Code" would silently keep being read by older clients.
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
	// `-U` updates if the entry already exists, otherwise creates it. The
	// whole command — including the token payload — is fed to `security -i`
	// via stdin so the secret never appears in argv, where any same-UID
	// process (or ps-polling telemetry/EDR agent) could read it.
	qService, err := securityQuote(service)
	if err != nil {
		return fmt.Errorf("quoting keychain service: %w", err)
	}
	qAccount, err := securityQuote(primary)
	if err != nil {
		return fmt.Errorf("quoting keychain account: %w", err)
	}
	qPayload, err := securityQuote(raw)
	if err != nil {
		return fmt.Errorf("quoting keychain payload: %w", err)
	}
	line := fmt.Sprintf("add-generic-password -U -s %s -a %s -w %s\n", qService, qAccount, qPayload)
	cmd := exec.Command("/usr/bin/security", "-i")
	cmd.Stdin = strings.NewReader(line)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("writing keychain entry via security CLI: %w (output: %s)", err, out)
	}
	return nil
}

// securityQuote wraps s in double quotes for the `security -i` command
// tokenizer, escaping backslashes and embedded quotes. The interactive parser
// is line-based, so control characters (which never occur in the compact-JSON
// OAuth payload) are rejected rather than escaped.
func securityQuote(s string) (string, error) {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("value contains control character %q", r)
		}
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`, nil
}

// DeleteMacKeychainOAuth removes every OAuth entry (all known account names)
// from the keychain slot derived from the given Claude config dir. Used during
// `ccpm remove` for a profile dir, and by `ccpm set-default` on the ~/.claude
// default slot when switching to an API-key profile, so IDE extensions cannot
// silently keep using a stale OAuth token. Returns nil if the entry is absent.
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

func readKeychainAnyAccount(service string) (string, error) {
	raw, _, err := readKeychainAnyAccountWithName(service)
	return raw, err
}

func readKeychainAnyAccountWithName(service string) (string, string, error) {
	var lastErr error
	for _, account := range keychainAccounts() {
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
