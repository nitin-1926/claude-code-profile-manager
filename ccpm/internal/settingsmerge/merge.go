package settingsmerge

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/atomicwrite"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/trust"
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
	// doesn't trigger the stale-mcpServers cleanup in MaterializeAll.
	delete(doc, "mcpServers")
	return doc, nil
}

// DeepMerge merges src into dst recursively, returning a fresh structure.
// Objects merge key-by-key; arrays and scalars in src replace the dst value.
//
// The result shares NO map or slice references with dst or src — every nested
// container is cloned, so mutating the result (or either input) afterwards
// can't corrupt the other. JSON-decoded values are cheap to clone and the
// maps involved are small profile settings, so the safety is worth the copy.
func DeepMerge(dst, src map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(dst)+len(src))
	for k, v := range dst {
		out[k] = cloneJSONValue(v)
	}
	for k, v := range src {
		if srcMap, ok := v.(map[string]interface{}); ok {
			if dstMap, ok := out[k].(map[string]interface{}); ok {
				out[k] = DeepMerge(dstMap, srcMap)
				continue
			}
		}
		out[k] = cloneJSONValue(v)
	}
	return out
}

// cloneJSONValue deep-copies the container types produced by encoding/json
// (map[string]interface{} and []interface{}); scalars are returned as-is.
func cloneJSONValue(v interface{}) interface{} {
	switch tv := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(tv))
		for k, val := range tv {
			out[k] = cloneJSONValue(val)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(tv))
		for i, val := range tv {
			out[i] = cloneJSONValue(val)
		}
		return out
	default:
		return v
	}
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
	// NOTE: deliberately NOT routed through atomicwrite. WriteJSON also writes
	// the Claude-Code-owned ~/.claude/settings.json (via the set-default API-key
	// env path), which a user may have symlinked into a dotfiles repo;
	// atomicwrite refuses to overwrite a symlink and would hard-fail there.
	// Instead, an existing symlink target is resolved first and the temp+rename
	// happens AT the resolved location — the symlink itself is preserved and
	// the write lands where the user pointed it.
	dest := path
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("resolving symlinked settings path %s: %w", path, err)
		}
		dest = resolved
	}
	// Unique, exclusively-created staging file (CreateTemp uses O_EXCL and
	// 0600 == config.FilePerm): a fixed "<dest>.tmp" would follow a symlink
	// planted there and let two concurrent writers clobber each other.
	f, err := os.CreateTemp(filepath.Dir(dest), filepath.Base(dest)+".*.tmp")
	if err != nil {
		return fmt.Errorf("staging %s: %w", dest, err)
	}
	tmp := f.Name()
	if _, err := f.Write(bytes); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	// fsync before rename so a crash can't commit the rename without the
	// contents (same durability story as internal/atomicwrite).
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("syncing %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("closing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming to %s: %w", dest, err)
	}
	return nil
}

// ComputeMerged returns the effective settings map for a profile as Claude
// Code sees it when launched from projectRoot, without writing to disk.
// Advisory commands (`ccpm settings get/show`, `ccpm hooks list`,
// `ccpm plugin list`) and the desktop app use it to describe that state.
//
// Precedence (lowest → highest, higher wins):
//  1. The profile layers MaterializeAll persists — see profileSettings.
//  2. Project <projectRoot>/.claude/settings.json (if projectRoot != "")
//  3. Project <projectRoot>/.claude/settings.local.json (if projectRoot != "")
//  4. Enterprise/managed settings — OS-level org policy file plus any
//     drop-ins under managed-settings.d/. Highest precedence so admin
//     policy always wins over per-user, per-profile, and per-project
//     layers, matching native Claude Code semantics.
//
// Layers 2–4 are view-only: Claude Code reads them itself (project files from
// the working directory, behind its own workspace-trust prompt; managed policy
// from the system directory), so they are never written into the profile.
func ComputeMerged(profileDir, profileName, projectRoot string) (map[string]interface{}, error) {
	merged, _, err := profileSettings(profileDir, profileName, loadMaterialized(profileDir).Settings)
	if err != nil {
		return nil, err
	}

	projectSettings, projectLocal, err := LoadProjectSettings(projectRoot)
	if err != nil {
		return nil, err
	}
	delete(projectSettings, "mcpServers")
	delete(projectLocal, "mcpServers")
	// Leave an untrusted project's non-allowlisted keys out of the view: a
	// project's settings.json is controlled by whoever pushed the repo, and
	// ccpm won't present its hooks or command helpers as in effect until the
	// user opts in via `ccpm trust add <path>`.
	projectSettings, stripped := trust.FilterProjectLayer(projectSettings, projectRoot)
	trust.WarnUntrusted(projectRoot, stripped)
	projectLocal, strippedLocal := trust.FilterProjectLayer(projectLocal, projectRoot)
	trust.WarnUntrusted(projectRoot, strippedLocal)
	// Even a TRUSTED project must not set PATH/loader/interpreter env vars —
	// a one-time trust grant is consent to hooks the user reviewed, not an
	// open-ended exec channel for future commits.
	projectSettings, strippedEnv := trust.FilterEnvAlways(projectSettings)
	trust.WarnStrippedEnv(projectRoot, strippedEnv)
	projectLocal, strippedEnvLocal := trust.FilterEnvAlways(projectLocal)
	trust.WarnStrippedEnv(projectRoot, strippedEnvLocal)
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

// profileSettings merges the layers MaterializeAll writes to
// <profileDir>/settings.json. It returns that map plus written, the part ccpm
// contributed, which MaterializeAll records in the sidecar so the next
// rebuild can tell ccpm's keys from the user's. prev is the previous record.
//
// Precedence (lowest → highest, higher wins):
//  1. User layer: <profileDir>/settings.json minus what ccpm wrote there last
//     time (see userOwned) — keys Claude Code or the user wrote directly
//     (/model, /permissions, …) survive; ccpm's own keys are re-derived below,
//     so one whose source is gone disappears.
//  2. Host ~/.claude/settings.json — the native Claude user/global layer.
//     Editing this file changes defaults for every ccpm profile, mirroring
//     native Claude semantics; it replaces the old ccpm-managed
//     ~/.ccpm/share/settings/global.json fragment (removed 2026-04-22).
//  3. Profile ccpm fragment ~/.ccpm/share/settings/<profileName>.json
//  4. Profile owned-keys re-assertion — any leaf key recorded in
//     <profileName>.owned.json is re-applied from the fragment so Claude
//     Code can't silently shadow a value the user set via
//     `ccpm settings set --profile`.
func profileSettings(profileDir, profileName string, prev map[string]interface{}) (merged, written map[string]interface{}, err error) {
	shareDir, err := share.SettingsDir()
	if err != nil {
		return nil, nil, err
	}
	profileFragPath := filepath.Join(shareDir, profileName+".json")

	profileFrag, err := LoadJSON(profileFragPath)
	if err != nil {
		return nil, nil, fmt.Errorf("loading profile settings fragment: %w", err)
	}

	existing, err := LoadJSON(filepath.Join(profileDir, "settings.json"))
	if err != nil {
		return nil, nil, fmt.Errorf("loading existing profile settings: %w", err)
	}

	hostSettings, err := loadHostClaudeSettings()
	if err != nil {
		return nil, nil, fmt.Errorf("loading host ~/.claude/settings.json: %w", err)
	}

	profileOwned, err := LoadOwnedKeys(profileFragPath)
	if err != nil {
		return nil, nil, fmt.Errorf("loading owned-keys for profile fragment: %w", err)
	}
	ccpm := applyOwnedKeys(DeepMerge(hostSettings, profileFrag), profileFrag, profileOwned)

	// settings.json never legitimately holds mcpServers — older ccpm versions
	// wrote them there before discovering Claude Code reads from .claude.json.
	// Dropping the key here makes the cleanup part of the normal rebuild.
	delete(ccpm, "mcpServers")
	user := userOwned(existing, prev, ccpm)
	delete(user, "mcpServers")

	return DeepMerge(user, ccpm), stripEqual(ccpm, user), nil
}

// MaterializeAll computes the profile's settings.json and .claude.json
// mcpServers and writes both, plus the sidecar recording what ccpm
// contributed, in a single atomicwrite transaction. Either every file reaches
// its new state, or none does — a crash, disk-full, or permissions error
// mid-merge cannot leave the profile half-written.
//
// Only profile-scoped sources are written (see profileSettings and
// profileMCPServers). Project and managed layers never are: Claude Code
// applies them itself, and a copy in the profile's user-scope files would
// keep applying them everywhere after the user left the repo, revoked trust,
// or the admin withdrew the policy. projectRoot is therefore unused; it stays
// in the signature so callers that pass the launch directory need not change.
//
// This is the function `ccpm run` (and any other command that materializes a
// profile in one shot) should call.
func MaterializeAll(profileDir, profileName, projectRoot string) error {
	prev := loadMaterialized(profileDir)

	settings, writtenSettings, err := profileSettings(profileDir, profileName, prev.Settings)
	if err != nil {
		return err
	}

	claudeJSONPath := filepath.Join(profileDir, ".claude.json")
	existing, err := LoadJSON(claudeJSONPath)
	if err != nil {
		return fmt.Errorf("loading profile .claude.json: %w", err)
	}
	mcpServers, writtenMCP, err := profileMCPServers(profileName, existing, prev.MCPServers)
	if err != nil {
		return err
	}

	settingsBytes, err := marshalIndentedJSON(settings)
	if err != nil {
		return fmt.Errorf("marshaling settings.json: %w", err)
	}

	changes := []atomicwrite.FileChange{
		atomicwrite.WriteFile(filepath.Join(profileDir, "settings.json"), settingsBytes, config.FilePerm),
	}

	// Also rewrite when the map shrank to empty, or the last removal would
	// never reach the file.
	if _, had := existing["mcpServers"]; had || len(mcpServers) > 0 {
		existing["mcpServers"] = mcpServers
		claudeBytes, err := marshalIndentedJSON(existing)
		if err != nil {
			return fmt.Errorf("marshaling .claude.json: %w", err)
		}
		changes = append(changes, atomicwrite.WriteFile(claudeJSONPath, claudeBytes, config.FilePerm))
	}

	record, err := json.MarshalIndent(materialized{Settings: fingerprintLeaves(writtenSettings), MCPServers: writtenMCP}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", materializedFile, err)
	}
	changes = append(changes, atomicwrite.WriteFile(filepath.Join(profileDir, materializedFile), append(record, '\n'), config.FilePerm))

	if err := os.MkdirAll(profileDir, config.DirPerm); err != nil {
		return fmt.Errorf("creating profile dir: %w", err)
	}
	return atomicwrite.Apply(changes)
}

func marshalIndentedJSON(m map[string]interface{}) ([]byte, error) {
	bytes, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(bytes, '\n'), nil
}

// profileMCPServers returns the mcpServers map MaterializeAll writes to the
// profile's .claude.json (where Claude Code reads user-scope MCP config),
// plus written, name → fingerprint of each server ccpm contributed. existing
// is the parsed .claude.json; prev is the record from last time.
//
// Merge precedence (later wins):
//  1. Servers in <profile>/.claude.json#mcpServers that ccpm didn't write —
//     added in a session (`claude mcp add --scope user`) or edited since.
//     Compared per whole server, never per field, so a kept entry is never
//     a partial definition.
//  2. Host top-level ~/.claude.json#mcpServers — so any MCP installed via
//     `claude mcp add --scope user`, `npx <thing> setup`, etc. auto-
//     propagates into every profile.
//  3. ccpm global fragment ~/.ccpm/share/mcp/global.json — ccpm-managed
//     servers shared across profiles.
//  4. ccpm profile fragment ~/.ccpm/share/mcp/<profile>.json — profile-
//     specific overrides.
func profileMCPServers(profileName string, existing, prev map[string]interface{}) (merged, written map[string]interface{}, err error) {
	mcpDir, err := share.MCPDir()
	if err != nil {
		return nil, nil, err
	}

	user := make(map[string]interface{})
	if v, ok := existing["mcpServers"].(map[string]interface{}); ok {
		for name, def := range v {
			if w, wrote := prev[name]; wrote && w == fingerprint(def) {
				continue
			}
			user[name] = def
		}
	}

	ccpm := make(map[string]interface{})
	if hostMCP, err := loadHostClaudeJSONMCP(); err != nil {
		return nil, nil, fmt.Errorf("loading host ~/.claude.json mcpServers: %w", err)
	} else {
		maps.Copy(ccpm, hostMCP)
	}

	if _, err := os.Stat(mcpDir); !os.IsNotExist(err) {
		globalMCP, err := LoadJSON(filepath.Join(mcpDir, "global.json"))
		if err != nil {
			return nil, nil, fmt.Errorf("loading global MCP fragment: %w", err)
		}
		maps.Copy(ccpm, globalMCP)

		profileMCP, err := LoadJSON(filepath.Join(mcpDir, profileName+".json"))
		if err != nil {
			return nil, nil, fmt.Errorf("loading profile MCP fragment: %w", err)
		}
		maps.Copy(ccpm, profileMCP)
	}

	merged = maps.Clone(user)
	maps.Copy(merged, ccpm)
	written = make(map[string]interface{}, len(ccpm))
	for name, def := range ccpm {
		if u, ok := user[name]; !ok || !equalJSON(u, def) {
			written[name] = fingerprint(def)
		}
	}
	return merged, written, nil
}
