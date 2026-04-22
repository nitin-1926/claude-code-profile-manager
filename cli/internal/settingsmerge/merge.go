package settingsmerge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nitin-1926/ccpm/internal/share"
)

// loadHostClaudeJSONMCP reads the user's host ~/.claude.json (the one Claude
// Code maintains without CLAUDE_CONFIG_DIR set) and returns its top-level
// mcpServers map. Missing file or absent key returns an empty map. Kept local
// to this package so the defaultclaude import pipeline doesn't need to grow a
// dependency on the live host state.
func loadHostClaudeJSONMCP() (map[string]interface{}, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return map[string]interface{}{}, nil
	}
	path := filepath.Join(home, ".claude.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]interface{}{}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		// Malformed host state shouldn't break profile materialization —
		// the same file has its own parse error recovery inside Claude Code.
		return map[string]interface{}{}, nil
	}
	servers, _ := doc["mcpServers"].(map[string]interface{})
	if servers == nil {
		return map[string]interface{}{}, nil
	}
	return servers, nil
}

// loadHostClaudeSettings reads ~/.claude/settings.json — the file native
// Claude Code uses as the user/global settings layer when CLAUDE_CONFIG_DIR
// is unset. ccpm treats it as the cross-profile baseline for settings, so
// editing it with a text editor (or running `claude /config ...` natively)
// changes defaults for every ccpm profile on the next materialize.
//
// Missing file returns an empty map. Malformed JSON is tolerated the same
// way loadHostClaudeJSONMCP tolerates it — we don't want a broken host file
// to take every ccpm profile down with it.
func loadHostClaudeSettings() (map[string]interface{}, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return map[string]interface{}{}, nil
	}
	path := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]interface{}{}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return map[string]interface{}{}, nil
	}
	if doc == nil {
		return map[string]interface{}{}, nil
	}
	// mcpServers here would be unusual (native Claude reads MCPs from
	// ~/.claude.json, not this file), but if present we strip it so it
	// doesn't trigger the stale-mcpServers cleanup in MaterializeMCP.
	delete(doc, "mcpServers")
	return doc, nil
}

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

// Materialize builds the effective settings.json for a profile.
//
// Precedence (lowest → highest, higher wins):
//  1. Existing <profileDir>/settings.json (preserves any keys Claude Code
//     auto-wrote during a previous session that nothing else redefines)
//  2. Host ~/.claude/settings.json — the native Claude user/global layer.
//     Editing this file changes defaults for every ccpm profile, mirroring
//     native Claude semantics; it replaces the old ccpm-managed
//     ~/.ccpm/share/settings/global.json fragment (removed 2026-04-22).
//  3. Profile ccpm fragment ~/.ccpm/share/settings/<profileName>.json
//  4. Profile owned-keys re-assertion — any leaf key recorded in
//     <profileName>.owned.json is re-applied from the fragment so Claude
//     Code can't silently shadow a value the user set via
//     `ccpm settings set --profile`.
//  5. Project <projectRoot>/.claude/settings.json (if projectRoot != "")
//  6. Project <projectRoot>/.claude/settings.local.json (if projectRoot != "")
//
// Project layers (5, 6) are highest-precedence so per-repo overrides beat
// ccpm's managed keys — an explicit per-project file is a stronger user
// signal than a profile-wide default. Pass projectRoot="" from non-launch
// codepaths that shouldn't bake CWD-relative state into the profile.
//
// Result is written back to profileDir/settings.json.
func Materialize(profileDir, profileName, projectRoot string) error {
	shareDir, err := share.SettingsDir()
	if err != nil {
		return err
	}

	profileFragPath := filepath.Join(shareDir, profileName+".json")
	targetPath := filepath.Join(profileDir, "settings.json")

	profileFrag, err := LoadJSON(profileFragPath)
	if err != nil {
		return fmt.Errorf("loading profile settings fragment: %w", err)
	}

	existing, err := LoadJSON(targetPath)
	if err != nil {
		return fmt.Errorf("loading existing profile settings: %w", err)
	}

	hostSettings, err := loadHostClaudeSettings()
	if err != nil {
		return fmt.Errorf("loading host ~/.claude/settings.json: %w", err)
	}

	// Layers 1 → 2 → 3. Start from existing (claude auto-writes survive when
	// nothing else redefines the key), overlay host settings, overlay the
	// profile fragment.
	merged := DeepMerge(existing, hostSettings)
	merged = DeepMerge(merged, profileFrag)

	// Layer 4: re-assert profile-level owned keys. This guarantees a key
	// set via `ccpm settings set --profile` cannot be silently shadowed by
	// a drift in <profileDir>/settings.json or by the host file.
	profileOwned, err := LoadOwnedKeys(profileFragPath)
	if err != nil {
		return fmt.Errorf("loading owned-keys for profile fragment: %w", err)
	}
	merged = applyOwnedKeys(merged, profileFrag, profileOwned)

	// Layers 5 + 6: project settings. mcpServers belong in .claude.json;
	// strip them so the stale-mcpServers cleanup in MaterializeMCP doesn't
	// immediately delete what we just merged in.
	projectSettings, projectLocal, err := LoadProjectSettings(projectRoot)
	if err != nil {
		return err
	}
	delete(projectSettings, "mcpServers")
	delete(projectLocal, "mcpServers")
	merged = DeepMerge(merged, projectSettings)
	merged = DeepMerge(merged, projectLocal)

	return WriteJSON(targetPath, merged)
}

// MaterializeMCP merges MCP server definitions into the profile's .claude.json
// under the top-level "mcpServers" key — that's where Claude Code actually
// reads user-scope MCP config from. settings.json#mcpServers is a no-op as far
// as Claude Code is concerned, so any stale entries left there by earlier ccpm
// versions are cleaned up here.
//
// Merge precedence (later wins):
//  1. Servers already present in <profile>/.claude.json#mcpServers (lowest —
//     preserved so previously-materialized state survives when no newer
//     source redefines a given server).
//  2. Host top-level ~/.claude.json#mcpServers — so any MCP installed via
//     `claude mcp add --scope user`, `npx <thing> setup`, etc. auto-
//     propagates into every profile.
//  3. ccpm global fragment ~/.ccpm/share/mcp/global.json — ccpm-managed
//     servers shared across profiles.
//  4. ccpm profile fragment ~/.ccpm/share/mcp/<profile>.json — profile-
//     specific overrides.
//  5. Project-level MCPs: <projectRoot>/.claude/settings.json#mcpServers
//     followed by <projectRoot>/.mcp.json (.mcp.json wins on collision).
//     Highest precedence so project MCPs override profile/global.
//
// Pass projectRoot="" to skip the project layer.
func MaterializeMCP(profileDir, profileName, projectRoot string) error {
	mcpDir, err := share.MCPDir()
	if err != nil {
		return err
	}

	claudeJSONPath := filepath.Join(profileDir, ".claude.json")
	existing, err := LoadJSON(claudeJSONPath)
	if err != nil {
		return fmt.Errorf("loading profile .claude.json: %w", err)
	}

	// Layer 4 (lowest implicit priority — gets overwritten by everything else):
	// preserve whatever the profile already had.
	mcpServers := make(map[string]interface{})
	if v, ok := existing["mcpServers"].(map[string]interface{}); ok {
		for k, v := range v {
			mcpServers[k] = v
		}
	}

	// Layer 1: host ~/.claude.json top-level mcpServers.
	if hostMCP, err := loadHostClaudeJSONMCP(); err != nil {
		return fmt.Errorf("loading host ~/.claude.json mcpServers: %w", err)
	} else {
		for k, v := range hostMCP {
			mcpServers[k] = v
		}
	}

	// Layers 2 + 3: ccpm fragments. Only merge the global fragment and this
	// profile's fragment — reading every *.json in the directory would leak
	// other profiles' MCP servers into this profile's config.
	if _, err := os.Stat(mcpDir); !os.IsNotExist(err) {
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
	}

	// Layer 5: project-level MCPs from .claude/settings.json#mcpServers and
	// .mcp.json at the project root. Highest precedence — wins over profile.
	projectMCP, err := LoadProjectMCP(projectRoot)
	if err != nil {
		return err
	}
	for k, v := range projectMCP {
		mcpServers[k] = v
	}

	if len(mcpServers) > 0 {
		existing["mcpServers"] = mcpServers
		if err := WriteJSON(claudeJSONPath, existing); err != nil {
			return fmt.Errorf("writing profile .claude.json: %w", err)
		}
	}

	// Clean up stale mcpServers left in settings.json by older ccpm versions.
	// Claude Code never read that location, so any data there is either
	// already-migrated (duplicated in .claude.json now) or was ineffective
	// from the start.
	settingsPath := filepath.Join(profileDir, "settings.json")
	settings, serr := LoadJSON(settingsPath)
	if serr == nil {
		if _, present := settings["mcpServers"]; present {
			delete(settings, "mcpServers")
			if err := WriteJSON(settingsPath, settings); err != nil {
				return fmt.Errorf("cleaning stale mcpServers from settings.json: %w", err)
			}
		}
	}

	return nil
}
