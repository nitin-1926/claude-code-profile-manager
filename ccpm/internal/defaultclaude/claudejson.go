package defaultclaude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// MCP scopes as they appear inside ~/.claude.json.
const (
	MCPScopeUser    = "user"    // top-level "mcpServers"
	MCPScopeProject = "project" // projects[<path>].mcpServers
)

// MCPEntry is a single MCP server definition discovered in ~/.claude.json.
// Project is populated only when Scope == MCPScopeProject.
type MCPEntry struct {
	Name       string
	Scope      string
	Project    string
	Definition map[string]interface{}
}

// ID returns a stable identifier used by the interactive picker. Project-scoped
// entries are disambiguated with "@<project>" so two projects with the same
// server name remain distinguishable in the UI.
func (e MCPEntry) ID() string {
	if e.Scope == MCPScopeProject {
		return e.Name + "@" + e.Project
	}
	return e.Name
}

// Source returns a short human label describing where this entry came from.
func (e MCPEntry) Source() string {
	if e.Scope == MCPScopeProject {
		return "project:" + e.Project
	}
	return "user:~/.claude.json"
}

// ClaudeJSONPath returns the location of ~/.claude.json.
func ClaudeJSONPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".claude.json"), nil
}

// ClaudeJSONExists reports whether ~/.claude.json is present.
func ClaudeJSONExists() bool {
	p, err := ClaudeJSONPath()
	if err != nil {
		return false
	}
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// LoadMCPEntries reads ~/.claude.json and returns every MCP server definition
// found, flattened across user-scope and project-scope. Absent file returns
// (nil, nil). Entries are sorted by name, then project path, for stable output.
func LoadMCPEntries() ([]MCPEntry, error) {
	p, err := ClaudeJSONPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", p, err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", p, err)
	}

	var out []MCPEntry

	if top, ok := doc["mcpServers"].(map[string]interface{}); ok {
		for name, v := range top {
			def, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			out = append(out, MCPEntry{
				Name:       name,
				Scope:      MCPScopeUser,
				Definition: def,
			})
		}
	}

	if projects, ok := doc["projects"].(map[string]interface{}); ok {
		for projPath, proj := range projects {
			pm, ok := proj.(map[string]interface{})
			if !ok {
				continue
			}
			servers, ok := pm["mcpServers"].(map[string]interface{})
			if !ok {
				continue
			}
			for name, v := range servers {
				def, ok := v.(map[string]interface{})
				if !ok {
					continue
				}
				out = append(out, MCPEntry{
					Name:       name,
					Scope:      MCPScopeProject,
					Project:    projPath,
					Definition: def,
				})
			}
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Project < out[j].Project
	})
	return out, nil
}
