//go:build darwin

package services

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Install spawns the swap helper and quits. If the helper cannot move the
// bundle the app is gone and never comes back, so a bundle it cannot replace
// must be refused up front, with the reason in the toast.
func TestInstallRefusesABundleItCannotReplace(t *testing.T) {
	if err := checkInstallable("/private/var/folders/xy/T/AppTranslocation/1234-ABCD/d/CCPM.app"); err == nil ||
		!strings.Contains(err.Error(), "Applications") {
		t.Errorf("translocated bundle: err = %v, want a move-to-Applications refusal", err)
	}

	ro := t.TempDir()
	bundle := filepath.Join(ro, "CCPM.app")
	if err := os.Mkdir(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	if err := checkInstallable(bundle); err == nil || !strings.Contains(err.Error(), "write") {
		t.Errorf("read-only parent: err = %v, want a permission refusal", err)
	}

	if err := checkInstallable(filepath.Join(t.TempDir(), "CCPM.app")); err != nil {
		t.Errorf("writable parent: %v", err)
	}
}

// The helper used `set -e`, so a failed mv exited before /usr/bin/open: the
// app had already quit and never relaunched, and its temp dirs were left. It
// must reopen the old bundle and clean up whatever happens to the swap.
func TestSwapHelperRelaunchesWhenTheMoveFails(t *testing.T) {
	root := t.TempDir()
	ro := filepath.Join(root, "ro")
	old := filepath.Join(ro, "CCPM.app")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ro, 0o555); err != nil { // mv "$OLD" "$BAK" fails here
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	newApp := filepath.Join(root, "new", "CCPM.app")
	dl := filepath.Join(root, "dl")
	for _, d := range []string{newApp, dl} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Stand-in for /usr/bin/open: records what it was asked to open. The real
	// one would launch the bundle.
	opened := filepath.Join(root, "opened")
	stub := filepath.Join(root, "open-stub")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\necho \"$1\" > '"+opened+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	shDir := filepath.Join(root, "sh")
	if err := os.Mkdir(shDir, 0o700); err != nil {
		t.Fatal(err)
	}
	sh := filepath.Join(shDir, "swap.sh")
	if err := os.WriteFile(sh, []byte(swapScript), 0o700); err != nil {
		t.Fatal(err)
	}

	// 99999 is above macOS's pid_max, so the wait loop ends at once.
	out, _ := exec.Command("/bin/sh", sh, "99999", old, newApp, dl, stub).CombinedOutput()

	got, err := os.ReadFile(opened)
	if err != nil || strings.TrimSpace(string(got)) != old {
		t.Errorf("old bundle not reopened (opened %q, err %v); helper output: %s", got, err, out)
	}
	if _, err := os.Stat(dl); !os.IsNotExist(err) {
		t.Error("download dir left behind after a failed swap")
	}
	if _, err := os.Stat(old); err != nil {
		t.Errorf("old bundle damaged by a failed swap: %v", err)
	}
}

// http.Client.Timeout covers reading the body, so 60s failed the 4.5MB
// download on any link under ~75KB/s. Deadlines are per request instead.
func TestUpdaterHasNoWholeRequestTimeout(t *testing.T) {
	u := NewUpdater()
	if u.http.Timeout != 0 {
		t.Errorf("http.Client.Timeout = %v, which also bounds the download body", u.http.Timeout)
	}
	tr, ok := u.http.Transport.(*http.Transport)
	if !ok || tr.ResponseHeaderTimeout <= 0 {
		t.Error("no ResponseHeaderTimeout: a server that never answers would hang the check")
	}
}
