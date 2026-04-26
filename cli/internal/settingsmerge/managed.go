package settingsmerge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// managedSettingsDirOverride lets tests inject a fake managed-settings
// directory instead of touching privileged OS paths. Production code keeps it
// as an empty string; tests set it via SetManagedSettingsDirForTest.
var managedSettingsDirOverride string

// SetManagedSettingsDirForTest replaces the managed-settings directory used by
// LoadManagedSettings with the given path. The returned closure restores the
// previous value and should be called via defer.
func SetManagedSettingsDirForTest(dir string) func() {
	prev := managedSettingsDirOverride
	managedSettingsDirOverride = dir
	return func() { managedSettingsDirOverride = prev }
}

// managedSettingsDir returns the directory containing managed-settings.json
// (and its managed-settings.d drop-ins) for the current OS, per native Claude
// Code docs. Returns "" if the OS isn't supported.
func managedSettingsDir() string {
	if managedSettingsDirOverride != "" {
		return managedSettingsDirOverride
	}
	switch runtime.GOOS {
	case "darwin":
		return "/Library/Application Support/ClaudeCode"
	case "linux":
		return "/etc/claude-code"
	case "windows":
		return filepath.Join("C:", "ProgramData", "ClaudeCode")
	default:
		return ""
	}
}

// LoadManagedSettings returns the merged enterprise/managed settings layer.
//
// Reads, in order:
//  1. <managedDir>/managed-settings.json (if present).
//  2. Every *.json file under <managedDir>/managed-settings.d/ in
//     alphabetical order, deep-merged on top of the base.
//
// Missing files are not an error (most machines won't have a managed layer).
// Malformed JSON is logged to stderr and skipped so one bad drop-in doesn't
// block every ccpm launch on the machine. This mirrors the tolerance used for
// ~/.claude/settings.json.
func LoadManagedSettings() (map[string]interface{}, error) {
	dir := managedSettingsDir()
	if dir == "" {
		return map[string]interface{}{}, nil
	}

	merged := map[string]interface{}{}

	basePath := filepath.Join(dir, "managed-settings.json")
	if data, err := readIfExists(basePath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read %s: %v\n", basePath, err)
