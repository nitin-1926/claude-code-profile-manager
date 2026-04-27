package keystore

import (
	"bytes"
	"encoding/base64"
	"testing"
)

// TestMemoryStore_VaultMasterKeyRoundTrip ensures the master key is stored
// base64-encoded (not as a raw string cast from random bytes) and that a
// second read returns the same 32 bytes.
func TestMemoryStore_VaultMasterKeyRoundTrip(t *testing.T) {
	s := NewMemoryStore()

	first, err := s.GetOrCreateVaultMasterKey()
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(first) != vaultKeyBytes {
		t.Fatalf("want %d bytes, got %d", vaultKeyBytes, len(first))
	}

	second, err := s.GetOrCreateVaultMasterKey()
	if err != nil {
		t.Fatalf("reread: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Error("second call must return the same key")
	}

	// Implementation detail: the stored form is base64, so decoding it round-
	// trips to the same bytes. This protects against a regression where
	// random bytes are cast to string — on Linux kwallet/secret-service that
	// round-trip can silently truncate at invalid UTF-8 or NUL.
	mem := s.(*MemoryStore)
	stored := mem.data[serviceVault+"/"+vaultAccount]
	decoded, err := base64.StdEncoding.DecodeString(stored)
	if err != nil {
		t.Fatalf("stored value must be base64 (got %q): %v", stored, err)
	}
	if !bytes.Equal(decoded, first) {
		t.Error("decoded bytes must match returned key")
	}
}

func TestDecodeVaultKey(t *testing.T) {
	valid := make([]byte, vaultKeyBytes)
	for i := range valid {
		valid[i] = byte(i)
	}
	encoded := base64.StdEncoding.EncodeToString(valid)

	got, err := decodeVaultKey(encoded)
	if err != nil {
		t.Fatalf("valid base64: %v", err)
	}
	if !bytes.Equal(got, valid) {
		t.Error("decoded != original")
	}

	// Legacy: exactly 32 bytes of string data round-trips as raw. Regression
	// guard: the decoder must not reject a legacy key so existing installs
	// stay openable after upgrade.
	legacy := string(valid)
	got, err = decodeVaultKey(legacy)
	if err != nil {
		t.Fatalf("legacy raw: %v", err)
	}
	if !bytes.Equal(got, valid) {
		t.Error("legacy path did not round-trip correctly")
	}

	// Wrong length must fail loud rather than returning a truncated key.
	if _, err := decodeVaultKey("short"); err == nil {
		t.Error("short string should error")
	}
}

func TestMemoryStore_APIKey(t *testing.T) {
	s := NewMemoryStore()
	if err := s.SetAPIKey("work", "sk-test"); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := s.GetAPIKey("work")
	if err != nil || got != "sk-test" {
		t.Errorf("get = %q, %v", got, err)
	}
	if err := s.DeleteAPIKey("work"); err != nil {
		t.Errorf("delete: %v", err)
	}
	if _, err := s.GetAPIKey("work"); err == nil {
		t.Error("get after delete should error")
	}
}
