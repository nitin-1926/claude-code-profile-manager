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
		return nil, fmt.Errorf("loading existing profile settings: %w", err)
	}

	hostSettings, err := loadHostClaudeSettings()
	if err != nil {
		return nil, fmt.Errorf("loading host ~/.claude/settings.json: %w", err)
	}

	merged := DeepMerge(existing, hostSettings)
	merged = DeepMerge(merged, profileFrag)

	profileOwned, err := LoadOwnedKeys(profileFragPath)
	if err != nil {
		return nil, fmt.Errorf("loading owned-keys for profile fragment: %w", err)
	}
	merged = applyOwnedKeys(merged, profileFrag, profileOwned)

	projectSettings, projectLocal, err := LoadProjectSettings(projectRoot)
	if err != nil {
		return nil, err
	}
	delete(projectSettings, "mcpServers")
	delete(projectLocal, "mcpServers")
	// Strip security-sensitive keys from untrusted projects. A project's
	// settings.json is controlled by whoever pushed the repo; we refuse to let
	// it register hooks/permissions/statusLine silently until the user opts
	// in via `ccpm trust add <path>`.
	projectSettings, stripped := trust.FilterProjectLayer(projectSettings, projectRoot)
	trust.WarnUntrusted(projectRoot, stripped)
	projectLocal, strippedLocal := trust.FilterProjectLayer(projectLocal, projectRoot)
	trust.WarnUntrusted(projectRoot, strippedLocal)
	merged = DeepMerge(merged, projectSettings)
	merged = DeepMerge(merged, projectLocal)

	managed, err := LoadManagedSettings()
	if err != nil {
		return nil, fmt.Errorf("loading managed settings: %w", err)
	}
	delete(managed, "mcpServers")
	merged = DeepMerge(merged, managed)

	return merged, nil
}

// Materialize builds the effective settings.json for a profile and writes it
// to <profileDir>/settings.json. See ComputeMerged for the precedence rules.
func Materialize(profileDir, profileName, projectRoot string) error {
	merged, err := ComputeMerged(profileDir, profileName, projectRoot)
	if err != nil {
		return err
	}
	return WriteJSON(filepath.Join(profileDir, "settings.json"), merged)
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
//  6. Managed/enterprise MCPs from managed-settings.json#mcpServers (plus
//     managed-settings.d/*.json). Highest precedence so admin-published
//     servers beat project and profile ones, matching the settings layer.
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
	// .mcp.json at the project root. Skipped entirely when the project isn't
	// trusted: the whole layer is attacker-controllable via `git clone`.
	if trust.IsTrusted(projectRoot) {
		projectMCP, err := LoadProjectMCP(projectRoot)
		if err != nil {
			return err
		}
		for k, v := range projectMCP {
			mcpServers[k] = v
		}
	} else if projectRoot != "" {
		// Peek at what we would have merged so the one-time warning message
		// accurately reports the dropped servers. Unconditional in
		// Materialize above, but here we only warn when there's actually
		// something to drop.
		if projectMCP, err := LoadProjectMCP(projectRoot); err == nil && len(projectMCP) > 0 {
			names := make([]string, 0, len(projectMCP))
			for k := range projectMCP {
				names = append(names, k)
			}
			trust.WarnUntrusted(projectRoot, []string{fmt.Sprintf("mcpServers(%v)", names)})
		}
	}

	// Layer 6: managed/enterprise MCPs. Highest precedence.
	managed, err := LoadManagedSettings()
	if err != nil {
		return fmt.Errorf("loading managed settings: %w", err)
	}
	for k, v := range ManagedMCP(managed) {
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
