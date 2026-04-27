package filetree

import (
	"fmt"
	"os"
	"path/filepath"
)

// SeedStoreEntry populates dst from src using one of two strategies:
//
//   - When live is true and src is itself a symlink-to-directory, dst is
//     created as a symlink pointing at the resolved absolute path of src.
//     Edits to the original tree are then visible through dst without a
//     re-copy.
//   - Otherwise, dst is populated by a file-level copy via CopyTree. Any
//     pre-existing dst is removed before seeding so the operation is
//     idempotent.
//
// Returns true when the live-symlink branch was taken.
func SeedStoreEntry(src, dst string, live bool) (bool, error) {
	if err := os.RemoveAll(dst); err != nil {
		return false, fmt.Errorf("clear %s: %w", dst, err)
	}

	if live {
		isLinkDir, err := SymlinkToDirectory(src)
		if err != nil {
			return false, fmt.Errorf("stat %s: %w", src, err)
		}
		if isLinkDir {
			resolved, err := filepath.EvalSymlinks(src)
			if err != nil {
				return false, fmt.Errorf("eval symlinks %s: %w", src, err)
			}
			abs, err := filepath.Abs(resolved)
			if err != nil {
				return false, fmt.Errorf("abs %s: %w", resolved, err)
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				return false, err
			}
			if err := os.Symlink(abs, dst); err != nil {
				return false, fmt.Errorf("symlink %s -> %s: %w", dst, abs, err)
			}
			return true, nil
		}
	}

	info, err := os.Stat(src)
	if err != nil {
		return false, err
	}
	if info.IsDir() {
		return false, CopyTree(src, dst, false)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return false, err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return false, err
	}
	return false, os.WriteFile(dst, data, info.Mode())
}
