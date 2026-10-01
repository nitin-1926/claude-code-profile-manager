//go:build darwin

package services

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// realHome is read before any test repoints HOME.
var realHome = os.Getenv("HOME")

// realCCPM builds the ccpm CLI from this repo and puts it alone on PATH, so a
// test drives the same binary the app shells out to and sees output shaped the
// way it really is. Call after syntheticProfile: HOME must already point at a
// scratch dir so the CLI never touches the real ~/.ccpm.
func realCCPM(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds the ccpm CLI")
	}
	dir := t.TempDir()
	build := exec.Command("go", "build", "-o", filepath.Join(dir, "ccpm"), "github.com/nitin-1926/claude-code-profile-manager/ccpm")
	// The real HOME, so go finds its module and build caches instead of
	// downloading every dependency into the scratch HOME.
	build.Env = append(os.Environ(), "HOME="+realHome)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building ccpm: %v\n%s", err, out)
	}
	t.Setenv("PATH", dir)
}

// addSyntheticProfile registers one more empty profile beside syntheticProfile's.
func addSyntheticProfile(t *testing.T, name string) {
	t.Helper()
	home := os.Getenv("HOME")
	path := filepath.Join(home, ".ccpm", "config.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".ccpm", "profiles", name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg["profiles"].(map[string]any)[name] = map[string]any{
		"name": name, "dir": dir, "auth_method": "oauth",
		"created_at": "2026-01-01T00:00:00Z", "last_used": "2026-01-01T00:00:00Z",
	}
	if b, err = json.Marshal(cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// mustCCPM runs the CLI and fails the test on error.
func mustCCPM(t *testing.T, args ...string) {
	t.Helper()
	if r := runCCPM(args...); !r.OK {
		t.Fatalf("ccpm %v: %s", args, r.Error)
	}
}
