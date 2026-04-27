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
	} else if data != nil {
		if parsed, ok := parseJSONTolerant(basePath, data); ok {
			merged = DeepMerge(merged, parsed)
		}
	}

	dropDir := filepath.Join(dir, "managed-settings.d")
	entries, err := os.ReadDir(dropDir)
	if err == nil {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			path := filepath.Join(dropDir, name)
			data, readErr := readIfExists(path)
			if readErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not read %s: %v\n", path, readErr)
				continue
			}
			if data == nil {
				continue
			}
			if parsed, ok := parseJSONTolerant(path, data); ok {
				merged = DeepMerge(merged, parsed)
			}
		}
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Warning: could not list %s: %v\n", dropDir, err)
	}

	return merged, nil
}

// ManagedMCP returns the managed layer's mcpServers block (if any). It's
// stripped from the settings map before being handed back so the
// settings-side merge doesn't trip the stale-mcpServers cleanup.
func ManagedMCP(managed map[string]interface{}) map[string]interface{} {
	servers, _ := managed["mcpServers"].(map[string]interface{})
	if servers == nil {
		return map[string]interface{}{}
	}
	delete(managed, "mcpServers")
	return servers
}

func readIfExists(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}

func parseJSONTolerant(path string, data []byte) (map[string]interface{}, bool) {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: skipping malformed %s: %v\n", path, err)
		return nil, false
	}
	if m == nil {
		return map[string]interface{}{}, true
	}
	return m, true
}
