package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadNonExistent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	m, err := Load()
	if err != nil {
		t.Fatalf("Load() should succeed for missing manifest: %v", err)
	}
	if m.Version != manifestVersion {
		t.Errorf("Version = %q, want %q", m.Version, manifestVersion)
	}
	if len(m.Installs) != 0 {
		t.Errorf("Installs should be empty, got %d", len(m.Installs))
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	os.MkdirAll(filepath.Join(tmp, ".ccpm"), 0755)

	m := &Manifest{Version: manifestVersion}
	m.Add(Install{
		ID:       "test-skill",
		Kind:     KindSkill,
		Scope:    ScopeGlobal,
		Source:   "/path/to/skill",
		Profiles: []string{"work", "personal"},
	})

	if err := Save(m); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(loaded.Installs) != 1 {
		t.Fatalf("expected 1 install, got %d", len(loaded.Installs))
	}

	inst := loaded.Installs[0]
	if inst.ID != "test-skill" {
		t.Errorf("ID = %q, want test-skill", inst.ID)
	}
	if inst.Kind != KindSkill {
		t.Errorf("Kind = %q, want skill", inst.Kind)
	}
	if inst.Scope != ScopeGlobal {
		t.Errorf("Scope = %q, want global", inst.Scope)
	}
	if inst.CreatedAt == "" {
		t.Error("CreatedAt should be set")
	}
}

func TestAddAndRemove(t *testing.T) {
	m := &Manifest{Version: manifestVersion}

	m.Add(Install{ID: "a", Kind: KindSkill, Scope: ScopeGlobal})
	m.Add(Install{ID: "b", Kind: KindMCP, Scope: ScopeProfile})
	m.Add(Install{ID: "c", Kind: KindSkill, Scope: ScopeProfile})

	if len(m.Installs) != 3 {
		t.Fatalf("expected 3 installs, got %d", len(m.Installs))
	}

	removed := m.Remove("b", KindMCP)
	if !removed {
		t.Error("Remove should return true for existing install")
	}
	if len(m.Installs) != 2 {
		t.Fatalf("expected 2 installs after remove, got %d", len(m.Installs))
	}

	removed = m.Remove("nonexistent", KindSkill)
	if removed {
		t.Error("Remove should return false for non-existent install")
	}
}

func TestFind(t *testing.T) {
	m := &Manifest{Version: manifestVersion}
	m.Add(Install{ID: "github", Kind: KindMCP, Scope: ScopeGlobal})

	found := m.Find("github", KindMCP)
	if found == nil {
		t.Fatal("Find should return the install")
	}
	if found.ID != "github" {
		t.Errorf("ID = %q, want github", found.ID)
	}

	notFound := m.Find("github", KindSkill)
	if notFound != nil {
