package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadNonExistent(t *testing.T) {
	// Override home to a temp dir so Load() reads from a non-existent path
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp) // Windows

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() should succeed for missing config, got: %v", err)
	}
	if cfg.Version != configVersion {
		t.Errorf("Version = %q, want %q", cfg.Version, configVersion)
	}
	if cfg.Profiles == nil {
		t.Error("Profiles map should be initialized, got nil")
	}
	if len(cfg.Profiles) != 0 {
		t.Errorf("Profiles should be empty, got %d", len(cfg.Profiles))
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	cfg := &Config{
		Version:        "1",
		DefaultProfile: "test",
		Profiles: map[string]ProfileConfig{
			"test": {
				Name:       "test",
				Dir:        filepath.Join(tmp, ".ccpm", "profiles", "test"),
				AuthMethod: "api_key",
				CreatedAt:  "2026-01-01T00:00:00Z",
				LastUsed:   "2026-01-01T00:00:00Z",
			},
		},
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if loaded.DefaultProfile != "test" {
		t.Errorf("DefaultProfile = %q, want %q", loaded.DefaultProfile, "test")
	}
	if len(loaded.Profiles) != 1 {
		t.Fatalf("Profiles count = %d, want 1", len(loaded.Profiles))
	}

	p := loaded.Profiles["test"]
	if p.AuthMethod != "api_key" {
		t.Errorf("AuthMethod = %q, want %q", p.AuthMethod, "api_key")
	}
	if p.Dir != cfg.Profiles["test"].Dir {
		t.Errorf("Dir = %q, want %q", p.Dir, cfg.Profiles["test"].Dir)
	}
}

func TestSaveAtomicWrite(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	cfg := &Config{Version: "1", Profiles: map[string]ProfileConfig{}}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify no .tmp file left behind
	configDir := filepath.Join(tmp, ".ccpm")
	entries, _ := os.ReadDir(configDir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("Temp file left behind: %s", e.Name())
		}
	}
}

func TestLoadCorruptedJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	// Write corrupt JSON
	configDir := filepath.Join(tmp, ".ccpm")
	os.MkdirAll(configDir, 0755)
	os.WriteFile(filepath.Join(configDir, "config.json"), []byte("{invalid"), 0644)

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should fail on corrupted JSON")
	}
}

func TestAddProfile(t *testing.T) {
	cfg := &Config{Version: "1", Profiles: map[string]ProfileConfig{}}
	cfg.AddProfile("work", "/home/user/.ccpm/profiles/work", "oauth")

	p, ok := cfg.Profiles["work"]
	if !ok {
		t.Fatal("Profile 'work' not found after AddProfile")
	}
	if p.AuthMethod != "oauth" {
		t.Errorf("AuthMethod = %q, want %q", p.AuthMethod, "oauth")
	}
	if p.CreatedAt == "" {
		t.Error("CreatedAt should be set")
	}
}

func TestRemoveProfile(t *testing.T) {
	cfg := &Config{
		Version:        "1",
		DefaultProfile: "work",
		Profiles: map[string]ProfileConfig{
			"work": {Name: "work"},
		},
	}

	cfg.RemoveProfile("work")

	if _, ok := cfg.Profiles["work"]; ok {
		t.Error("Profile should be removed")
	}
	if cfg.DefaultProfile != "" {
		t.Errorf("DefaultProfile should be cleared when default is removed, got %q", cfg.DefaultProfile)
	}
}

func TestUpdateLastUsed(t *testing.T) {
	cfg := &Config{
		Version: "1",
		Profiles: map[string]ProfileConfig{
			"test": {Name: "test", LastUsed: "2020-01-01T00:00:00Z"},
		},
	}

	cfg.UpdateLastUsed("test")

	p := cfg.Profiles["test"]
	if p.LastUsed == "2020-01-01T00:00:00Z" {
		t.Error("LastUsed should be updated")
	}
}

func TestEnsureDirs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs() error: %v", err)
	}

	for _, sub := range []string{"", "profiles", "vault"} {
		dir := filepath.Join(tmp, ".ccpm", sub)
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("Directory %q should exist: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%q should be a directory", dir)
		}
	}
}

func TestConfigJSONSerialization(t *testing.T) {
	cfg := &Config{
		Version:        "1",
		DefaultProfile: "",
		Profiles:       map[string]ProfileConfig{},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Verify fields are present in JSON (not omitted)
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	if _, ok := raw["version"]; !ok {
		t.Error("JSON should contain 'version' field")
	}
	if _, ok := raw["profiles"]; !ok {
		t.Error("JSON should contain 'profiles' field")
	}
}
