//go:build !darwin

package credentials

import (
	"errors"
	"time"
)

// MacKeychainOAuth is a stub outside darwin so callers can compile against a
// common API. All functions here return ErrNotDarwin or no-op.
type MacKeychainOAuth struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Email        string
	Raw          string
}

var ErrNotDarwin = errors.New("macOS keychain access is only available on darwin")

func KeychainService(profileDir string) (string, error) {
	return "", ErrNotDarwin
}

func ReadMacKeychainOAuth(profileDir string) (*MacKeychainOAuth, error) {
	return nil, nil
}

func WriteMacKeychainOAuth(profileDir string, raw string) error {
	return ErrNotDarwin
}

func DeleteMacKeychainOAuth(profileDir string) error {
	return nil
}

func DeleteMacKeychainOAuthDefault(homeClaudeDir string) error {
	return nil
}
