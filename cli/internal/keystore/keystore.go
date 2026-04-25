package keystore

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

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
		key, decodeErr := decodeVaultKey(existing)
		if decodeErr != nil {
			return nil, fmt.Errorf("decoding master key from keychain: %w", decodeErr)
		}
		return key, nil
	}

	key := make([]byte, vaultKeyBytes)
