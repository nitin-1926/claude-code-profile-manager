// Package atomicwrite applies a batch of file changes (writes and deletes)
// transactionally. Either all of the changes are applied to disk, or none of
// them are: any failure mid-transaction restores every target to its
// pre-transaction state.
//
// The implementation uses snapshot/stage/commit/rollback:
//
//  1. Snapshot every target path's current state (contents + mode, or
//     "absent" if the path doesn't exist).
//  2. Stage each Write to a sibling temp file (<path>.ccpm-staged-<rand>).
//  3. Commit by atomic-renaming each staged file over its target, or
//     removing the target for Delete operations.
//  4. On any error during commit, restore each already-committed target
//     from its snapshot. Remove any not-yet-committed staged files.
//
// Safety properties enforced:
//
//   - Symlink targets are refused; only regular files (or absent paths) can
//     be written or deleted. This prevents a transaction from clobbering a
//     file outside ~/.ccpm via an attacker-controlled symlink.
//   - Non-regular targets (directories, devices, sockets) are refused.
//   - Targets are deduplicated; the same path can't appear twice in one
//     transaction.
//   - Atomic rename uses os.Rename, which is atomic when source and target
//     are on the same filesystem. ccpm only writes inside ~/.ccpm and
//     ~/.claude (always the same FS in practice).
//
// On Windows, os.Rename over an existing file works on modern releases. The
// atomic-rename guarantee may be weaker on very old Windows versions; ccpm
// targets aren't shared across processes so the practical risk is low.
package atomicwrite

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DefaultMode is used when a Write change has Mode == 0. Mirrors
// config.FilePerm so ccpm-owned files stay 0600.
const DefaultMode os.FileMode = 0o600

// Kind identifies the type of file change.
type Kind int

const (
	// Write replaces (or creates) the target as a regular file with Data.
	// Refuses to overwrite an existing symlink (security property: prevents
	// a transaction from clobbering data outside ~/.ccpm via an attacker-
	// controlled symlink).
	Write Kind = iota
	// Delete removes whatever is at the target path (regular file or
	// symlink). Absent path is a no-op.
	Delete
	// Symlink creates (or replaces) a symbolic link at Path pointing to
	// LinkTarget. Tolerates an existing symlink or regular file at Path —
	// snapshots the prior state so rollback can restore it.
	Symlink
)

// FileChange is one entry in an atomic transaction.
type FileChange struct {
	Path       string
	Kind       Kind
	Data       []byte      // Write only
	Mode       os.FileMode // Write only; defaults to DefaultMode if zero
	LinkTarget string      // Symlink only — what the symlink points to
}

// WriteFile is a convenience constructor for a Write change.
func WriteFile(path string, data []byte, mode os.FileMode) FileChange {
	return FileChange{Path: path, Kind: Write, Data: data, Mode: mode}
}

// DeleteFile is a convenience constructor for a Delete change.
func DeleteFile(path string) FileChange {
	return FileChange{Path: path, Kind: Delete}
}

// SymlinkAt is a convenience constructor for a Symlink change.
func SymlinkAt(path, target string) FileChange {
	return FileChange{Path: path, Kind: Symlink, LinkTarget: target}
}

type snapshotKind int

const (
	snapAbsent snapshotKind = iota
	snapFile
	snapSymlink
)

type snapshot struct {
	kind   snapshotKind
	data   []byte      // snapFile
	mode   os.FileMode // snapFile
	target string      // snapSymlink
}

// Apply executes every change atomically. If any step fails, every target is
// restored to its pre-call state and the original error is returned.
func Apply(changes []FileChange) error {
	if len(changes) == 0 {
		return nil
	}

	if err := validate(changes); err != nil {
		return err
	}

	snapshots := make([]snapshot, len(changes))
	for i, c := range changes {
		s, err := snapshotPath(c)
		if err != nil {
			return err
		}
		snapshots[i] = s
	}

	staged := make([]string, len(changes))
	for i, c := range changes {
		if c.Kind != Write {
			continue
		}
		path, err := stageWrite(c)
		if err != nil {
			cleanupStaged(staged)
			return err
		}
		staged[i] = path
	}

	committed := make([]int, 0, len(changes))
	for i, c := range changes {
		switch c.Kind {
		case Write:
			if err := os.Rename(staged[i], c.Path); err != nil {
				rollback(changes, snapshots, staged, committed)
				return fmt.Errorf("atomicwrite: rename staged %q -> %q: %w", staged[i], c.Path, err)
			}
		case Delete:
			if snapshots[i].kind == snapAbsent {
				committed = append(committed, i)
				continue
			}
			if err := os.Remove(c.Path); err != nil {
				rollback(changes, snapshots, staged, committed)
				return fmt.Errorf("atomicwrite: remove %q: %w", c.Path, err)
			}
		case Symlink:
			if err := commitSymlink(c, snapshots[i]); err != nil {
				rollback(changes, snapshots, staged, committed)
				return err
			}
		}
		committed = append(committed, i)
	}

	return nil
}

func commitSymlink(c FileChange, s snapshot) error {
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o700); err != nil {
		return fmt.Errorf("atomicwrite: mkdir parent of %q: %w", c.Path, err)
	}
	if s.kind != snapAbsent {
		if err := os.Remove(c.Path); err != nil {
			return fmt.Errorf("atomicwrite: clearing %q before symlink: %w", c.Path, err)
		}
	}
	if err := os.Symlink(c.LinkTarget, c.Path); err != nil {
		return fmt.Errorf("atomicwrite: symlink %q -> %q: %w", c.Path, c.LinkTarget, err)
	}
	return nil
}

func validate(changes []FileChange) error {
	seen := make(map[string]bool, len(changes))
	for _, c := range changes {
		if c.Path == "" {
			return errors.New("atomicwrite: empty path")
		}
		if !filepath.IsAbs(c.Path) {
			return fmt.Errorf("atomicwrite: path must be absolute, got %q", c.Path)
		}
		if seen[c.Path] {
			return fmt.Errorf("atomicwrite: duplicate target path %q", c.Path)
		}
		seen[c.Path] = true
		switch c.Kind {
		case Write, Delete:
		case Symlink:
			if c.LinkTarget == "" {
				return fmt.Errorf("atomicwrite: empty LinkTarget for symlink at %q", c.Path)
			}
		default:
			return fmt.Errorf("atomicwrite: unknown change kind %d for %q", c.Kind, c.Path)
		}
	}
	return nil
}

// snapshotPath records the current state of c.Path. Refusal rules depend on
// the change kind:
//
//   - Write refuses to overwrite an existing symlink (security: prevents a
//     transaction from following an attacker-controlled link).
//   - Delete and Symlink tolerate either a regular file or a symlink at the
//     path, since both kinds rewrite the target by design.
//
// Non-regular non-symlink targets (directories, devices) are always refused.
func snapshotPath(c FileChange) (snapshot, error) {
	info, err := os.Lstat(c.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return snapshot{kind: snapAbsent}, nil
	}
	if err != nil {
		return snapshot{}, fmt.Errorf("atomicwrite: stat %q: %w", c.Path, err)
	}
	mode := info.Mode()
	if mode&os.ModeSymlink != 0 {
		if c.Kind == Write {
			return snapshot{}, fmt.Errorf("atomicwrite: refusing to overwrite symlink %q with regular-file write", c.Path)
		}
		target, err := os.Readlink(c.Path)
		if err != nil {
			return snapshot{}, fmt.Errorf("atomicwrite: readlink %q: %w", c.Path, err)
		}
		return snapshot{kind: snapSymlink, target: target}, nil
	}
	if !mode.IsRegular() {
		return snapshot{}, fmt.Errorf("atomicwrite: refusing non-regular file %q (mode %s)", c.Path, mode)
	}
	data, err := os.ReadFile(c.Path)
	if err != nil {
		return snapshot{}, fmt.Errorf("atomicwrite: read %q: %w", c.Path, err)
	}
	return snapshot{kind: snapFile, data: data, mode: mode.Perm()}, nil
}

// stageWrite writes c.Data to a sibling temp file and returns its path.
// Creates parent directories with 0700 permissions if missing.
func stageWrite(c FileChange) (string, error) {
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o700); err != nil {
		return "", fmt.Errorf("atomicwrite: mkdir parent of %q: %w", c.Path, err)
	}
	suffix, err := randomSuffix()
	if err != nil {
		return "", err
	}
	staged := c.Path + ".ccpm-staged-" + suffix
	mode := c.Mode
	if mode == 0 {
		mode = DefaultMode
	}
	if err := os.WriteFile(staged, c.Data, mode); err != nil {
		return "", fmt.Errorf("atomicwrite: write staged %q: %w", staged, err)
	}
	return staged, nil
}

func randomSuffix() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("atomicwrite: rand: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func cleanupStaged(staged []string) {
	for _, p := range staged {
		if p == "" {
			continue
		}
		_ = os.Remove(p)
	}
}

func rollback(changes []FileChange, snapshots []snapshot, staged []string, committed []int) {
	committedSet := make(map[int]bool, len(committed))
	for _, i := range committed {
		committedSet[i] = true
	}

	for _, i := range committed {
		c := changes[i]
		s := snapshots[i]
		switch s.kind {
		case snapFile:
			tmp := c.Path + ".ccpm-rollback"
			if err := os.WriteFile(tmp, s.data, s.mode); err == nil {
				_ = os.Rename(tmp, c.Path)
			}
		case snapSymlink:
			_ = os.Remove(c.Path)
			_ = os.Symlink(s.target, c.Path)
		case snapAbsent:
			_ = os.Remove(c.Path)
		}
	}

	for i, p := range staged {
		if p == "" || committedSet[i] {
			continue
		}
		_ = os.Remove(p)
	}
}
