package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/keystore"
)

type Vault struct {
	Store keystore.Store
}

func New(store keystore.Store) *Vault {
	return &Vault{Store: store}
}

func (v *Vault) Backup(profileName string, data []byte) error {
	key, err := v.Store.GetOrCreateVaultMasterKey()
	if err != nil {
		return fmt.Errorf("getting vault master key: %w", err)
	}

	encrypted, err := encrypt(data, key)
	if err != nil {
		return fmt.Errorf("encrypting credentials: %w", err)
	}

	vaultDir, err := config.VaultDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(vaultDir, 0755); err != nil {
		return fmt.Errorf("creating vault directory: %w", err)
	}

	path := filepath.Join(vaultDir, profileName+".enc")
	if err := os.WriteFile(path, encrypted, 0600); err != nil {
		return fmt.Errorf("writing vault file: %w", err)
	}

	return nil
}

func (v *Vault) Restore(profileName string) ([]byte, error) {
	key, err := v.Store.GetOrCreateVaultMasterKey()
	if err != nil {
		return nil, fmt.Errorf("getting vault master key: %w", err)
	}

	vaultDir, err := config.VaultDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(vaultDir, profileName+".enc")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading vault file: %w", err)
	}

	decrypted, err := decrypt(data, key)
	if err != nil {
		return nil, fmt.Errorf("decrypting credentials: %w", err)
	}

	return decrypted, nil
}

func (v *Vault) Exists(profileName string) bool {
	vaultDir, err := config.VaultDir()
	if err != nil {
		return false
	}
	path := filepath.Join(vaultDir, profileName+".enc")
	_, err = os.Stat(path)
	return err == nil
}

func (v *Vault) Remove(profileName string) error {
	vaultDir, err := config.VaultDir()
	if err != nil {
		return err
	}
	path := filepath.Join(vaultDir, profileName+".enc")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing vault file: %w", err)
	}
	return nil
}

func encrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func decrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ct, nil)
}
