package usage

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// EncodeCwd mirrors native Claude Code's cwd encoding for the directory layout
// <profileDir>/projects/<encoded-cwd>/: EVERY non-alphanumeric character becomes
// its own "-". Nothing is collapsed and nothing is trimmed, so a leading "/" is
// a leading "-" and "/." becomes "--".
//
// This used to collapse runs of non-alphanumerics and trim the ends, which
// matched no real directory: "/Users/x/.claude-brain" encoded to
// "Users-x-claude-brain" where Claude Code writes "-Users-x--claude-brain".
// Measured against a real profile, 0 of 11 directories matched. Every caller
// that used the result as a filesystem lookup silently found nothing — the
// onlyEncodedSubdir filter below, and with it the cwd-scoped default of
// `ccpm sessions list`, which returned "no sessions" unless given --all.
// One further subtlety: Claude Code's encoder is JavaScript, so it replaces
// per UTF-16 CODE UNIT, not per rune. A non-BMP character — an emoji in a
// directory name — is a surrogate pair there and yields TWO dashes, where a Go
// `for range` sees one rune and yields one. Verified against node:
// "/Users/x/<rocket>proj" encodes to "-Users-x---proj", three dashes, not two.
// Getting this wrong is the same failure the collapse-and-trim bug above had —
// a lookup that silently finds nothing.
func EncodeCwd(cwd string) string {
	var b strings.Builder
	b.Grow(len(cwd))
	for _, r := range cwd {
		if isAlnum(r) {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
		if r > 0xFFFF {
			b.WriteByte('-')
		}
	}
	return b.String()
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
