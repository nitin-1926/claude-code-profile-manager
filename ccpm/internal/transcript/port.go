package transcript

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

// PortResult is what a port did and where the session should resume.
type PortResult struct {
	SessionID string
	// Cwd is the session's first recorded directory: a session's cwd drifts, and
	// its first one is where its tools and project context belong. Empty when
	// the transcript records none.
	Cwd string
	// CwdMissing reports that Cwd is gone. Claude Code does not validate it (see
	// Port), so this informs the caller rather than failing the port.
	CwdMissing bool
	// AlreadyPorted means the target already held identical bytes.
	AlreadyPorted bool
}

// Port copies a session from one profile to another, under the same encoded-cwd
// directory, so `claude --resume <id>` finds it there. It copies, never moves or
// links: two profiles appending to one file would corrupt it.
//
// What travels was measured against Claude Code 2.1.318 in a sandbox:
//
//   - The .jsonl alone resumes with the whole history re-sent. The target's
//     .claude.json needs no entry, and the recorded cwd is not validated.
//   - <id>/subagents/ travels: without it a resumed conversation that messages an
//     earlier subagent fails with "No transcript found for agent ID", and the
//     History reader could not open those transcripts in the target profile.
//   - <id>/tool-results/ stays: a transcript names those files by absolute path
//     in the SOURCE profile, so a copy here would never be read. The re-sent
//     conversation was byte-identical with and without it.
//
// usage.Adopt writes the copy, so tokens already spent stay with the source.
func Port(from, to Scope, sessionID string) (PortResult, error) {
	if from.Dir == to.Dir {
		return PortResult{}, fmt.Errorf("session %s is already in profile %q", sessionID, to.Profile)
	}
	ix, _ := BuildIndex(from.Dir) // a build error still yields a usable index
	e := ix.Entries[sessionID]
	if e == nil || sessionIDFromPath(e.RelPath) != sessionID {
		return PortResult{}, fmt.Errorf("profile %q has no session %s", from.Profile, sessionID)
	}

	res := PortResult{SessionID: sessionID, Cwd: e.Cwd}
	if e.Cwd != "" {
		fi, err := os.Stat(e.Cwd)
		res.CwdMissing = err != nil || !fi.IsDir()
	}

	data, err := readPortSource(from.Dir, e.RelPath)
	if err != nil {
		return PortResult{}, err
	}
	if err := checkPortTarget(to.Dir, e.RelPath); err != nil {
		return PortResult{}, fmt.Errorf("refusing to write into profile %q: %w", to.Profile, err)
	}

	target := filepath.Join(to.Dir, "projects", filepath.FromSlash(e.RelPath))
	fi, err := os.Lstat(target)
	switch {
	case err == nil:
		if !fi.Mode().IsRegular() {
			return PortResult{}, fmt.Errorf("refusing to write into profile %q: its copy of session %s is not a regular file", to.Profile, sessionID)
		}
		have, err := os.ReadFile(target)
		if err != nil {
			return PortResult{}, err
		}
		if !bytes.Equal(have, data) {
			return PortResult{}, fmt.Errorf("profile %q already has its own copy of session %s with different content, so nothing was changed. The copies have continued separately: resume that copy in %s instead", to.Profile, sessionID, to.Profile)
		}
		res.AlreadyPorted = true
	case errors.Is(err, fs.ErrNotExist):
		files, err := readSubagents(from.Dir, e.RelPath)
		if err != nil {
			return PortResult{}, err
		}
		for rel := range files {
			if err := checkPortTarget(to.Dir, rel); err != nil {
				return PortResult{}, fmt.Errorf("refusing to write into profile %q: %w", to.Profile, err)
			}
		}
		files[e.RelPath] = data
		if err := usage.Adopt(to.Dir, files); err != nil {
			return PortResult{}, fmt.Errorf("copying session into profile %q: %w", to.Profile, err)
		}
	default:
		return PortResult{}, err
	}

	// Resume and the reader consult the sidecar, not the disk, so the session is
	// not there yet until the sidecar knows it.
	if _, err := BuildIndex(to.Dir); err != nil {
		return PortResult{}, fmt.Errorf("indexing session in profile %q: %w", to.Profile, err)
	}
	return res, nil
}

// readPortSource reads a transcript named by a projects-relative path, which came
// out of an on-disk index and is untrusted. ResolvePath proves containment; its
// result has symlinks resolved away, so the path as stored is checked separately.
func readPortSource(dir, rel string) ([]byte, error) {
	full, err := ResolvePath(dir, rel)
	if err != nil {
		return nil, fmt.Errorf("transcript %s is no longer on disk", rel)
	}
	fi, err := os.Lstat(filepath.Join(dir, "projects", filepath.FromSlash(rel)))
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("refusing to copy %s: it is not a regular file", rel)
	}
	return os.ReadFile(full)
}

// readSubagents reads every file under a session's subagents/ directory, keyed by
// projects-relative path. A symlink anywhere in it refuses the whole port, which
// is why this walks rather than reusing subagentTranscripts: that one skips them,
// and it also omits the .meta.json holding each agent's type.
//
// ponytail: holds the session in memory for atomicwrite (largest measured 77 MB,
// briefly). Stage to temp files if a port ever needs to handle far larger ones.
func readSubagents(dir, rel string) (map[string][]byte, error) {
	projects := filepath.Join(dir, "projects")
	root := filepath.Join(projects, filepath.FromSlash(strings.TrimSuffix(rel, ".jsonl")), "subagents")
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			if errors.Is(werr, fs.ErrNotExist) {
				return nil // no subagents
			}
			return werr
		}
		if d.IsDir() {
			return nil
		}
		sub, err := filepath.Rel(projects, p)
		if err != nil {
			return err
		}
		sub = filepath.ToSlash(sub)
		data, err := readPortSource(dir, sub)
		if err != nil {
			return err
		}
		files[sub] = data
		return nil
	})
	return files, err
}

// checkPortTarget proves a path about to be written lands inside the profile's
// projects/. ResolvePath needs the path to exist, so it is asked about the nearest
// ancestor that does: a symlink there would carry the write out of the profile.
func checkPortTarget(dir, rel string) error {
	root := filepath.Join(dir, "projects")
	for p := path.Dir(rel); ; p = path.Dir(p) {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(p))); err == nil {
			if _, err := ResolvePath(dir, p); err != nil {
				return fmt.Errorf("%s does not stay inside its projects directory", p)
			}
			return nil
		}
		if p == "." {
			return nil // no projects/ yet: everything gets created fresh
		}
	}
}
