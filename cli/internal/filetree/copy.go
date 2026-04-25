// Package filetree provides small filesystem helpers used across ccpm.
package filetree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nitin-1926/ccpm/internal/config"
)

// CopyTree walks src and copies regular files into dst, preserving relative paths.
// Directories are created as needed.
//
// If skipExisting is true, an existing regular file at the destination path is left
// unchanged (merge / preserve behavior).
//
// Symlink handling: filepath.Walk uses Lstat and does not follow symlinks. A
// symlink-to-directory is followed only when its resolved target stays inside
// the original src root (F10 — otherwise a rogue symlink in ~/.claude/skills/<x>
// pointing at ~/.ssh or /etc would let `ccpm import default` copy unrelated
// files into the shared store).
func CopyTree(src, dst string, skipExisting bool) error {
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return fmt.Errorf("resolving src %q: %w", src, err)
	}
	absSrc, err = filepath.EvalSymlinks(absSrc)
	if err != nil {
		return fmt.Errorf("evaluating src symlinks: %w", err)
	}

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				return err
			}
			fi, err := os.Stat(resolved)
			if err != nil {
				return err
			}
			// Refuse to follow symlinks whose resolved target escapes the
			// original src root — those are a classic exfil/DoS primitive
			// when src is user-writable (e.g. ~/.claude/skills/).
			if !isWithin(absSrc, resolved) {
				return fmt.Errorf("refusing to follow symlink %q: target %q lies outside %q", path, resolved, absSrc)
			}
			if fi.IsDir() {
				if err := os.MkdirAll(target, fi.Mode()); err != nil {
					return err
				}
				return CopyTree(resolved, target, skipExisting)
			}
		}

		if skipExisting {
			if _, err := os.Stat(target); err == nil {
				return nil
			}
		}

		if err := os.MkdirAll(filepath.Dir(target), config.DirPerm); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

// isWithin reports whether candidate lies inside (or equals) root. Both paths
// are expected to be absolute and symlink-resolved. A trailing separator
// guard prevents "/foo" from matching "/foobar" via plain string prefix.
func isWithin(root, candidate string) bool {
	if root == candidate {
		return true
	}
	rootWithSep := strings.TrimRight(root, string(filepath.Separator)) + string(filepath.Separator)
	return strings.HasPrefix(candidate, rootWithSep)
}
