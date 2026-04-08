package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/nitin-1926/ccpm/internal/config"
)

var nameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

const maxNameLen = 32

func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("profile name cannot be empty")
	}
	if len(name) > maxNameLen {
		return fmt.Errorf("profile name too long (max %d characters)", maxNameLen)
	}
	if !nameRegex.MatchString(name) {
		return fmt.Errorf("profile name must start with alphanumeric and contain only alphanumeric, hyphens, or underscores")
	}
	return nil
}

func GetDir(name string) (string, error) {
	profilesDir, err := config.ProfilesDir()
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(filepath.Join(profilesDir, name))
	if err != nil {
		return "", fmt.Errorf("resolving profile path: %w", err)
	}
	return abs, nil
}

func Exists(name string) (bool, error) {
	dir, err := GetDir(name)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

func Create(name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}

	exists, err := Exists(name)
	if err != nil {
		return "", err
	}
	if exists {
		return "", fmt.Errorf("profile %q already exists", name)
	}

	dir, err := GetDir(name)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating profile directory: %w", err)
	}

	return dir, nil
}

func Remove(name string) error {
	dir, err := GetDir(name)
	if err != nil {
		return err
	}

	exists, err := Exists(name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("profile %q not found", name)
	}

	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("removing profile directory: %w", err)
	}
	return nil
}
