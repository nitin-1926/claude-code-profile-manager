package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAPIKeyEnvPreservesOtherKeys(t *testing.T) {
	tmp := t.TempDir()

	existing := map[string]interface{}{
		"theme": "dark",
		"env": map[string]interface{}{
			"FOO": "bar",
		},
	}
	raw, _ := json.Marshal(existing)
	if err := os.WriteFile(filepath.Join(tmp, "settings.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}

	if err := writeAPIKeyEnv(tmp, "sk-test-123"); err != nil {
		t.Fatalf("writeAPIKeyEnv: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmp, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out["theme"] != "dark" {
		t.Errorf("theme lost: %v", out["theme"])
	}
	env, _ := out["env"].(map[string]interface{})
	if env["FOO"] != "bar" {
		t.Errorf("pre-existing env key clobbered: %v", env)
	}
	if env["ANTHROPIC_API_KEY"] != "sk-test-123" {
		t.Errorf("API key not written: %v", env["ANTHROPIC_API_KEY"])
	}
}

func TestWriteAPIKeyEnvCreatesFile(t *testing.T) {
	tmp := t.TempDir()

	if err := writeAPIKeyEnv(tmp, "sk-xyz"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(tmp, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	env, _ := out["env"].(map[string]interface{})
	if env["ANTHROPIC_API_KEY"] != "sk-xyz" {
		t.Errorf("API key missing: %v", out)
	}
}

func TestClearAPIKeyEnvStripsOnlyThatKey(t *testing.T) {
	tmp := t.TempDir()
	claude := filepath.Join(tmp, ".claude")
	if err := os.MkdirAll(claude, 0755); err != nil {
		t.Fatal(err)
	}
	existing := map[string]interface{}{
		"theme": "dark",
		"env": map[string]interface{}{
			"ANTHROPIC_API_KEY": "leaked",
			"FOO":               "bar",
		},
	}
	raw, _ := json.Marshal(existing)
	if err := os.WriteFile(filepath.Join(claude, "settings.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", tmp)

	if err := clearAPIKeyEnv(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(claude, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out["theme"] != "dark" {
		t.Errorf("theme lost")
	}
	env, _ := out["env"].(map[string]interface{})
	if _, present := env["ANTHROPIC_API_KEY"]; present {
		t.Errorf("API key still present: %v", env)
	}
	if env["FOO"] != "bar" {
		t.Errorf("other env key lost: %v", env)
	}
}

func TestClearAPIKeyEnvDropsEmptyEnvBlock(t *testing.T) {
	tmp := t.TempDir()
	claude := filepath.Join(tmp, ".claude")
	if err := os.MkdirAll(claude, 0755); err != nil {
		t.Fatal(err)
	}
	existing := map[string]interface{}{
		"env": map[string]interface{}{
			"ANTHROPIC_API_KEY": "x",
		},
	}
	raw, _ := json.Marshal(existing)
	if err := os.WriteFile(filepath.Join(claude, "settings.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", tmp)

	if err := clearAPIKeyEnv(); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(claude, "settings.json"))
	var out map[string]interface{}
	_ = json.Unmarshal(data, &out)
	if _, has := out["env"]; has {
		t.Errorf("empty env block should have been removed: %v", out)
	}
}

func TestClearAPIKeyEnvIsNoopIfMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := clearAPIKeyEnv(); err != nil {
		t.Fatalf("clearAPIKeyEnv on missing file: %v", err)
	}
}
