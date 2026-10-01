package share

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/filetree"
)

func Dir() (string, error) {
	base, err := config.BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "share"), nil
}

func SkillsDir() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "skills"), nil
}

func AgentsDir() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "agents"), nil
}

func CommandsDir() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "commands"), nil
}

func RulesDir() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "rules"), nil
}

func HooksDir() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "hooks"), nil
}

func MCPDir() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "mcp"), nil
}

func SettingsDir() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "settings"), nil
}

func EnsureDirs() error {
	dirs := []func() (string, error){
		Dir,
		SkillsDir, AgentsDir, CommandsDir, RulesDir, HooksDir,
		MCPDir, SettingsDir,
	}
	for _, fn := range dirs {
		d, err := fn()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(d, config.DirPerm); err != nil {
			return fmt.Errorf("creating share directory %s: %w", d, err)
		}
	}
	return nil
}

// ErrNotLink is returned by Link when dst is a real file or directory that
// ccpm did not create — profile-local content (e.g. a skill Claude Code wrote
// inside the profile). Link never deletes it; callers treat this as
// "profile-local wins" (AGENTS.md invariant 11) and skip the entry.
var ErrNotLink = errors.New("destination exists and is not a ccpm link")

// Link creates a symlink from dst pointing to src. If symlinks are not
// available (Windows without Developer Mode / admin), it falls back to a
// recursive copy and emits a one-time warning so the user knows
// deduplication is degraded. A real file/dir at dst is refused with
// ErrNotLink unless it is a copy-fallback Link itself recorded.
func Link(src, dst string) error {
	if fi, err := os.Lstat(dst); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			if target, terr := os.Readlink(dst); terr == nil {
				if target == src {
					return nil
				}
				absSrc, _ := filepath.Abs(src)
				absTarget, _ := filepath.Abs(target)
				if absSrc == absTarget {
					return nil
				}
			}
			// Wrong-target symlink: leave it in place — the rename below
			// replaces it atomically, so concurrent profile launches never
			// observe a missing dst.
		} else if !isCopyFallback(dst) {
			return fmt.Errorf("%w: %s", ErrNotLink, dst)
		} else {
			// A Windows copy-fallback Link recorded earlier. This one case is
			// inherently non-atomic: a directory cannot be replaced by
			// rename, so it must be removed before linking.
			if err := os.RemoveAll(dst); err != nil {
				return fmt.Errorf("removing existing path at %s: %w", dst, err)
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(dst), config.DirPerm); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	// Try a real symlink first. On Unix this always works; on Windows it
	// works when Developer Mode is on or the process is elevated. The link is
	// created under a temp name and renamed over dst (atomic replace).
	tmp := fmt.Sprintf("%s.ccpm-link-%d.tmp", dst, os.Getpid())
	_ = os.Remove(tmp)
	if err := os.Symlink(src, tmp); err == nil {
		if err := os.Rename(tmp, dst); err != nil {
			// Windows cannot rename over some existing entries; degrade to
			// remove+rename rather than failing the cascade.
			_ = os.RemoveAll(dst)
			if err := os.Rename(tmp, dst); err != nil {
				_ = os.Remove(tmp)
				return fmt.Errorf("activating symlink at %s: %w", dst, err)
			}
		}
		return nil
	} else if runtime.GOOS != "windows" || !isPrivilegeError(err) {
		return err
	}

	// Windows fallback: copy the tree and leave a breadcrumb so ccpm doctor
	// / the next `ccpm sync` can warn about degraded dedup. The breadcrumb
	// also records dst so the next Link may refresh this copy; if recording
	// fails the copy is merely never refreshed (treated as profile-local).
	// dst here is absent, a symlink, or a recorded copy — real profile-local
	// entries were refused above.
	emitWindowsCopyFallbackWarning()
	if err := os.RemoveAll(dst); err != nil {
		return fmt.Errorf("removing existing path at %s: %w", dst, err)
	}
	_ = recordCopyFallback(dst)
	return filetree.CopyTree(src, dst, false)
}

// isPrivilegeError recognizes the Windows error that is returned when the
// current user cannot create symlinks (the default state without Developer
// Mode).
func isPrivilegeError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "A required privilege is not held by the client") {
		return true
	}
	if strings.Contains(msg, "ERROR_PRIVILEGE_NOT_HELD") {
		return true
	}
	// os.LinkError wraps the underlying errno; we can also match on "not
	// supported" for old FAT32-mounted volumes.
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) && strings.Contains(linkErr.Err.Error(), "privilege") {
		return true
	}
	return false
}

var copyFallbackWarningOnce sync.Once

func emitWindowsCopyFallbackWarning() {
	copyFallbackWarningOnce.Do(func() {
		fmt.Fprintln(os.Stderr, "Warning: symlinks unavailable on this Windows system — ccpm is falling back to copying shared assets.")
		fmt.Fprintln(os.Stderr, "         Turn on Developer Mode (Settings → For developers → Developer Mode) for real deduplication.")
	})
}

func copyFallbackMarker() (string, error) {
	base, err := config.BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, ".windows-copy-fallback"), nil
}

// recordCopyFallback writes the breadcrumb (header line on first use) and
// appends dst's absolute path, one per line.
func recordCopyFallback(dst string) error {
	marker, err := copyFallbackMarker()
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	if isCopyFallback(abs) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(marker), config.DirPerm); err != nil {
		return err
	}
	line := abs + "\n"
	if _, err := os.Stat(marker); os.IsNotExist(err) {
		line = "ccpm fell back to copies because the Windows user cannot create symlinks.\n" + line
	}
	f, err := os.OpenFile(marker, os.O_APPEND|os.O_CREATE|os.O_WRONLY, config.FilePerm)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(line); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// isCopyFallback reports whether dst is a copy Link made in place of a
// symlink (recorded by recordCopyFallback), and so safe to replace.
func isCopyFallback(dst string) bool {
	marker, err := copyFallbackMarker()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(dst)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == abs {
			return true
		}
	}
	return false
}

// Unlink removes a symlink (or on Windows, the copied directory).
func Unlink(dst string) error {
	info, err := os.Lstat(dst)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return os.Remove(dst)
	}
	return os.RemoveAll(dst)
}

// IsLinked checks whether dst is a link or copy pointing to src. It handles
// both real symlinks and Windows copy-fallbacks by resolving with
// filepath.EvalSymlinks and comparing absolute paths.
func IsLinked(src, dst string) bool {
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return false
	}
	resolvedSrc, err := filepath.EvalSymlinks(absSrc)
	if err != nil {
		resolvedSrc = absSrc
	}

	info, err := os.Lstat(dst)
	if err != nil {
		return false
	}
	// Fast path: real symlink.
	if info.Mode()&os.ModeSymlink != 0 {
		resolvedDst, err := filepath.EvalSymlinks(dst)
		if err != nil {
			return false
		}
		return resolvedDst == resolvedSrc
	}
	// Copy fallback: dst exists and matches src if its resolved path IS
	// src (only possible if the FS supports symlinks after all), otherwise
	// we can't know without hashing. Treat as "linked enough" if same path.
	resolvedDst, err := filepath.EvalSymlinks(dst)
	if err != nil {
		return false
	}
	return resolvedDst == resolvedSrc
}
