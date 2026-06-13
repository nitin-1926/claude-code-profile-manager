package keystore

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/zalando/go-keyring"
)

const (
	serviceAPI       = "ccpm"
	serviceVault     = "ccpm-vault"
	vaultAccount     = "master-key"
	vaultKeyBytes    = 32
	vaultLegacyBytes = 32
)

// Store defines the interface for keychain operations.
// This allows testing with a mock implementation.
type Store interface {
	SetAPIKey(profile, key string) error
	GetAPIKey(profile string) (string, error)
	DeleteAPIKey(profile string) error
	GetOrCreateVaultMasterKey() ([]byte, error)
}

// SystemStore uses the OS keychain via go-keyring.
type SystemStore struct{}

func New() Store {
	return &SystemStore{}
}

func (s *SystemStore) SetAPIKey(profile, key string) error {
	return keyring.Set(serviceAPI, profile, key)
}

func (s *SystemStore) GetAPIKey(profile string) (string, error) {
	key, err := keyring.Get(serviceAPI, profile)
	if err != nil {
		return "", fmt.Errorf("retrieving API key for profile %q: %w", profile, err)
	}
	return key, nil
}

func (s *SystemStore) DeleteAPIKey(profile string) error {
	err := keyring.Delete(serviceAPI, profile)
	if err == keyring.ErrNotFound {
		return nil
	}
	return err
}

func (s *SystemStore) GetOrCreateVaultMasterKey() ([]byte, error) {
	if existing, err := keyring.Get(serviceVault, vaultAccount); err == nil {
		key, legacy, decodeErr := decodeVaultKey(existing)
		if decodeErr != nil {
			return nil, fmt.Errorf("decoding master key from keychain: %w", decodeErr)
		}
		if legacy {
			// Migrate the raw-byte legacy entry to base64 immediately so the
			// length-based fallback below can be removed.
			if err := keyring.Set(serviceVault, vaultAccount, base64.StdEncoding.EncodeToString(key)); err != nil {
				return nil, fmt.Errorf("migrating legacy master key to base64: %w", err)
			}
			fmt.Fprintln(os.Stderr, "ccpm: migrated vault master key to base64 keychain encoding")
		}
		return key, nil
	}

	key := make([]byte, vaultKeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generating master key: %w", err)
	}

	if err := keyring.Set(serviceVault, vaultAccount, base64.StdEncoding.EncodeToString(key)); err != nil {
		return nil, fmt.Errorf("storing master key in keychain: %w", err)
	}

	return key, nil
}

// decodeVaultKey accepts a keychain-stored master key. New installs store the
// key base64-encoded so that arbitrary random bytes survive the UTF-8 layers in
// secret-service / kwallet / wincred. Legacy installs (pre-base64) stored the
// raw bytes cast to string; that path reports legacy=true so the caller can
// re-store the key base64-encoded right away instead of accepting any
// 32-byte keychain value as the master key forever.
//
// DEPRECATION: the legacy branch exists only for pre-base64 installs and
// should be removed once migrate-on-read has been in two released versions
// (added in the release after v0.5.1).
func decodeVaultKey(stored string) (key []byte, legacy bool, err error) {
	if decoded, err := base64.StdEncoding.DecodeString(stored); err == nil && len(decoded) == vaultKeyBytes {
		return decoded, false, nil
	}
	if len(stored) == vaultLegacyBytes {
		return []byte(stored), true, nil
	}
	return nil, false, fmt.Errorf("master key has unexpected length %d (expected base64 of %d bytes)", len(stored), vaultKeyBytes)
}

// MemoryStore is an in-memory implementation for testing.
type MemoryStore struct {
	data map[string]string
}

func NewMemoryStore() Store {
	return &MemoryStore{data: make(map[string]string)}
}

func (m *MemoryStore) SetAPIKey(profile, key string) error {
	m.data[serviceAPI+"/"+profile] = key
	return nil
}

func (m *MemoryStore) GetAPIKey(profile string) (string, error) {
	key, ok := m.data[serviceAPI+"/"+profile]
	if !ok {
		return "", fmt.Errorf("API key not found for profile %q", profile)
	}
	return key, nil
}

func (m *MemoryStore) DeleteAPIKey(profile string) error {
	delete(m.data, serviceAPI+"/"+profile)
	return nil
}

func (m *MemoryStore) GetOrCreateVaultMasterKey() ([]byte, error) {
	if existing, ok := m.data[serviceVault+"/"+vaultAccount]; ok {
		key, legacy, err := decodeVaultKey(existing)
		if err != nil {
			return nil, err
		}
		if legacy {
			m.data[serviceVault+"/"+vaultAccount] = base64.StdEncoding.EncodeToString(key)
		}
		return key, nil
	}
	key := make([]byte, vaultKeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	m.data[serviceVault+"/"+vaultAccount] = base64.StdEncoding.EncodeToString(key)
	return key, nil
}
