// Package filetree provides small filesystem helpers used across ccpm.
package filetree

import (
	"os"
	"path/filepath"
)

// CopyTree walks src and copies regular files into dst, preserving relative paths.
// Directories are created as needed.
//
// If skipExisting is true, an existing regular file at the destination path is left
// unchanged (merge / preserve behavior).
//
// filepath.Walk uses Lstat and does not follow symlinks. A symlink whose target is a
// directory would otherwise be misclassified as a file and fail on ReadFile with
// EISDIR; those are handled by recursively copying from the resolved directory.
func CopyTree(src, dst string, skipExisting bool) error {
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
			fi, err := os.Stat(path)
			if err != nil {
				return err
			}
			if fi.IsDir() {
				real, err := filepath.EvalSymlinks(path)
				if err != nil {
					return err
				}
				if err := os.MkdirAll(target, fi.Mode()); err != nil {
					return err
				}
				return CopyTree(real, target, skipExisting)
			}
		}

		if skipExisting {
			if _, err := os.Stat(target); err == nil {
				return nil
			}
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
