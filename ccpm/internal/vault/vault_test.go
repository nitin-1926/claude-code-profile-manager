package vault

import (
	"bytes"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/keystore"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		plaintext []byte
	}{
		{"simple", []byte("hello world")},
		{"empty", []byte("")},
		{"json", []byte(`{"accessToken":"sk-ant-api03-abc","expiresAt":"2026-12-31T00:00:00Z"}`)},
		{"binary", func() []byte { b := make([]byte, 256); rand.Read(b); return b }()},
		{"large", bytes.Repeat([]byte("x"), 10000)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := encrypt(tt.plaintext, key)
			if err != nil {
				t.Fatalf("encrypt() error: %v", err)
			}

			// Encrypted should differ from plaintext
			if len(tt.plaintext) > 0 && bytes.Equal(encrypted, tt.plaintext) {
				t.Error("Encrypted data should differ from plaintext")
			}

			decrypted, err := decrypt(encrypted, key)
			if err != nil {
				t.Fatalf("decrypt() error: %v", err)
			}

			if !bytes.Equal(decrypted, tt.plaintext) {
				t.Errorf("Roundtrip failed: got %q, want %q", decrypted, tt.plaintext)
			}
		})
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	rand.Read(key1)
	rand.Read(key2)

	plaintext := []byte("secret credentials")
	encrypted, err := encrypt(plaintext, key1)
	if err != nil {
		t.Fatal(err)
	}

	_, err = decrypt(encrypted, key2)
	if err == nil {
		t.Error("decrypt() with wrong key should fail")
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	encrypted, _ := encrypt([]byte("secret"), key)

	// Tamper with the ciphertext
	encrypted[len(encrypted)-1] ^= 0xff

	_, err := decrypt(encrypted, key)
	if err == nil {
		t.Error("decrypt() with tampered ciphertext should fail")
	}
}

func TestDecryptTooShort(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	_, err := decrypt([]byte("short"), key)
	if err == nil {
		t.Error("decrypt() with too-short data should fail")
	}
}

// Restore only reads. If it minted a master key when none is found, a later
// keychain hiccup-then-recovery would leave two keys in play and backups
// written under the real one undecryptable.
func TestRestore_NeverCreatesMasterKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store := keystore.NewMemoryStore()

	if _, err := New(store).Restore("work"); !errors.Is(err, keystore.ErrVaultKeyNotFound) {
		t.Fatalf("Restore with no master key: err = %v, want ErrVaultKeyNotFound", err)
	}
	if _, err := store.GetVaultMasterKey(); !errors.Is(err, keystore.ErrVaultKeyNotFound) {
		t.Fatalf("Restore created a vault master key (GetVaultMasterKey err = %v)", err)
	}
}

func TestBackupRestoreRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	v := New(keystore.NewMemoryStore())
	for _, payload := range []string{"first", "second"} {
		if err := v.Backup("work", []byte(payload)); err != nil {
			t.Fatal(err)
		}
		got, err := v.Restore("work")
		if err != nil || string(got) != payload {
			t.Fatalf("Restore = %q, %v; want %q", got, err, payload)
		}
	}
}

// Backup replaces the previous good backup, so it must go through
// atomicwrite (stage + rename): a crash or full disk mid-write must not leave
// a truncated .enc, and a symlink planted at the backup path must not be
// followed to clobber a file outside the vault.
func TestBackup_DoesNotWriteThroughSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	v := New(keystore.NewMemoryStore())
	if err := v.Backup("work", []byte("seed")); err != nil {
		t.Fatal(err)
	}
	vaultDir, err := config.VaultDir()
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(home, "outside.txt")
	if err := os.WriteFile(outside, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	encPath := filepath.Join(vaultDir, "work.enc")
	if err := os.Remove(encPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, encPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	if err := v.Backup("work", []byte("new")); err == nil {
		t.Error("Backup over a symlinked .enc succeeded, want refusal")
	}
	if got, _ := os.ReadFile(outside); string(got) != "untouched" {
		t.Fatalf("Backup wrote through the symlink: outside file now %q", got)
	}
}

func TestEncryptProducesUniqueNonces(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	plaintext := []byte("same plaintext")

	enc1, _ := encrypt(plaintext, key)
	enc2, _ := encrypt(plaintext, key)

	if bytes.Equal(enc1, enc2) {
		t.Error("Two encryptions of same plaintext should produce different ciphertext (unique nonces)")
	}
}
