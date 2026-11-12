//go:build darwin

package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A key on argv is readable by every local user through `ps`.
func TestCreateFromPresetSendsKeyOnStdinOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	fakeCCPM(t, `printf '%s\n' "$@" > "$HOME/argv"; /bin/cat > "$HOME/stdin"`)

	const key = "sk-or-v1-secret"
	if r := NewMutate().CreateFromPreset("router", "openrouter", key); !r.OK {
		t.Fatalf("CreateFromPreset: %+v", r)
	}
	argv, err := os.ReadFile(filepath.Join(home, "argv"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Fields(string(argv)), []string{"add", "--preset", "openrouter", "--", "router"}; strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("argv = %v, want %v", got, want)
	}
	if strings.Contains(string(argv), key) {
		t.Error("key reached argv")
	}
	stdin, err := os.ReadFile(filepath.Join(home, "stdin"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stdin) != key+"\n" {
		t.Errorf("stdin = %q", stdin)
	}
}

func TestCreateFromPresetRejectsBadName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fakeCCPM(t, `exit 0`)
	if r := NewMutate().CreateFromPreset("../evil", "openrouter", "k"); r.OK || r.Error == "" {
		t.Fatalf("bad name accepted: %+v", r)
	}
}
