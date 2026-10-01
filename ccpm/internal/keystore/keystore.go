package keystore

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
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
	// DeleteVaultMasterKey removes the vault master key. Only `ccpm uninstall`
	// calls it, after every vault file it encrypts is gone. Absent is not an error.
	DeleteVaultMasterKey() error
	// GetVaultMasterKey never creates a key: reads (vault restore) must not
	// mint a new one, or existing backups become undecryptable.
	GetVaultMasterKey() ([]byte, error)
}

// ErrVaultKeyNotFound is returned by GetVaultMasterKey when the keychain has
// no vault master key.
var ErrVaultKeyNotFound = errors.New("vault master key not found in keychain")

// Indirection so tests can simulate keychain failures other than not-found.
var (
	keyringGet = keyring.Get
	keyringSet = keyring.Set
)

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
	key, err := s.GetVaultMasterKey()
	if !errors.Is(err, ErrVaultKeyNotFound) {
		// Found, or a real failure (locked keychain, denied ACL prompt, bad
		// decode). Creating on anything but not-found would overwrite the real
		// key and make every existing .enc backup permanently undecryptable.
		return key, err
	}

	key = make([]byte, vaultKeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generating master key: %w", err)
	}

	if err := keyringSet(serviceVault, vaultAccount, base64.StdEncoding.EncodeToString(key)); err != nil {
		return nil, fmt.Errorf("storing master key in keychain: %w", err)
	}

	return key, nil
}

func (s *SystemStore) DeleteVaultMasterKey() error {
	err := keyring.Delete(serviceVault, vaultAccount)
	if err == keyring.ErrNotFound {
		return nil
	}
	return err
}

func (s *SystemStore) GetVaultMasterKey() ([]byte, error) {
	existing, err := keyringGet(serviceVault, vaultAccount)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, ErrVaultKeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("reading vault master key from keychain: %w", err)
	}
	key, legacy, err := decodeVaultKey(existing)
	if err != nil {
		return nil, fmt.Errorf("decoding master key from keychain: %w", err)
	}
	if legacy {
		// Migrate the raw-byte legacy entry to base64 immediately so the
		// length-based fallback below can be removed.
		if err := keyringSet(serviceVault, vaultAccount, base64.StdEncoding.EncodeToString(key)); err != nil {
			return nil, fmt.Errorf("migrating legacy master key to base64: %w", err)
		}
		fmt.Fprintln(os.Stderr, "ccpm: migrated vault master key to base64 keychain encoding")
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
	key, err := m.GetVaultMasterKey()
	if !errors.Is(err, ErrVaultKeyNotFound) {
		return key, err
	}
	key = make([]byte, vaultKeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	m.data[serviceVault+"/"+vaultAccount] = base64.StdEncoding.EncodeToString(key)
	return key, nil
}

func (m *MemoryStore) DeleteVaultMasterKey() error {
	delete(m.data, serviceVault+"/"+vaultAccount)
	return nil
}

func (m *MemoryStore) GetVaultMasterKey() ([]byte, error) {
	existing, ok := m.data[serviceVault+"/"+vaultAccount]
	if !ok {
		return nil, ErrVaultKeyNotFound
	}
	key, legacy, err := decodeVaultKey(existing)
	if err != nil {
		return nil, err
	}
	if legacy {
		m.data[serviceVault+"/"+vaultAccount] = base64.StdEncoding.EncodeToString(key)
	}
	return key, nil
}
