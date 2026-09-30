package keystore

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
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

	got, isLegacy, err := decodeVaultKey(encoded)
	if err != nil {
		t.Fatalf("valid base64: %v", err)
	}
	if isLegacy {
		t.Error("base64 key must not be flagged legacy")
	}
	if !bytes.Equal(got, valid) {
		t.Error("decoded != original")
	}

	// Legacy: exactly 32 bytes of string data round-trips as raw. Regression
	// guard: the decoder must not reject a legacy key so existing installs
	// stay openable after upgrade — but it must flag it for migration.
	legacy := string(valid)
	got, isLegacy, err = decodeVaultKey(legacy)
	if err != nil {
		t.Fatalf("legacy raw: %v", err)
	}
	if !isLegacy {
		t.Error("raw 32-byte key must be flagged legacy")
	}
	if !bytes.Equal(got, valid) {
		t.Error("legacy path did not round-trip correctly")
	}

	// Wrong length must fail loud rather than returning a truncated key.
	if _, _, err := decodeVaultKey("short"); err == nil {
		t.Error("short string should error")
	}
}

// TestMemoryStore_LegacyKeyMigratesOnRead seeds a raw legacy key and verifies
// the first read re-stores it base64-encoded with identical bytes.
func TestMemoryStore_LegacyKeyMigratesOnRead(t *testing.T) {
	raw := make([]byte, vaultKeyBytes)
	for i := range raw {
		raw[i] = byte(40 + i)
	}
	s := NewMemoryStore()
	mem := s.(*MemoryStore)
	mem.data[serviceVault+"/"+vaultAccount] = string(raw)

	key, err := s.GetOrCreateVaultMasterKey()
	if err != nil {
		t.Fatalf("read legacy: %v", err)
	}
	if !bytes.Equal(key, raw) {
		t.Error("legacy key bytes changed during migration")
	}
	stored := mem.data[serviceVault+"/"+vaultAccount]
	decoded, err := base64.StdEncoding.DecodeString(stored)
	if err != nil {
		t.Fatalf("stored value must be base64 after migration (got %q): %v", stored, err)
	}
	if !bytes.Equal(decoded, raw) {
		t.Error("migrated stored bytes != original key")
	}
}

// stubKeyring swaps the keyring seam for the duration of the test and records
// every Set, so tests can assert the real master key is never overwritten.
func stubKeyring(t *testing.T, getErr error) *[]string {
	t.Helper()
	origGet, origSet := keyringGet, keyringSet
	t.Cleanup(func() { keyringGet, keyringSet = origGet, origSet })
	var sets []string
	keyringGet = func(service, user string) (string, error) { return "", getErr }
	keyringSet = func(service, user, pass string) error {
		sets = append(sets, service+"/"+user)
		return nil
	}
	return &sets
}

// A keychain read failure that is NOT "not found" (locked keychain, the user
// clicking Deny on the ACL prompt) must not be treated as "no key yet":
// minting and storing a new key would overwrite the real one and make every
// existing .enc backup permanently undecryptable.
func TestSystemStore_GetOrCreateVaultMasterKey_ReadErrorNeverCreates(t *testing.T) {
	denied := errors.New("exit status 128: user interaction is not allowed")
	sets := stubKeyring(t, denied)

	key, err := New().GetOrCreateVaultMasterKey()
	if !errors.Is(err, denied) {
		t.Fatalf("err = %v, want the keychain read error surfaced", err)
	}
	if key != nil {
		t.Errorf("returned a key despite the read error")
	}
	if len(*sets) != 0 {
		t.Fatalf("keyring.Set called %v after a non-not-found read error: this overwrites the real vault master key", *sets)
	}
}

func TestSystemStore_GetOrCreateVaultMasterKey_CreatesOnlyWhenNotFound(t *testing.T) {
	sets := stubKeyring(t, keyring.ErrNotFound)

	key, err := New().GetOrCreateVaultMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != vaultKeyBytes || len(*sets) != 1 {
		t.Fatalf("got %d-byte key and sets %v, want a new %d-byte key stored once", len(key), *sets, vaultKeyBytes)
	}
}

func TestGetVaultMasterKey_NeverCreates(t *testing.T) {
	sets := stubKeyring(t, keyring.ErrNotFound)
	if _, err := New().GetVaultMasterKey(); !errors.Is(err, ErrVaultKeyNotFound) {
		t.Fatalf("SystemStore: err = %v, want ErrVaultKeyNotFound", err)
	}
	if len(*sets) != 0 {
		t.Fatalf("SystemStore.GetVaultMasterKey stored a key: %v", *sets)
	}

	m := NewMemoryStore()
	if _, err := m.GetVaultMasterKey(); !errors.Is(err, ErrVaultKeyNotFound) {
		t.Fatalf("MemoryStore: err = %v, want ErrVaultKeyNotFound", err)
	}
	if len(m.(*MemoryStore).data) != 0 {
		t.Fatal("MemoryStore.GetVaultMasterKey stored a key")
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
