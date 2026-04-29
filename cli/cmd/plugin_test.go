package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadInstalledPlugins_MissingFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	got, err := loadInstalledPlugins()
	if err != nil {
		t.Fatalf("loadInstalledPlugins() error = %v, want nil (missing is OK)", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty, got %+v", got)
	}
}

func TestLoadInstalledPlugins_ArrayShape(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	path := filepath.Join(tmp, ".claude", "plugins")
	os.MkdirAll(path, 0755)
	os.WriteFile(filepath.Join(path, "installed_plugins.json"),
		[]byte(`[{"name":"vercel","marketplace":"claude-plugins-official","version":"0.40.0"}]`), 0644)

	got, err := loadInstalledPlugins()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(got) != 1 || got[0].id() != "vercel@claude-plugins-official" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestLoadInstalledPlugins_ObjectShape(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	path := filepath.Join(tmp, ".claude", "plugins")
	os.MkdirAll(path, 0755)
	// Key already contains "@marketplace" — marketplace should be parsed out.
	os.WriteFile(filepath.Join(path, "installed_plugins.json"),
		[]byte(`{"vercel@claude-plugins-official":{"version":"0.40.0"}}`), 0644)

	got, err := loadInstalledPlugins()
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
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	path := filepath.Join(tmp, ".claude", "plugins")
	os.MkdirAll(path, 0755)
	os.WriteFile(filepath.Join(path, "installed_plugins.json"),
		[]byte(`{"installs":[{"name":"notion","marketplace":"notion-mkt"}]}`), 0644)

	got, err := loadInstalledPlugins()
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
