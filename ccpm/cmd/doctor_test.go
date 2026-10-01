package cmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestDoctorCorruptConfigExitsUnhealthy: a corrupt config.json printed
// "Config load failed" and returned nil — exit 0, dropping every issue already
// counted — although doctor's contract is exit 4 whenever it finds issues.
func TestDoctorCorruptConfigExitsUnhealthy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub claude binary is a shell script")
	}
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ccpm"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ccpm", "config.json"), []byte("{corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Stub claude so doctor never runs a real binary for `--version`.
	stub := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\necho 9.9.9\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	c := exec.Command(os.Args[0])
	c.Env = append(os.Environ(),
		"CCPM_TEST_EXECUTE=doctor",
		"HOME="+home,
		"USERPROFILE="+home,
		"CLAUDE_BINARY="+stub,
		"NO_COLOR=1",
		"CCPM_NO_TTY=1",
	)
	out, err := c.CombinedOutput()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != exitUnhealthy {
		t.Fatalf("doctor on corrupt config: err %v, want exit %d; output:\n%s", err, exitUnhealthy, out)
	}
	if !strings.Contains(string(out), "parsing config") {
		t.Fatalf("output does not name the config problem:\n%s", out)
	}
}

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2.1.56", "2.1.56", 0},
		{"2.1.56", "2.1.55", 1},
		{"2.1.55", "2.1.56", -1},
		{"2.1.56", "2.1.5", 1},   // numeric compare, not lexicographic
		{"v2.1.56", "2.1.56", 0}, // "v" prefix tolerated
		{"2.1.56 (claude-code)", "2.1.56", 0},
		{"2.0.0", "2.1.0", -1},
		{"2.10.0", "2.9.0", 1},
		{"", "2.1.0", -1}, // empty parses as 0; 0 < 2
		{"2.1.0", "", 1},
	}
	for _, c := range cases {
		if got := compareSemver(c.a, c.b); got != c.want {
			t.Errorf("compareSemver(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestNormalizeSemver(t *testing.T) {
	cases := map[string]string{
		"v2.1.56":              "v2.1.56",
		"2.1.56 (claude-code)": "v2.1.56",
		"   v0.3.2  ":          "v0.3.2",
		"abc":                  "v0.0.0",
		"":                     "v0.0.0",
		"1.2.3-beta":           "v1.2.3-beta",
		// x/mod/semver accepts and canonicalizes short forms itself.
		"2.1": "v2.1",
		"2":   "v2",
	}
	for in, want := range cases {
		if got := normalizeSemver(in); got != want {
			t.Errorf("normalizeSemver(%q) = %q, want %q", in, got, want)
		}
	}
}
