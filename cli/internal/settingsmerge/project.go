package settingsmerge

import (
	"fmt"
	"os"
	"path/filepath"
)

// Files that mark a directory as a Claude Code "project" when found during the
// upward CWD walk. Matching any of these makes the directory the project root.
var projectMarkers = []string{
	filepath.Join(".claude", "settings.json"),
	filepath.Join(".claude", "settings.local.json"),
	".mcp.json",
}

// FindProjectRoot walks upward from startDir looking for a directory that
// contains .claude/settings.json, .claude/settings.local.json, or .mcp.json.
// Stops at the filesystem root or the user's home directory (whichever comes
// first) to avoid treating ~/.claude/settings.json as a "project" layer — that
// file already participates as the global layer and re-including it would
// double-count. Returns "" if no project root is found.
func FindProjectRoot(startDir string) string {
	if startDir == "" {
		return ""
	}
	abs, err := filepath.Abs(startDir)
	if err != nil {
		return ""
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		if homeAbs, err := filepath.Abs(home); err == nil {
			home = homeAbs
		}
	}

	dir := abs
	for {
		// Never treat $HOME itself as a project root — its .claude dir is
		// the global/user layer.
		if home != "" && dir == home {
			return ""
		}
		for _, marker := range projectMarkers {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// LoadProjectSettings reads <projectRoot>/.claude/settings.json and
// <projectRoot>/.claude/settings.local.json. Returns empty maps when
// projectRoot is "" or the files are absent. Malformed JSON is returned as
// an error so we fail loud for the project the user is actually working in.
func LoadProjectSettings(projectRoot string) (settings, localSettings map[string]interface{}, err error) {
	if projectRoot == "" {
		return map[string]interface{}{}, map[string]interface{}{}, nil
	}
	settingsPath := filepath.Join(projectRoot, ".claude", "settings.json")
	localPath := filepath.Join(projectRoot, ".claude", "settings.local.json")

	settings, err = LoadJSON(settingsPath)
	if err != nil {
		return nil, nil, fmt.Errorf("loading project settings: %w", err)
	}
	localSettings, err = LoadJSON(localPath)
	if err != nil {
		return nil, nil, fmt.Errorf("loading project settings.local: %w", err)
	}
	return settings, localSettings, nil
}

// LoadProjectMCP collects MCP server definitions declared in the project's
// .claude/settings.json#mcpServers plus .mcp.json at the root. The return
// value is a single merged map where .mcp.json entries override entries of
// the same name from .claude/settings.json (matching Claude CLI's precedence,
// where the dedicated .mcp.json file is authoritative for project-scoped MCPs).
func LoadProjectMCP(projectRoot string) (map[string]interface{}, error) {
	merged := map[string]interface{}{}
	if projectRoot == "" {
		return merged, nil
	}

	settingsPath := filepath.Join(projectRoot, ".claude", "settings.json")
	settings, err := LoadJSON(settingsPath)
	if err != nil {
		return nil, fmt.Errorf("loading project settings for mcpServers: %w", err)
	}
	if servers, ok := settings["mcpServers"].(map[string]interface{}); ok {
		for k, v := range servers {
			merged[k] = v
		}
	}

	mcpJSONPath := filepath.Join(projectRoot, ".mcp.json")
	mcpJSON, err := LoadJSON(mcpJSONPath)
	if err != nil {
		return nil, fmt.Errorf("loading project .mcp.json: %w", err)
	}
	// Claude CLI's .mcp.json uses a top-level mcpServers key, matching the
	// shape used elsewhere. Older/alternate forms that put servers at the
	// top level are also accepted as a fallback.
	if servers, ok := mcpJSON["mcpServers"].(map[string]interface{}); ok {
		for k, v := range servers {
			merged[k] = v
		}
	} else if len(mcpJSON) > 0 {
		for k, v := range mcpJSON {
			if _, isMap := v.(map[string]interface{}); isMap {
				merged[k] = v
			}
		}
	}

	return merged, nil
}
