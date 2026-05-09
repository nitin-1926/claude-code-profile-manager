package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestRewritePluginMetadataPaths_RewritesEmbeddedAbsPaths covers the regression
// behind the missing-skills-after-rename bug: `installed_plugins.json` and
// `known_marketplaces.json` carry absolute paths under the OLD profile dir,
// and `os.Rename` of the directory leaves those strings stale, so Claude Code
// cannot resolve any plugin under the new name.
func TestRewritePluginMetadataPaths_RewritesEmbeddedAbsPaths(t *testing.T) {
	tmp := t.TempDir()
	newDir := filepath.Join(tmp, "labs")
	oldDir := filepath.Join(tmp, "rocketium")
	if err := os.MkdirAll(filepath.Join(newDir, "plugins"), 0755); err != nil {
		t.Fatal(err)
	}

	installed := map[string]interface{}{
		"version": 2,
		"plugins": map[string]interface{}{
			"compound-engineering@compound-engineering-plugin": []interface{}{
				map[string]interface{}{
					"scope":       "user",
					"installPath": filepath.Join(oldDir, "plugins/cache/compound-engineering-plugin/compound-engineering/3.4.1"),
					"version":     "3.4.1",
				},
			},
			"unrelated@x": []interface{}{
				map[string]interface{}{
					// Path under a totally different profile MUST be left alone.
					"installPath": "/Users/x/.ccpm/profiles/other/plugins/cache/x/y/1",
				},
			},
		},
	}
	mustWriteJSON(t, filepath.Join(newDir, "plugins/installed_plugins.json"), installed)

	marketplaces := map[string]interface{}{
		"compound-engineering-plugin": map[string]interface{}{
			"installLocation": filepath.Join(oldDir, "plugins/marketplaces/compound-engineering-plugin"),
		},
	}
	mustWriteJSON(t, filepath.Join(newDir, "plugins/known_marketplaces.json"), marketplaces)

	if err := rewritePluginMetadataPaths(newDir, oldDir, newDir); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	got := mustReadJSON(t, filepath.Join(newDir, "plugins/installed_plugins.json"))
	plugins := got["plugins"].(map[string]interface{})
	ce := plugins["compound-engineering@compound-engineering-plugin"].([]interface{})[0].(map[string]interface{})
	wantNew := filepath.Join(newDir, "plugins/cache/compound-engineering-plugin/compound-engineering/3.4.1")
	if ce["installPath"] != wantNew {
		t.Errorf("installPath not rewritten:\n got %s\nwant %s", ce["installPath"], wantNew)
	}
	other := plugins["unrelated@x"].([]interface{})[0].(map[string]interface{})
	if other["installPath"] != "/Users/x/.ccpm/profiles/other/plugins/cache/x/y/1" {
		t.Errorf("non-matching path was rewritten: %v", other["installPath"])
	}

	mk := mustReadJSON(t, filepath.Join(newDir, "plugins/known_marketplaces.json"))
	wantMK := filepath.Join(newDir, "plugins/marketplaces/compound-engineering-plugin")
	if mk["compound-engineering-plugin"].(map[string]interface{})["installLocation"] != wantMK {
		t.Errorf("installLocation not rewritten")
	}
}

func TestRewritePluginMetadataPaths_NoopWhenFilesMissing(t *testing.T) {
	tmp := t.TempDir()
	newDir := filepath.Join(tmp, "labs")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := rewritePluginMetadataPaths(newDir, "/old", "/new"); err != nil {
		t.Fatalf("expected no-op, got %v", err)
	}
}

// Substring matches must NOT be rewritten — only exact prefix or prefix + "/".
// Otherwise a profile named "lab" renamed to "labs" would corrupt the path
// "/profiles/labs/..." -> "/profiles/labss/...".
func TestRewritePluginMetadataPaths_OnlyMatchesFullPathBoundary(t *testing.T) {
	tmp := t.TempDir()
	newDir := filepath.Join(tmp, "lab")
	if err := os.MkdirAll(filepath.Join(newDir, "plugins"), 0755); err != nil {
		t.Fatal(err)
	}
	mustWriteJSON(t, filepath.Join(newDir, "plugins/installed_plugins.json"), map[string]interface{}{
		"plugins": map[string]interface{}{
			"x": []interface{}{
				map[string]interface{}{
					// "/labs/..." must NOT be touched when renaming "lab" -> "labs".
					"installPath": "/Users/x/.ccpm/profiles/labs/plugins/cache/x",
				},
			},
		},
	})
	if err := rewritePluginMetadataPaths(newDir, "/Users/x/.ccpm/profiles/lab", "/Users/x/.ccpm/profiles/labs"); err != nil {
		t.Fatal(err)
	}
	got := mustReadJSON(t, filepath.Join(newDir, "plugins/installed_plugins.json"))
	plugins := got["plugins"].(map[string]interface{})
	x := plugins["x"].([]interface{})[0].(map[string]interface{})
	if x["installPath"] != "/Users/x/.ccpm/profiles/labs/plugins/cache/x" {
		t.Errorf("substring path was wrongly rewritten: %v", x["installPath"])
	}
}

func mustWriteJSON(t *testing.T, path string, v interface{}) {
	t.Helper()
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func mustReadJSON(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}
