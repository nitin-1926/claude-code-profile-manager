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

// Link creates a symlink from dst pointing to src. If symlinks are not
// available (Windows without Developer Mode / admin), it falls back to a
// recursive copy and emits a one-time warning so the user knows
// deduplication is degraded.
func Link(src, dst string) error {
	if _, err := os.Lstat(dst); err == nil {
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
		if err := os.RemoveAll(dst); err != nil {
			return fmt.Errorf("removing existing path at %s: %w", dst, err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(dst), config.DirPerm); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	// Try a real symlink first. On Unix this always works; on Windows it
	// works when Developer Mode is on or the process is elevated.
	if err := os.Symlink(src, dst); err == nil {
		return nil
	} else if runtime.GOOS != "windows" || !isPrivilegeError(err) {
		return err
	}

	// Windows fallback: copy the tree and leave a breadcrumb so ccpm doctor
	// / the next `ccpm sync` can warn about degraded dedup. The breadcrumb
	// is non-fatal — a missing file only degrades the friendliness of the
	// next diagnostic message.
	emitWindowsCopyFallbackWarning()
	_ = markWindowsCopyFallback()
	return copyDir(src, dst)
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

func markWindowsCopyFallback() error {
	base, err := config.BaseDir()
	if err != nil {
		return err
	}
	marker := filepath.Join(base, ".windows-copy-fallback")
	if _, err := os.Stat(marker); err == nil {
		return nil
	}
	if err := os.MkdirAll(base, config.DirPerm); err != nil {
		return err
	}
	return os.WriteFile(marker, []byte("ccpm fell back to copies because the Windows user cannot create symlinks.\n"), config.FilePerm)
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

func copyDir(src, dst string) error {
	return filetree.CopyTree(src, dst, false)
}
