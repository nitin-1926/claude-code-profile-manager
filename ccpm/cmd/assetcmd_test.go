package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/manifest"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
)

func TestAssetSpecPluralAndSubdir(t *testing.T) {
	cases := []struct {
		spec      AssetSpec
		wantPlur  string
		wantSubdir string
	}{
		{AssetSpec{Name: "agent"}, "agents", "agents"},
		{AssetSpec{Name: "rule"}, "rules", "rules"},
		{AssetSpec{Name: "command"}, "commands", "commands"},
		{AssetSpec{Name: "skill", Plural: "skills", ProfileSubdir: "skills"}, "skills", "skills"},
		{AssetSpec{Name: "ox", Plural: "oxen"}, "oxen", "oxen"},
	}
	for _, c := range cases {
		if got := c.spec.plural(); got != c.wantPlur {
			t.Errorf("plural() for %q = %q, want %q", c.spec.Name, got, c.wantPlur)
		}
		if got := c.spec.profileSubdir(); got != c.wantSubdir {
			t.Errorf("profileSubdir() for %q = %q, want %q", c.spec.Name, got, c.wantSubdir)
		}
	}
}

func TestFindStoreEntry(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	if err := share.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	agentsDir, _ := share.AgentsDir()
	// File-based entry: logical ID "foo", on-disk "foo.md".
	if err := os.WriteFile(filepath.Join(agentsDir, "foo.md"), []byte("..."), 0644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	// Directory-based entry: logical ID == basename.
	if err := os.MkdirAll(filepath.Join(agentsDir, "bar"), 0755); err != nil {
		t.Fatalf("seed dir: %v", err)
	}

	spec := AssetSpec{Name: "agent", Kind: manifest.KindAgent, SharedDir: share.AgentsDir}

	if got := findStoreEntry(spec, "foo"); got != "foo.md" {
		t.Errorf("findStoreEntry(foo) = %q, want foo.md", got)
	}
	if got := findStoreEntry(spec, "bar"); got != "bar" {
		t.Errorf("findStoreEntry(bar) = %q, want bar", got)
	}
	// Unknown ID falls back to the ID itself (caller will get a stat error later).
	if got := findStoreEntry(spec, "missing"); got != "missing" {
		t.Errorf("findStoreEntry(missing) = %q, want missing (fallback)", got)
	}
}

func TestTitleCase(t *testing.T) {
	cases := map[string]string{
		"":        "",
		"a":       "A",
		"agent":   "Agent",
		"command": "Command",
	}
	for in, want := range cases {
		if got := titleCase(in); got != want {
			t.Errorf("titleCase(%q) = %q, want %q", in, got, want)
		}
	}
}
