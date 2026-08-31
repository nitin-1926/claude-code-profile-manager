//go:build darwin

package services

import (
	"os"
	"path/filepath"
	"testing"
)

// The desktop bundle ships its GUI binary as Contents/MacOS/CCPM. macOS
// filesystems are case-insensitive by default, so a lookup for "ccpm" in that
// same directory stats the GUI binary itself. findCCPM used to hand that back,
// and every `ccpm ...` shell-out then relaunched the app — each new instance
// shelling out again on load, multiplying until the machine died.
func TestUsableCCPMRejectsOwnExecutable(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "CCPM")
	if err := os.WriteFile(self, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(dir, "linked-ccpm")
	if err := os.Symlink(self, link); err != nil {
		t.Fatal(err)
	}

	for _, candidate := range []string{
		self,                       // the exact same path
		filepath.Join(dir, "ccpm"), // the case-insensitive spelling that caused the bomb
		link,                       // reached through a symlink
	} {
		if usableCCPM(candidate, self) {
			t.Errorf("usableCCPM(%q, self) = true, want false — this relaunches the GUI instead of the CLI", candidate)
		}
	}
}

func TestUsableCCPMAcceptsRealCLI(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "CCPM")
	cli := filepath.Join(dir, "bin-ccpm")
	for _, p := range []string{self, cli} {
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if !usableCCPM(cli, self) {
		t.Error("usableCCPM rejected a genuine, distinct ccpm binary")
	}
}

func TestUsableCCPMRejectsNonExecutable(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "ccpm")
	if err := os.WriteFile(plain, []byte("not executable"), 0o644); err != nil {
		t.Fatal(err)
	}
	if usableCCPM(plain, filepath.Join(dir, "CCPM")) {
		t.Error("usableCCPM accepted a non-executable file")
	}
	if usableCCPM(filepath.Join(dir, "missing"), "") {
		t.Error("usableCCPM accepted a missing file")
	}
}

// End-to-end guard on the real failure shape. Compile this test binary as
// `CCPM` into a directory (see the build-as-CCPM verification in CI notes) and
// findCCPM's `<dir>/ccpm` probe stats it on a case-insensitive filesystem —
// exactly what happens inside Contents/MacOS. It must never come back as the
// CLI, whatever the binary is called.
func TestFindCCPMNeverReturnsOwnExecutable(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("HOME", t.TempDir())

	self, err := os.Executable()
	if err != nil {
		t.Skipf("cannot resolve own executable: %v", err)
	}
	got := findCCPM()
	if got == "" {
		return // nothing found at all is the safe answer
	}
	a, err1 := os.Stat(got)
	b, err2 := os.Stat(self)
	if err1 == nil && err2 == nil && os.SameFile(a, b) {
		t.Fatalf("findCCPM returned our own executable (%q); shelling out to it relaunches the GUI", got)
	}
}
