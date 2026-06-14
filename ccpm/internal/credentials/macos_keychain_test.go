//go:build darwin

package credentials

import (
	"strings"
	"testing"
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
