//go:build darwin

package credentials

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

// securityQuote feeds the OAuth payload to `security -i` via stdin; a quoting
// bug here corrupts the stored token and breaks every profile's login, so the
// escape rules are pinned exhaustively.
func TestSecurityQuote(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"plain", `"plain"`},
		{`with"quote`, `"with\"quote"`},
		{`back\slash`, `"back\\slash"`},
		{`{"claudeAiOauth":{"accessToken":"a\"b"}}`, `"{\"claudeAiOauth\":{\"accessToken\":\"a\\\"b\"}}"`},
		{"spaces and -flags --too", `"spaces and -flags --too"`},
		{"", `""`},
	}
	for _, tc := range cases {
		got, err := securityQuote(tc.in)
		if err != nil {
			t.Errorf("securityQuote(%q) error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("securityQuote(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestSecurityQuoteRejectsControlChars(t *testing.T) {
	for _, in := range []string{"line\nbreak", "carriage\rreturn", "tab\there", "nul\x00byte", "del\x7f"} {
		if _, err := securityQuote(in); err == nil {
			t.Errorf("securityQuote(%q) = nil error, want rejection", in)
		}
	}
}

func TestSecurityQuoteRoundTripShape(t *testing.T) {
	// The quoted form must always be a single double-quoted token with no
	// unescaped inner quotes, so the security -i tokenizer reads exactly one
	// argument.
	payload := `{"claudeAiOauth":{"accessToken":"sk-ant-oat01-xyz","refreshToken":"sk-ant-ort01-abc","expiresAt":1760000000000}}`
	q, err := securityQuote(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(q, `"`) || !strings.HasSuffix(q, `"`) {
		t.Fatalf("quoted form not wrapped: %s", q)
	}
	inner := q[1 : len(q)-1]
	for i := 0; i < len(inner); i++ {
		if inner[i] == '"' {
			if i == 0 || inner[i-1] != '\\' {
				t.Fatalf("unescaped quote at %d in %s", i, q)
			}
		}
	}
}

// Claude Code's keychain payload can exceed 4 KB (mcpOAuth tokens). `security
// -i` truncates longer lines and still runs the cut-off upsert, replacing the
// good entry with invalid JSON, so securityAddLine must refuse them up front.
func TestSecurityAddLineRefusesOverLimit(t *testing.T) {
	const svc, acct = "Claude Code-credentials-0123abcd", "someone"
	base, err := securityAddLine(svc, acct, "")
	if err != nil {
		t.Fatal(err)
	}
	fits := strings.Repeat("a", securityMaxLine-len(base))
	line, err := securityAddLine(svc, acct, fits)
	if err != nil {
		t.Fatalf("line of exactly %d bytes refused: %v", securityMaxLine, err)
	}
	if len(line) != securityMaxLine {
		t.Fatalf("test setup: line is %d bytes, want %d", len(line), securityMaxLine)
	}
	for _, n := range []int{len(fits) + 1, 4100, 16 << 10} {
		payload := `{"claudeAiOauth":{"accessToken":"` + strings.Repeat("x", n) + `"}}`
		if _, err := securityAddLine(svc, acct, payload); !errors.Is(err, ErrKeychainPayloadTooLarge) {
			t.Errorf("payload of %d bytes: err = %v, want ErrKeychainPayloadTooLarge (security -i would truncate it and overwrite the good entry)", len(payload), err)
		}
	}
}

// Touches the real login keychain under a throwaway service derived from a
// temp dir, so it is opt-in: CCPM_KEYCHAIN_TESTS=1 go test ./internal/credentials/
func TestWriteMacKeychainOAuth_FailedWriteKeepsEntries(t *testing.T) {
	if os.Getenv("CCPM_KEYCHAIN_TESTS") != "1" {
		t.Skip("set CCPM_KEYCHAIN_TESTS=1 to run against the login keychain")
	}
	dir := t.TempDir()
	service, err := KeychainService(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = DeleteMacKeychainOAuth(dir) })
	primary := keychainAccounts()[0]
	readPrimary := func() string {
		out, err := exec.Command("/usr/bin/security", "find-generic-password", "-s", service, "-a", primary, "-w").Output()
		if err != nil {
			t.Fatalf("reading primary entry: %v", err)
		}
		return strings.TrimSuffix(string(out), "\n")
	}

	good := `{"claudeAiOauth":{"accessToken":"good"}}`
	if err := WriteMacKeychainOAuth(dir, good); err != nil {
		t.Fatal(err)
	}
	if err := keyring.Set(service, "Claude Code", "stale"); err != nil {
		t.Fatal(err)
	}

	huge := `{"claudeAiOauth":{"accessToken":"` + strings.Repeat("x", 4100) + `"}}`
	if err := WriteMacKeychainOAuth(dir, huge); !errors.Is(err, ErrKeychainPayloadTooLarge) {
		t.Fatalf("oversize write: err = %v, want ErrKeychainPayloadTooLarge", err)
	}
	if got := readPrimary(); got != good {
		t.Fatalf("failed write clobbered the good entry: now %d bytes", len(got))
	}
	if _, err := keyring.Get(service, "Claude Code"); err != nil {
		t.Fatalf("failed write deleted the other-account entry: %v", err)
	}

	// A successful write still prunes the stale account.
	if err := WriteMacKeychainOAuth(dir, good); err != nil {
		t.Fatal(err)
	}
	if _, err := keyring.Get(service, "Claude Code"); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("stale account not pruned after successful write: %v", err)
	}
}
