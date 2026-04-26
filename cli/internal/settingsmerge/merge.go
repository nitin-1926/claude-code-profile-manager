package settingsmerge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/share"
	"github.com/nitin-1926/ccpm/internal/trust"
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

// DeepMerge merges src into dst recursively, returning a fresh top-level map.
// Objects merge key-by-key; arrays and scalars in src replace the dst value.
//
// Aliasing note: the returned map shares value references with dst and src —
// nested submaps are NOT deep-cloned. Callers should treat the result as
// immutable (i.e. never mutate a submap after merging) so a later edit in
// either source doesn't surprise the reader. Every ccpm caller today obeys
// this by discarding dst/src after the merge and writing the result
// atomically via WriteJSON.
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

// WriteJSON atomically writes a map as formatted JSON with user-only perms
// (0600) via config.FilePerm. Profile fragments may carry env entries with
// tokens; keeping this file not-world-readable is part of the security
// baseline for ~/.ccpm.
func WriteJSON(path string, data map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), config.DirPerm); err != nil {
		return err
	}
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	bytes = append(bytes, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, bytes, config.FilePerm); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming to %s: %w", path, err)
	}
	return nil
}

// ComputeMerged returns the fully-merged settings map for a profile without
// writing to disk. It is the single source of truth for the settings-side
// precedence pipeline; Materialize uses it to produce the on-disk state, and
// advisory commands (`ccpm settings get/show`, `ccpm hooks list`,
// `ccpm plugin list`) use it to describe that same state.
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
//  7. Enterprise/managed settings — OS-level org policy file plus any
//     drop-ins under managed-settings.d/. Highest precedence so admin
//     policy always wins over per-user, per-profile, and per-project
//     layers, matching native Claude Code semantics.
//
// Pass projectRoot="" from non-launch codepaths that shouldn't bake
// CWD-relative state into the profile.
func ComputeMerged(profileDir, profileName, projectRoot string) (map[string]interface{}, error) {
	shareDir, err := share.SettingsDir()
	if err != nil {
		return nil, err
	}
	profileFragPath := filepath.Join(shareDir, profileName+".json")

	profileFrag, err := LoadJSON(profileFragPath)
	if err != nil {
		return nil, fmt.Errorf("loading profile settings fragment: %w", err)
	}

	existing, err := LoadJSON(filepath.Join(profileDir, "settings.json"))
	if err != nil {
