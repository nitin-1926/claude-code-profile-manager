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
func findCCPM() string {
	if p, err := exec.LookPath("ccpm"); err == nil {
		return p
	}
	// bundled alongside the app executable (see M6 packaging)
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, c := range []string{
			filepath.Join(dir, "ccpm"),
			filepath.Join(dir, "..", "Resources", "ccpm"),
		} {
			if isExec(c) {
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
		if isExec(c) {
			return c
		}
	}
	return ""
}

func isExec(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode()&0111 != 0
}
