package keystore

import (
	"crypto/rand"
	"fmt"

	"github.com/zalando/go-keyring"
)

const (
	serviceAPI   = "ccpm"
	serviceVault = "ccpm-vault"
	vaultAccount = "master-key"
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
	existing, err := keyring.Get(serviceVault, vaultAccount)
	if err == nil {
		return []byte(existing), nil
	}

	// Generate new 32-byte key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generating master key: %w", err)
	}

	if err := keyring.Set(serviceVault, vaultAccount, string(key)); err != nil {
		return nil, fmt.Errorf("storing master key in keychain: %w", err)
	}

	return key, nil
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
		return []byte(existing), nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	m.data[serviceVault+"/"+vaultAccount] = string(key)
	return key, nil
}
