package settingsmerge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDeepMergeEmptyDst(t *testing.T) {
	dst := map[string]interface{}{}
	src := map[string]interface{}{"key": "value"}
	result := DeepMerge(dst, src)
	if result["key"] != "value" {
		t.Errorf("expected key=value, got %v", result["key"])
	}
}

func TestDeepMergeEmptySrc(t *testing.T) {
	dst := map[string]interface{}{"key": "value"}
	src := map[string]interface{}{}
	result := DeepMerge(dst, src)
	if result["key"] != "value" {
		t.Errorf("expected key=value, got %v", result["key"])
	}
}

func TestDeepMergeScalarOverwrite(t *testing.T) {
	dst := map[string]interface{}{"model": "old"}
	src := map[string]interface{}{"model": "new"}
	result := DeepMerge(dst, src)
	if result["model"] != "new" {
		t.Errorf("expected model=new, got %v", result["model"])
	}
}

func TestDeepMergeNestedObjects(t *testing.T) {
	dst := map[string]interface{}{
		"permissions": map[string]interface{}{
			"allow": []interface{}{"read"},
			"deny":  []interface{}{"delete"},
		},
	}
	src := map[string]interface{}{
		"permissions": map[string]interface{}{
			"allow": []interface{}{"read", "write"},
		},
	}
	result := DeepMerge(dst, src)

	perms, ok := result["permissions"].(map[string]interface{})
	if !ok {
		t.Fatal("permissions should be a map")
	}

	allow, ok := perms["allow"].([]interface{})
	if !ok {
		t.Fatal("allow should be an array")
	}
	if len(allow) != 2 {
		t.Errorf("allow should have 2 items (src overwrites), got %d", len(allow))
	}

	deny, ok := perms["deny"].([]interface{})
	if !ok {
		t.Fatal("deny should still exist from dst")
	}
	if len(deny) != 1 {
		t.Errorf("deny should have 1 item, got %d", len(deny))
	}
}

func TestDeepMergeDstNotMutated(t *testing.T) {
	dst := map[string]interface{}{"a": "1"}
	src := map[string]interface{}{"b": "2"}
	result := DeepMerge(dst, src)
	if _, ok := dst["b"]; ok {
		t.Error("dst should not be mutated")
	}
	if result["a"] != "1" || result["b"] != "2" {
		t.Error("result should contain both keys")
	}
}

func TestLoadJSONMissing(t *testing.T) {
	m, err := LoadJSON("/nonexistent/path.json")
	if err != nil {
		t.Fatalf("missing file should return empty map, got error: %v", err)
	}
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestLoadAndWriteJSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.json")

	data := map[string]interface{}{
		"model": "test",
		"mcpServers": map[string]interface{}{
			"github": map[string]interface{}{
				"command": "npx",
			},
		},
	}

	if err := WriteJSON(path, data); err != nil {
		t.Fatalf("WriteJSON error: %v", err)
	}

	loaded, err := LoadJSON(path)
	if err != nil {
		t.Fatalf("LoadJSON error: %v", err)
	}

	if loaded["model"] != "test" {
		t.Errorf("model = %v, want test", loaded["model"])
	}

	servers, ok := loaded["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatal("mcpServers should be a map")
	}
	if _, ok := servers["github"]; !ok {
		t.Error("github server should exist")
	}
}

func TestWriteJSONAtomic(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "atomic.json")

	data := map[string]interface{}{"key": "value"}
	if err := WriteJSON(path, data); err != nil {
		t.Fatalf("WriteJSON error: %v", err)
	}

	entries, _ := os.ReadDir(tmp)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestMaterialize(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	settingsDir := filepath.Join(tmp, ".ccpm", "share", "settings")
	os.MkdirAll(settingsDir, 0755)

	profileDir := filepath.Join(tmp, ".ccpm", "profiles", "work")
	os.MkdirAll(profileDir, 0755)

	// Global fragment
	globalData := map[string]interface{}{
		"model": "claude-sonnet-4-20250514",
		"permissions": map[string]interface{}{
			"allow": []interface{}{"Bash(git:*)"},
		},
	}
	globalBytes, _ := json.MarshalIndent(globalData, "", "  ")
	os.WriteFile(filepath.Join(settingsDir, "global.json"), globalBytes, 0644)

	// Profile fragment overrides model
	profileData := map[string]interface{}{
		"model": "claude-opus-4-20250514",
	}
	profileBytes, _ := json.MarshalIndent(profileData, "", "  ")
	os.WriteFile(filepath.Join(settingsDir, "work.json"), profileBytes, 0644)

	if err := Materialize(profileDir, "work"); err != nil {
		t.Fatalf("Materialize error: %v", err)
	}

	result, err := LoadJSON(filepath.Join(profileDir, "settings.json"))
	if err != nil {
		t.Fatalf("LoadJSON error: %v", err)
	}

	if result["model"] != "claude-opus-4-20250514" {
		t.Errorf("model should be overridden to claude-opus-4-20250514, got %v", result["model"])
	}

	perms, ok := result["permissions"].(map[string]interface{})
	if !ok {
		t.Fatal("permissions should exist from global")
	}
	if _, ok := perms["allow"]; !ok {
		t.Error("permissions.allow should exist from global")
	}
}

func TestMaterializeMCP(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	mcpDir := filepath.Join(tmp, ".ccpm", "share", "mcp")
	os.MkdirAll(mcpDir, 0755)

	profileDir := filepath.Join(tmp, ".ccpm", "profiles", "work")
	os.MkdirAll(profileDir, 0755)

	// Global MCP fragment
	globalMCP := map[string]interface{}{
		"github": map[string]interface{}{
			"command": "npx",
			"args":    []interface{}{"-y", "@modelcontextprotocol/server-github"},
		},
	}
	globalBytes, _ := json.MarshalIndent(globalMCP, "", "  ")
	os.WriteFile(filepath.Join(mcpDir, "global.json"), globalBytes, 0644)

	// Existing profile settings with a pre-existing MCP server
	existing := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"slack": map[string]interface{}{
				"command": "npx",
			},
		},
	}
	existingBytes, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(filepath.Join(profileDir, "settings.json"), existingBytes, 0644)

	if err := MaterializeMCP(profileDir, "work"); err != nil {
		t.Fatalf("MaterializeMCP error: %v", err)
	}

	result, err := LoadJSON(filepath.Join(profileDir, "settings.json"))
	if err != nil {
		t.Fatalf("LoadJSON error: %v", err)
	}

	servers, ok := result["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatal("mcpServers should be a map")
	}

	if _, ok := servers["github"]; !ok {
		t.Error("github should be present from global MCP fragment")
	}
	if _, ok := servers["slack"]; !ok {
		t.Error("slack should be preserved from existing settings")
	}
}

// TestMaterializeOwnedKeysWin ensures a key recorded as owned in the profile
// fragment beats whatever the profile's settings.json has.
func TestMaterializeOwnedKeysWin(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	settingsDir := filepath.Join(tmp, ".ccpm", "share", "settings")
	os.MkdirAll(settingsDir, 0755)

	profileDir := filepath.Join(tmp, ".ccpm", "profiles", "work")
	os.MkdirAll(profileDir, 0755)

	// User ran `ccpm settings set model claude-opus-4 --profile work`
	fragPath := filepath.Join(settingsDir, "work.json")
	frag := map[string]interface{}{"model": "claude-opus-4"}
	fragBytes, _ := json.MarshalIndent(frag, "", "  ")
	os.WriteFile(fragPath, fragBytes, 0644)
	if err := MarkOwned(fragPath, "model"); err != nil {
		t.Fatalf("MarkOwned: %v", err)
	}

	// Claude Code (or the user) wrote a different value into settings.json.
	existing := map[string]interface{}{"model": "claude-sonnet-3.5"}
	existingBytes, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(filepath.Join(profileDir, "settings.json"), existingBytes, 0644)

	if err := Materialize(profileDir, "work"); err != nil {
		t.Fatalf("Materialize: %v", err)
	}

	result, err := LoadJSON(filepath.Join(profileDir, "settings.json"))
	if err != nil {
		t.Fatalf("LoadJSON: %v", err)
	}
	if result["model"] != "claude-opus-4" {
		t.Errorf("expected owned model=claude-opus-4 to win, got %v", result["model"])
	}
}

// TestMaterializeUnownedKeysPreserved ensures keys that aren't ccpm-owned
// remain writable by the user via settings.json.
func TestMaterializeUnownedKeysPreserved(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	settingsDir := filepath.Join(tmp, ".ccpm", "share", "settings")
	os.MkdirAll(settingsDir, 0755)
	profileDir := filepath.Join(tmp, ".ccpm", "profiles", "work")
	os.MkdirAll(profileDir, 0755)

	// Global fragment suggests a theme, but we do NOT mark it owned.
	fragPath := filepath.Join(settingsDir, "global.json")
	os.WriteFile(fragPath, []byte(`{"theme":"light"}`), 0644)

	// User changed theme in settings.json directly.
	os.WriteFile(filepath.Join(profileDir, "settings.json"), []byte(`{"theme":"dark"}`), 0644)

	if err := Materialize(profileDir, "work"); err != nil {
		t.Fatalf("Materialize: %v", err)
	}

	result, err := LoadJSON(filepath.Join(profileDir, "settings.json"))
	if err != nil {
		t.Fatalf("LoadJSON: %v", err)
	}
	if result["theme"] != "dark" {
		t.Errorf("expected user-edited theme=dark to survive, got %v", result["theme"])
	}
}

// TestMaterializeMCPIsolation ensures a profile never picks up MCP servers
// from another profile's fragment, regardless of what other *.json files
// live in ~/.ccpm/share/mcp/.
func TestMaterializeMCPIsolation(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	mcpDir := filepath.Join(tmp, ".ccpm", "share", "mcp")
	os.MkdirAll(mcpDir, 0755)

	personalDir := filepath.Join(tmp, ".ccpm", "profiles", "personal")
	os.MkdirAll(personalDir, 0755)

	// Other profile's MCP fragment — should NOT leak into "personal".
	workMCP := map[string]interface{}{
		"work-secret": map[string]interface{}{"command": "npx"},
	}
	workBytes, _ := json.MarshalIndent(workMCP, "", "  ")
	os.WriteFile(filepath.Join(mcpDir, "work.json"), workBytes, 0644)

	// Personal's own fragment.
	personalMCP := map[string]interface{}{
		"personal-notes": map[string]interface{}{"command": "npx"},
	}
	personalBytes, _ := json.MarshalIndent(personalMCP, "", "  ")
	os.WriteFile(filepath.Join(mcpDir, "personal.json"), personalBytes, 0644)

	if err := MaterializeMCP(personalDir, "personal"); err != nil {
		t.Fatalf("MaterializeMCP error: %v", err)
	}

	result, err := LoadJSON(filepath.Join(personalDir, "settings.json"))
	if err != nil {
		t.Fatalf("LoadJSON error: %v", err)
	}
	servers, _ := result["mcpServers"].(map[string]interface{})
	if _, leaked := servers["work-secret"]; leaked {
		t.Error("work-secret MCP server leaked into personal profile")
	}
	if _, ok := servers["personal-notes"]; !ok {
		t.Error("personal-notes MCP server should be present from personal.json")
	}
}
