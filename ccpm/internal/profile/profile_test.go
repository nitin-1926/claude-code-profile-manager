package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"personal", false},
		{"work-acme", false},
		{"my_profile", false},
		{"Profile1", false},
		{"a", false},

		{"", true},                          // empty
		{"has space", true},                 // space
		{"../escape", true},                 // path traversal
		{"-starts-with-dash", true},         // must start alphanumeric
		{"_starts-with-underscore", true},    // must start alphanumeric
		{strings.Repeat("a", 33), true},     // too long
		{"hello!", true},                    // special char
		{"hello@world", true},               // special char
		{"hello/world", true},               // slash
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateName(%q) error = %v, wantErr = %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestGetDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	dir, err := GetDir("myprofile")
	if err != nil {
		t.Fatalf("GetDir() error: %v", err)
	}

	// Must be absolute
	if !filepath.IsAbs(dir) {
		t.Errorf("GetDir() returned non-absolute path: %s", dir)
	}

	// Must NOT contain ~/
	if strings.Contains(dir, "~/") {
		t.Errorf("GetDir() contains '~/' which Claude cannot handle: %s", dir)
	}

	// Must end with the profile name
	if filepath.Base(dir) != "myprofile" {
		t.Errorf("GetDir() base = %q, want %q", filepath.Base(dir), "myprofile")
	}
}

func TestCreateAndRemove(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	dir, err := Create("testprofile")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// Dir should exist
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Created dir should exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("Created path should be a directory")
	}

	// Exists should return true
	exists, err := Exists("testprofile")
	if err != nil {
		t.Fatalf("Exists() error: %v", err)
	}
	if !exists {
		t.Error("Exists() should return true after Create()")
	}

	// Duplicate create should fail
	_, err = Create("testprofile")
	if err == nil {
		t.Error("Create() should fail for existing profile")
	}

	// Remove
	if err := Remove("testprofile"); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}

	exists, _ = Exists("testprofile")
	if exists {
		t.Error("Exists() should return false after Remove()")
	}
}

func TestRemoveNonExistent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	err := Remove("doesnotexist")
	if err == nil {
		t.Error("Remove() should fail for non-existent profile")
	}
}

func TestPathNeverContainsTilde(t *testing.T) {
	// This is critical: Claude Code has a bug with ~/ paths.
	// We check that ccpm never introduces a tilde prefix. On Windows CI
	// the temp dir itself can contain ~ (e.g., RUNNER~1) due to 8.3
	// short names, so we only check the prefix.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	dir, err := GetDir("test")
	if err != nil {
		t.Fatalf("GetDir error: %v", err)
	}

	if strings.HasPrefix(dir, "~") {
		t.Errorf("Path starts with ~, this will break Claude Code: %s", dir)
	}
}
