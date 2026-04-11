package vault

import (
	"bytes"
	"crypto/rand"
	"testing"
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
