// Package filetree provides small filesystem helpers used across ccpm.
package filetree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
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
	return copyTree(src, dst, skipExisting, false)
}

// CopyTreeSkipEscaping behaves like CopyTree but, instead of erroring on a
// symlink whose target escapes src, it silently SKIPS that entry. This is for
// callers that copy a ccpm *profile* directory (e.g. `ccpm clone`): a profile's
// shared/host-cascaded assets are symlinks pointing into ~/.ccpm/share or
// ~/.claude — outside the profile — and they are re-linked automatically by the
// cascade / ApplyGlobals on the copy, so there is no reason to fail on them.
// (The strict-refusal CopyTree is still used for `import default`, where src is
// the user-writable ~/.claude and an escaping symlink is a genuine exfil risk.)
func CopyTreeSkipEscaping(src, dst string, skipExisting bool) error {
	return copyTree(src, dst, skipExisting, true)
}

// maxCopyDepth bounds how many symlink-to-directory hops copyTree will
// follow. Legitimate trees nest a handful of links at most; anything deeper
// is a loop or a deliberately hostile layout.
const maxCopyDepth = 40

func copyTree(src, dst string, skipExisting, skipEscaping bool) error {
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return fmt.Errorf("resolving src %q: %w", src, err)
	}
	absSrc, err = filepath.EvalSymlinks(absSrc)
	if err != nil {
		return fmt.Errorf("evaluating src symlinks: %w", err)
	}
	// inChain tracks the resolved directories on the CURRENT recursion path
	// so a symlink pointing at its own ancestor (skills/x → skills) cannot
	// recurse forever. It is a stack, not a global set: two sibling links
	// legitimately pointing at the same target are not a cycle.
	inChain := map[string]bool{absSrc: true}
	return copyTreeFrom(absSrc, dst, absSrc, skipExisting, skipEscaping, inChain, 0)
}

// copyTreeFrom walks src (already symlink-resolved) and copies into dst.
// root is the ORIGINAL copy root: every followed symlink is checked against
// it, not against the subtree being recursed into — re-deriving the root from
// a resolved target would let nested links walk back out of the tree.
func copyTreeFrom(src, dst, root string, skipExisting, skipEscaping bool, inChain map[string]bool, depth int) error {
	if depth > maxCopyDepth {
		return fmt.Errorf("copy tree: symlink nesting exceeds %d levels under %q", maxCopyDepth, root)
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

		readPath := path
		mode := info.Mode()
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				if skipEscaping {
					return nil // dangling/unresolvable link in a profile — skip, don't fail the copy
				}
				return err
			}
			fi, err := os.Stat(resolved)
			if err != nil {
				return err
			}
			// Symlinks whose resolved target escapes the original copy root are
			// a classic exfil/DoS primitive when src is user-writable
			// (e.g. ~/.claude/skills/). Strict callers refuse; profile-copy
			// callers skip (the target re-links via cascade/ApplyGlobals).
			if !isWithin(root, resolved) {
				if skipEscaping {
					return nil
				}
				return fmt.Errorf("refusing to follow symlink %q: target %q lies outside %q", path, resolved, root)
			}
			if fi.IsDir() {
				if inChain[resolved] {
					if skipEscaping {
						return nil
					}
					return fmt.Errorf("refusing to follow symlink %q: cycle back to %q", path, resolved)
				}
				if err := os.MkdirAll(target, fi.Mode()); err != nil {
					return err
				}
				inChain[resolved] = true
				recErr := copyTreeFrom(resolved, target, root, skipExisting, skipEscaping, inChain, depth+1)
				delete(inChain, resolved)
				return recErr
			}
			// Symlink to a regular file: read through the verified resolved
			// path, not the link, so the content cannot be re-pointed between
			// the check and the read.
			readPath = resolved
			mode = fi.Mode()
		} else {
			// Plain regular file at walk time. Re-Lstat immediately before
			// reading: ReadFile follows symlinks, so an entry swapped for a
			// link after the walk statted it would otherwise read through it.
			fi, err := os.Lstat(path)
			if err != nil {
				return err
			}
			if !fi.Mode().IsRegular() {
				return fmt.Errorf("refusing to copy %q: changed type during copy", path)
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
		data, err := os.ReadFile(readPath)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, mode)
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
