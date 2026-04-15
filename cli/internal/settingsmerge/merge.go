package settingsmerge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nitin-1926/ccpm/internal/share"
)

// DeepMerge merges src into dst recursively.
// Objects merge key-by-key; all other types (arrays, scalars) in src overwrite dst.
func DeepMerge(dst, src map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(dst))
	for k, v := range dst {
		out[k] = v
	}
	for k, v := range src {
		if srcMap, ok := v.(map[string]interface{}); ok {
			if dstMap, ok := out[k].(map[string]interface{}); ok {
				out[k] = DeepMerge(dstMap, srcMap)
				continue
			}
		}
		out[k] = v
	}
	return out
}

// LoadJSON reads a JSON file into a map. Returns empty map if file doesn't exist.
func LoadJSON(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]interface{}), nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return m, nil
}

// WriteJSON atomically writes a map as formatted JSON.
func WriteJSON(path string, data map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	bytes = append(bytes, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, bytes, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming to %s: %w", path, err)
	}
	return nil
}

// Materialize builds the effective settings.json for a profile by merging:
// 1. Global settings fragment from ~/.ccpm/share/settings/global.json
// 2. Profile-specific fragment from ~/.ccpm/share/settings/<profile>.json
// 3. Existing settings.json in the profile dir (user edits are preserved)
// Result is written back to profileDir/settings.json.
func Materialize(profileDir, profileName string) error {
	shareDir, err := share.SettingsDir()
	if err != nil {
		return err
	}

	globalPath := filepath.Join(shareDir, "global.json")
	profileFragPath := filepath.Join(shareDir, profileName+".json")
	targetPath := filepath.Join(profileDir, "settings.json")

	global, err := LoadJSON(globalPath)
	if err != nil {
		return fmt.Errorf("loading global settings fragment: %w", err)
	}

	profileFrag, err := LoadJSON(profileFragPath)
	if err != nil {
		return fmt.Errorf("loading profile settings fragment: %w", err)
	}

	existing, err := LoadJSON(targetPath)
	if err != nil {
		return fmt.Errorf("loading existing profile settings: %w", err)
	}

	merged := DeepMerge(global, profileFrag)
	merged = DeepMerge(merged, existing)

	// Re-assert ccpm-owned keys on top so the user's settings.json (or
	// Claude Code itself) can't silently shadow keys the user explicitly
	// set via `ccpm settings set`.
	globalOwned, err := LoadOwnedKeys(globalPath)
	if err != nil {
		return fmt.Errorf("loading owned-keys for global fragment: %w", err)
	}
	profileOwned, err := LoadOwnedKeys(profileFragPath)
	if err != nil {
		return fmt.Errorf("loading owned-keys for profile fragment: %w", err)
	}
	merged = applyOwnedKeys(merged, global, globalOwned)
	merged = applyOwnedKeys(merged, profileFrag, profileOwned)

	return WriteJSON(targetPath, merged)
}

// MaterializeMCP merges MCP server definitions from the global and profile
// fragments into the profile's settings.json under the "mcpServers" key.
func MaterializeMCP(profileDir, profileName string) error {
	mcpDir, err := share.MCPDir()
	if err != nil {
		return err
	}

	targetPath := filepath.Join(profileDir, "settings.json")
	existing, err := LoadJSON(targetPath)
	if err != nil {
		return fmt.Errorf("loading profile settings: %w", err)
	}

	mcpServers := make(map[string]interface{})
	if v, ok := existing["mcpServers"].(map[string]interface{}); ok {
		mcpServers = v
	}

	// Only merge the global fragment and this profile's fragment. Reading
	// every *.json in the directory would leak other profiles' MCP servers
	// into this profile's settings.json.
	if _, err := os.Stat(mcpDir); os.IsNotExist(err) {
		return nil
	}

	globalMCP, err := LoadJSON(filepath.Join(mcpDir, "global.json"))
	if err != nil {
		return fmt.Errorf("loading global MCP fragment: %w", err)
	}
	for k, v := range globalMCP {
		mcpServers[k] = v
	}

	profileMCP, err := LoadJSON(filepath.Join(mcpDir, profileName+".json"))
	if err != nil {
		return fmt.Errorf("loading profile MCP fragment: %w", err)
	}
	for k, v := range profileMCP {
		mcpServers[k] = v
	}

	if len(mcpServers) > 0 {
		existing["mcpServers"] = mcpServers
	}

	return WriteJSON(targetPath, existing)
}
