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
