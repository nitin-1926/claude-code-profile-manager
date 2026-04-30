package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nitin-1926/ccpm/internal/manifest"
	"github.com/nitin-1926/ccpm/internal/share"
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
