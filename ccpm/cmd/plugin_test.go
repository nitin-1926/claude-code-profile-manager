package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func writePluginsFile(t *testing.T, profileDir, body string) {
	t.Helper()
	dir := filepath.Join(profileDir, "plugins")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "installed_plugins.json"), []byte(body), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestLoadInstalledPlugins_MissingFile(t *testing.T) {
	got, err := loadInstalledPlugins(t.TempDir())
	if err != nil {
		t.Fatalf("loadInstalledPlugins() error = %v, want nil (missing is OK)", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty, got %+v", got)
	}
}

func TestLoadInstalledPlugins_RequiresProfileDir(t *testing.T) {
	if _, err := loadInstalledPlugins(""); err == nil {
		t.Fatal("expected error for empty profileDir")
	}
}

func TestLoadInstalledPlugins_V2Shape(t *testing.T) {
	tmp := t.TempDir()
	writePluginsFile(t, tmp, `{
		"version": 2,
		"plugins": {
			"vercel@claude-plugins-official": [
				{
					"scope": "user",
					"installPath": "/x/cache/claude-plugins-official/vercel/0.40.1",
					"version": "0.40.1",
					"installedAt": "2026-04-19T13:12:36.218Z",
					"lastUpdated": "2026-05-01T06:00:11.104Z",
					"gitCommitSha": "abc"
				}
			],
			"superpowers@claude-plugins-official": [
				{
					"scope": "user",
					"installPath": "/x/cache/claude-plugins-official/superpowers/5.0.7",
					"version": "5.0.7",
					"installedAt": "2026-05-02T08:16:26.806Z",
					"lastUpdated": "2026-05-02T08:16:26.806Z",
					"gitCommitSha": "def"
				}
			]
		}
	}`)

	got, err := loadInstalledPlugins(tmp)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 plugins, got %d (%+v)", len(got), got)
	}
	ids := []string{got[0].id(), got[1].id()}
	sort.Strings(ids)
	want := []string{"superpowers@claude-plugins-official", "vercel@claude-plugins-official"}
	if ids[0] != want[0] || ids[1] != want[1] {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	for _, p := range got {
		if p.Version == "" {
			t.Errorf("plugin %q missing version", p.id())
		}
	}
}

func TestLoadInstalledPlugins_V2PicksMostRecentEntry(t *testing.T) {
	tmp := t.TempDir()
	writePluginsFile(t, tmp, `{
		"version": 2,
		"plugins": {
			"vercel@claude-plugins-official": [
				{"version": "0.40.0", "installedAt": "2026-04-19T13:12:36.218Z", "lastUpdated": "2026-04-19T13:12:36.218Z"},
				{"version": "0.40.1", "installedAt": "2026-05-01T06:00:11.104Z", "lastUpdated": "2026-05-01T06:00:11.104Z"}
			]
		}
	}`)

	got, err := loadInstalledPlugins(tmp)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(got) != 1 || got[0].Version != "0.40.1" {
		t.Errorf("want most recent version 0.40.1, got %+v", got)
	}
}

func TestLoadInstalledPlugins_ArrayShape(t *testing.T) {
	tmp := t.TempDir()
	writePluginsFile(t, tmp,
		`[{"name":"vercel","marketplace":"claude-plugins-official","version":"0.40.0"}]`)

	got, err := loadInstalledPlugins(tmp)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(got) != 1 || got[0].id() != "vercel@claude-plugins-official" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestLoadInstalledPlugins_ObjectShape(t *testing.T) {
	tmp := t.TempDir()
	// Key already contains "@marketplace" — marketplace should be parsed out.
	writePluginsFile(t, tmp,
		`{"vercel@claude-plugins-official":{"version":"0.40.0"}}`)

	got, err := loadInstalledPlugins(tmp)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 plugin, got %d", len(got))
	}
	if got[0].Name != "vercel" || got[0].Marketplace != "claude-plugins-official" {
		t.Errorf("unexpected parse: %+v", got[0])
	}
}

func TestLoadInstalledPlugins_WrappedShape(t *testing.T) {
	tmp := t.TempDir()
	writePluginsFile(t, tmp,
		`{"installs":[{"name":"notion","marketplace":"notion-mkt"}]}`)

	got, err := loadInstalledPlugins(tmp)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(got) != 1 || got[0].id() != "notion@notion-mkt" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestInstalledPluginID(t *testing.T) {
	cases := []struct {
		in   installedPlugin
		want string
	}{
		{installedPlugin{Name: "vercel", Marketplace: "claude-plugins-official"}, "vercel@claude-plugins-official"},
		{installedPlugin{Name: "solo"}, "solo"},
	}
	for _, c := range cases {
		if got := c.in.id(); got != c.want {
			t.Errorf("id() = %q, want %q", got, c.want)
		}
	}
}
