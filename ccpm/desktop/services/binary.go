//go:build darwin

package services

import (
	"os"
	"os/exec"
	"path/filepath"
)

// findCCPM locates the ccpm CLI binary. A GUI launched from Finder inherits a
// minimal PATH, so we check PATH first, then a bundled copy next to the app
// executable, then the usual install locations.
//
// Every candidate is rejected if it resolves to our own executable. macOS
// filesystems are case-insensitive by default, so `<bundle>/Contents/MacOS/ccpm`
// stats successfully against the GUI binary `CCPM` sitting right there. Handing
// that back turned each `ccpm ...` shell-out into a fresh app launch, and since
// the UI shells out as soon as it loads, every new instance spawned more —
// an exponential fork bomb that took the whole machine down.
func findCCPM() string {
	self, _ := os.Executable()

	if p, err := exec.LookPath("ccpm"); err == nil && usableCCPM(p, self) {
		return p
	}
	// bundled alongside the app executable (see M6 packaging)
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, c := range []string{
			filepath.Join(dir, "ccpm"),
			filepath.Join(dir, "..", "Resources", "ccpm"),
		} {
			if usableCCPM(c, self) {
				return c
			}
		}
	}
	home, _ := os.UserHomeDir()
	for _, c := range []string{
		"/opt/homebrew/bin/ccpm",
		"/usr/local/bin/ccpm",
		filepath.Join(home, ".npm-global", "bin", "ccpm"),
		filepath.Join(home, ".local", "bin", "ccpm"),
		filepath.Join(home, "go", "bin", "ccpm"),
	} {
		if usableCCPM(c, self) {
			return c
		}
	}
	return ""
}

// usableCCPM reports whether p is an executable file that is not the running
// binary. os.SameFile compares device+inode, so it sees through symlinks, hard
// links and case-insensitive path spellings alike — a plain string compare
// would miss every one of those.
func usableCCPM(p, self string) bool {
	if !isExec(p) {
		return false
	}
	if self == "" {
		return true
	}
	a, err1 := os.Stat(p)
	b, err2 := os.Stat(self)
	if err1 != nil || err2 != nil {
		return true
	}
	return !os.SameFile(a, b)
}

func isExec(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode()&0111 != 0
}
