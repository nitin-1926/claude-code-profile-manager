package usage

import (
	"bufio"
	"errors"
	"io"
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
		// Regular files only. WalkDir reports a symlinked FILE (it only declines
		// to descend into symlinked dirs), and every caller then opens it, which
		// follows the link: projects/-x/evil.jsonl -> /dev/zero is an endless
		// read, -> ~/.ssh/id_rsa is a disclosure. A FIFO or device blocks or
		// never ends the same way. The profile dir can be shared or restored
		// from elsewhere, so nothing it contains is trusted to be a transcript.
		if d.Type()&fs.ModeType != 0 || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		return fn(path, rel)
	})
}

// MaxLineBytes is the largest JSONL line any transcript reader will hold. The
// longest line measured in a real profile was 1.3 MB (a single tool result), so
// this is generous headroom; a line beyond it is skipped rather than aborting
// the file, because one pathological line must not cost every line after it.
//
// It bounds ALLOCATION, not just decode cost: a transcript truncated or
// concatenated without a trailing newline would otherwise have its entire
// length materialised by ReadBytes before any cap could be consulted — 77 MB on
// the largest file observed, and without limit on a crafted one.
const MaxLineBytes = 8 << 20

// EachLine streams r, handing fn every complete ('\n'-terminated) line together
// with its length in bytes (newline included), until fn returns false. A line
// longer than MaxLineBytes is drained without being kept and handed over as nil
// with its true length, so a caller that tracks byte offsets still advances past
// it. A trailing fragment with no newline is a line still being written and is
// never delivered — decoding half a JSON object would corrupt whatever reads it.
//
// line is only valid until fn returns; it aliases the reader's buffer.
func EachLine(r io.Reader, fn func(line []byte, n int64) bool) error {
	// bufio.Scanner is not usable: it yields the final unterminated line, which
	// is exactly the half-written one this must skip. ReadBytes has the right
	// semantics but no cap, so the line is assembled fragment by fragment.
	br := bufio.NewReaderSize(r, 1<<20)
	var line []byte
	var n int64
	oversize := false
	for {
		frag, err := br.ReadSlice('\n')
		n += int64(len(frag))
		if errors.Is(err, bufio.ErrBufferFull) {
			if oversize || len(line)+len(frag) > MaxLineBytes {
				oversize = true // keep draining, stop accumulating
				line = line[:0]
			} else {
				line = append(line, frag...)
			}
			continue
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil // bytes pending without '\n': still being written
			}
			return err
		}
		// Re-check here too: the fragment holding the newline arrives with
		// err == nil, so a line just over the cap would otherwise be assembled.
		var out []byte
		switch {
		case oversize || len(line)+len(frag) > MaxLineBytes:
			// out stays nil: skipped
		case len(line) == 0:
			out = frag // fast path: whole line already contiguous
		default:
			out = append(line, frag...)
		}
		if !fn(out, n) {
			return nil
		}
		line, n, oversize = line[:0], 0, false
	}
}
