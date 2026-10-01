//go:build darwin

package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeCCPM puts a shell script named ccpm first — and alone — on PATH, so
// findCCPM resolves it. Callers point HOME at a scratch dir (syntheticProfile
// does), so nothing here can reach the developer's real ~/.ccpm.
func fakeCCPM(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ccpm"), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

// Every shell-out shares one skeleton, so the outdated-CLI explanation reaches
// the Health tab too — before, only mutations had it and doctor showed a bare
// "exit status 1".
func TestDoctorExplainsAnOutdatedCLI(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fakeCCPM(t, `echo 'Error: unknown command "doctor" for "ccpm"' >&2; exit 1`)
	res, err := NewHealth().Doctor()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Error, "too old") {
		t.Errorf("Error = %q, want the outdated-CLI explanation", res.Error)
	}
}
