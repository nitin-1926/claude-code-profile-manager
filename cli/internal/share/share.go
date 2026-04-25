package share

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/filetree"
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
