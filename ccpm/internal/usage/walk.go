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

// WalkTranscripts invokes fn(absPath, relPath) for each *.jsonl file under
// <profileDir>/projects, where relPath is relative to that projects root. When
// onlyEncodedSubdir is non-empty, only that project subdir is scanned (matching
// how `claude --resume` scopes to the current cwd). A missing projects/ dir is
// not an error — fn is simply never called.
func WalkTranscripts(profileDir, onlyEncodedSubdir string, fn func(abs, rel string) error) error {
	root := filepath.Join(profileDir, "projects")
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			// A transient error on one entry (e.g. native claude mid-write)
			// shouldn't abort the whole walk.
			return nil
		}
		if d.IsDir() {
			if onlyEncodedSubdir != "" {
				rel, _ := filepath.Rel(root, path)
				if rel != "." && rel != onlyEncodedSubdir && !strings.HasPrefix(rel, onlyEncodedSubdir+string(filepath.Separator)) {
					return fs.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		return fn(path, rel)
	})
}
