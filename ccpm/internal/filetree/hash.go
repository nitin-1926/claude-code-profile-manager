package filetree

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrTooLarge is returned by ContentHash when a tree exceeds its budget.
var ErrTooLarge = errors.New("too large to hash")

// ContentHash returns a sha256 over what path holds, following path itself if
// it is a symlink. A file hashes its bytes; a directory hashes every entry's
// relative path, kind and content in lexical order, so two trees hash equal
// exactly when they hold the same names and bytes. Symlinks inside a
// directory are hashed by their target string and never followed, so nothing
// outside the tree is read. More than maxBytes of file content or maxEntries
// entries returns ErrTooLarge.
func ContentHash(path string, maxBytes int64, maxEntries int) (string, error) {
	root, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if maxEntries--; maxEntries < 0 {
			return ErrTooLarge
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "l %s\x00%s\x00", rel, target)
		case d.IsDir():
			fmt.Fprintf(h, "d %s\x00", rel)
		case d.Type().IsRegular():
			info, err := d.Info()
			if err != nil {
				return err
			}
			if maxBytes -= info.Size(); maxBytes < 0 {
				return ErrTooLarge
			}
			fmt.Fprintf(h, "f %s\x00%d\x00", rel, info.Size())
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			_, err = io.Copy(h, io.LimitReader(f, info.Size()))
			f.Close()
			return err
		default: // devices, pipes, sockets: named, never opened
			fmt.Fprintf(h, "o %s\x00", rel)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
