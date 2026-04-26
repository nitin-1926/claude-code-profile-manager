package settingsmerge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManagedSettings_BaseFileOnly(t *testing.T) {
	dir := t.TempDir()
	base := map[string]interface{}{
		"permissions": map[string]interface{}{"defaultMode": "plan"},
		"model":       "claude-sonnet-4-6",
	}
	writeJSONForTest(t, filepath.Join(dir, "managed-settings.json"), base)

	restore := SetManagedSettingsDirForTest(dir)
	defer restore()

	got, err := LoadManagedSettings()
	if err != nil {
		t.Fatalf("LoadManagedSettings: %v", err)
	}
	if got["model"] != "claude-sonnet-4-6" {
		t.Fatalf("model: got %v, want claude-sonnet-4-6", got["model"])
	}
	perms, _ := got["permissions"].(map[string]interface{})
	if perms == nil || perms["defaultMode"] != "plan" {
		t.Fatalf("defaultMode: got %v, want plan", perms)
	}
}

func TestLoadManagedSettings_DropInsMergedAlphabetically(t *testing.T) {
	dir := t.TempDir()
	// base
	writeJSONForTest(t, filepath.Join(dir, "managed-settings.json"),
		map[string]interface{}{"outputStyle": "default"})
	// drop-ins — b.json should win over a.json by sort order.
	dropDir := filepath.Join(dir, "managed-settings.d")
	if err := os.MkdirAll(dropDir, 0755); err != nil {
		t.Fatalf("mkdir drop: %v", err)
	}
	writeJSONForTest(t, filepath.Join(dropDir, "a.json"),
		map[string]interface{}{"outputStyle": "Build"})
	writeJSONForTest(t, filepath.Join(dropDir, "b.json"),
		map[string]interface{}{"outputStyle": "Explanatory"})

	restore := SetManagedSettingsDirForTest(dir)
	defer restore()
