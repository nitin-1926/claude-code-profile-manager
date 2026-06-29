package usage

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// EncodeCwd mirrors native Claude Code's cwd encoding for the directory layout
// <profileDir>/projects/<encoded-cwd>/: every run of non-alphanumeric
// characters collapses to a single "-", with leading/trailing dashes trimmed.
func EncodeCwd(cwd string) string {
	var b strings.Builder
	b.Grow(len(cwd))
	prevDash := false
	for _, r := range cwd {
		if isAlnum(r) {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func isAlnum(r rune) bool {
	switch {
	case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		return true
	}
	return false
}

