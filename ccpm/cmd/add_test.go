package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAddScriptedStdinAnswersEachPromptInOrder: pickAuthMethod read stdin
// through a bufio.Scanner, which buffered the whole pipe, and the import
// wizard then read os.Stdin directly and saw EOF. So
// `printf '2\n1\n<key>\n' | ccpm add x` gave the wizard nothing and took the
// wizard's answer ("1") as the API key. The key here is deliberately too
// short so validation fails before anything reaches the keychain; the error
// reports its length, which tells us which line was read as the key.
func TestAddScriptedStdinAnswersEachPromptInOrder(t *testing.T) {
	home := t.TempDir()
	// An existing ~/.claude makes add run the import wizard between the auth
	// choice and the key prompt.
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	c := exec.Command(os.Args[0])
	c.Env = append(os.Environ(),
		"CCPM_TEST_EXECUTE=add scripted",
		"HOME="+home,
		"USERPROFILE="+home,
		"NO_COLOR=1",
		"CCPM_NO_TTY=1",
	)
	c.Stdin = strings.NewReader("2\n1\nshortkey\n")
	out, _ := c.CombinedOutput()
	if !strings.Contains(string(out), "too short (8 chars)") {
		t.Fatalf("API key was not read from the third line; output:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(home, ".ccpm", "profiles", "scripted")); !os.IsNotExist(err) {
		t.Fatalf("failed add left its profile dir behind (stat err %v)", err)
	}
}
