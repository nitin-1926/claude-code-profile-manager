//go:build darwin

package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// setProfileEnv sets a profile's `ccpm env set` vars in config.json.
func setProfileEnv(t *testing.T, name string, env map[string]string) {
	t.Helper()
	path := filepath.Join(os.Getenv("HOME"), ".ccpm", "config.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg["profiles"].(map[string]any)[name].(map[string]any)["env"] = env
	if b, err = json.Marshal(cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// compareFixture builds two profiles in a scratch HOME that differ in every
// category, from the real on-disk shapes: assets as symlinks into ~/.claude
// and ~/.ccpm/share, settings fragments under share/settings, MCP in
// ~/.claude.json and share/mcp, launch env in config.json.
func compareFixture(t *testing.T) (left, right string) {
	t.Helper()
	left = syntheticProfile(t)
	right = "beta"
	addSyntheticProfile(t, right)
	t.Cleanup(settingsmerge.SetManagedSettingsDirForTest(t.TempDir()))

	home := os.Getenv("HOME")
	ccpm := filepath.Join(home, ".ccpm")
	lp, rp := filepath.Join(ccpm, "profiles", left), filepath.Join(ccpm, "profiles", right)

	// skills: shared from host in both, a global one and a profile-local one only left
	writeFile(t, filepath.Join(home, ".claude", "skills", "shared", "SKILL.md"), "x")
	writeFile(t, filepath.Join(ccpm, "share", "skills", "g", "SKILL.md"), "x")
	symlink(t, filepath.Join(home, ".claude", "skills", "shared"), filepath.Join(lp, "skills", "shared"))
	symlink(t, filepath.Join(home, ".claude", "skills", "shared"), filepath.Join(rp, "skills", "shared"))
	symlink(t, filepath.Join(ccpm, "share", "skills", "g"), filepath.Join(lp, "skills", "g"))
	writeFile(t, filepath.Join(lp, "skills", "local-only", "SKILL.md"), "x")
	// the plugin store is not a list of plugins and must not become rows
	writeFile(t, filepath.Join(lp, "plugins", "installed_plugins.json"), `{"version":2,"plugins":{}}`)

	// Profile-local copies are compared by content: their paths always differ.
	writeFile(t, filepath.Join(lp, "hooks", "heal.mjs"), "export {}\n")
	writeFile(t, filepath.Join(rp, "hooks", "heal.mjs"), "export {}\n")
	for _, p := range []string{lp, rp} {
		writeFile(t, filepath.Join(p, "skills", "copied", "SKILL.md"), "same")
		writeFile(t, filepath.Join(p, "skills", "copied", "scripts", "run.sh"), "echo")
	}
	writeFile(t, filepath.Join(lp, "skills", "synced", "SKILL.md"), "v1")
	writeFile(t, filepath.Join(rp, "skills", "synced", "SKILL.md"), "v2")
	// A symlink on one side, an identical local copy on the other.
	writeFile(t, filepath.Join(home, ".claude", "hooks", "linked.sh"), "echo hi\n")
	symlink(t, filepath.Join(home, ".claude", "hooks", "linked.sh"), filepath.Join(lp, "hooks", "linked.sh"))
	writeFile(t, filepath.Join(rp, "hooks", "linked.sh"), "echo hi\n")
	// Two symlinks compare by target, even when the targets hold the same bytes.
	writeFile(t, filepath.Join(home, ".claude", "skills", "twin", "SKILL.md"), "same")
	writeFile(t, filepath.Join(ccpm, "share", "skills", "twin", "SKILL.md"), "same")
	symlink(t, filepath.Join(home, ".claude", "skills", "twin"), filepath.Join(lp, "skills", "twin"))
	symlink(t, filepath.Join(ccpm, "share", "skills", "twin"), filepath.Join(rp, "skills", "twin"))

	long := strings.Repeat("a", 200)
	writeFile(t, filepath.Join(home, ".claude", "settings.json"),
		`{"model":"sonnet","permissions":{"allow":["Bash(ls)"]}}`)
	writeFile(t, filepath.Join(ccpm, "share", "settings", left+".json"), `{
		"model": "opus",
		"longKey": "`+long+`x",
		"permissions": {"deny": ["Bash(rm)"], "defaultMode": "plan"},
		"enabledPlugins": {"p@m": true},
		"env": {"API_TOKEN": "sk-secret-left"}
	}`)
	writeFile(t, filepath.Join(ccpm, "share", "settings", right+".json"), `{
		"longKey": "`+long+`y",
		"permissions": {"allow": ["Bash(rm)"], "defaultMode": "plan"},
		"enabledPlugins": {"p@m": false},
		"env": {"API_TOKEN": "sk-secret-right"}
	}`)
	setProfileEnv(t, left, map[string]string{"LAUNCH_VAR": "launch-secret"})

	writeFile(t, filepath.Join(home, ".claude.json"), `{"numStartups": 3, "mcpServers": {
		"gh": {"command": "npx", "args": ["--token", "ghp_argsecret"], "env": {"GH_TOKEN": "ghp_envsecret"}},
		"remote": {"type": "http", "url": "https://u:pwsecret@api.example.com/mcp?key=urlsecret", "headers": {"Authorization": "Bearer hdrsecret"}}
	}}`)
	writeFile(t, filepath.Join(ccpm, "share", "mcp", right+".json"), `{"beta-only": {"command": "x"}}`)
	return left, right
}

func rowsByKey(c *Comparison) map[string]CompareRow {
	out := map[string]CompareRow{}
	for _, cat := range c.Categories {
		for _, r := range cat.Rows {
			out[cat.ID+"/"+r.Name] = r
		}
	}
	return out
}

func TestCompareRows(t *testing.T) {
	left, right := compareFixture(t)
	c, err := NewCompare().Compare(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if c.Message != "" {
		t.Fatalf("Message = %q for two known profiles", c.Message)
	}
	got := rowsByKey(c)
	want := map[string]CompareRow{
		"skill/shared":           {Status: StatusSame, LeftLayer: LayerHost, RightLayer: LayerHost},
		"skill/g":                {Status: StatusOnlyLeft, LeftLayer: LayerGlobal},
		"skill/local-only":       {Status: StatusOnlyLeft, LeftLayer: LayerProfile},
		"hook/heal.mjs":          {Status: StatusSame, LeftLayer: LayerProfile, RightLayer: LayerProfile},
		"skill/copied":           {Status: StatusSame, LeftLayer: LayerProfile, RightLayer: LayerProfile},
		"skill/synced":           {Status: StatusChanged, LeftLayer: LayerProfile, RightLayer: LayerProfile},
		"hook/linked.sh":         {Status: StatusSame, LeftLayer: LayerHost, RightLayer: LayerProfile},
		"skill/twin":             {Status: StatusChanged, LeftLayer: LayerHost, RightLayer: LayerGlobal},
		"permission/Bash(ls)":    {Status: StatusOnlyLeft, Left: "allow", LeftLayer: LayerHost},
		"permission/Bash(rm)":    {Status: StatusChanged, Left: "deny", LeftLayer: LayerGlobal, Right: "allow", RightLayer: LayerGlobal},
		"permission/defaultMode": {Status: StatusSame, Left: "plan", LeftLayer: LayerGlobal, Right: "plan", RightLayer: LayerGlobal},
		"setting/model":          {Status: StatusChanged, Left: `"opus"`, LeftLayer: LayerGlobal, Right: `"sonnet"`, RightLayer: LayerHost},
		"setting/longKey":        {Status: StatusChanged, LeftLayer: LayerGlobal, RightLayer: LayerGlobal},
		"plugin/p@m":             {Status: StatusChanged, Left: "true", LeftLayer: LayerGlobal, Right: "false", RightLayer: LayerGlobal},
		"env/API_TOKEN":          {Status: StatusChanged, Left: "sk-secret-left", LeftLayer: LayerGlobal, Right: "sk-secret-right", RightLayer: LayerGlobal},
		"env/LAUNCH_VAR":         {Status: StatusOnlyLeft, Left: "launch-secret", LeftLayer: LayerProfile},
		"mcp/gh":                 {Status: StatusSame, Left: "npx · 2 args · env: GH_TOKEN", LeftLayer: LayerHost, Right: "npx · 2 args · env: GH_TOKEN", RightLayer: LayerHost},
		"mcp/remote":             {Status: StatusSame, Left: "http · https://api.example.com/mcp?… · headers: Authorization", LeftLayer: LayerHost, Right: "http · https://api.example.com/mcp?… · headers: Authorization", RightLayer: LayerHost},
		"mcp/beta-only":          {Status: StatusOnlyRight, Right: "x", RightLayer: LayerProfile},
	}
	for key, w := range want {
		g, ok := got[key]
		if !ok {
			t.Errorf("%s: missing row", key)
			continue
		}
		if g.Status != w.Status || g.LeftLayer != w.LeftLayer || g.RightLayer != w.RightLayer {
			t.Errorf("%s: status=%s layers=%q/%q, want %s %q/%q", key, g.Status, g.LeftLayer, g.RightLayer, w.Status, w.LeftLayer, w.RightLayer)
		}
		if (w.Left != "" && g.Left != w.Left) || (w.Right != "" && g.Right != w.Right) {
			t.Errorf("%s: values %q | %q, want %q | %q", key, g.Left, g.Right, w.Left, w.Right)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d rows, want %d: %v", len(got), len(want), got)
	}
	for _, cat := range c.Categories {
		if cat.Secret != (cat.ID == "env") {
			t.Errorf("category %s: secret=%v", cat.ID, cat.Secret)
		}
	}
}

// Secrets reach the frontend only as env values, which the UI masks until a
// reveal. MCP definitions are summarized, never sent whole.
func TestCompareSendsNoMCPSecrets(t *testing.T) {
	left, right := compareFixture(t)
	c, err := NewCompare().Compare(left, right)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(c)
	for _, s := range []string{"ghp_argsecret", "ghp_envsecret", "pwsecret", "urlsecret", "hdrsecret"} {
		if strings.Contains(string(b), s) {
			t.Errorf("%q reached the frontend", s)
		}
	}
	var nonEnv []CompareCategory
	for _, cat := range c.Categories {
		if cat.ID != "env" {
			nonEnv = append(nonEnv, cat)
		}
	}
	b, _ = json.Marshal(nonEnv)
	for _, s := range []string{"sk-secret-left", "sk-secret-right", "launch-secret"} {
		if strings.Contains(string(b), s) {
			t.Errorf("env value %q appears outside the masked env category", s)
		}
	}
}

func TestCompareSelfIsAllSame(t *testing.T) {
	left, _ := compareFixture(t)
	c, err := NewCompare().Compare(left, left)
	if err != nil {
		t.Fatal(err)
	}
	rows := rowsByKey(c)
	if len(rows) == 0 {
		t.Fatal("self-compare produced no rows")
	}
	for key, r := range rows {
		if r.Status != StatusSame || r.Left != r.Right || r.LeftLayer != r.RightLayer {
			t.Errorf("%s: %+v, want identical sides", key, r)
		}
	}
}

func TestCompareUnknownProfile(t *testing.T) {
	name := syntheticProfile(t)
	cases := map[string][2]string{
		"right unknown": {name, "ghost"},
		"left unknown":  {"ghost", name},
		"both unknown":  {"ghost", "phantom"},
		"self unknown":  {"ghost", "ghost"},
	}
	for label, pair := range cases {
		t.Run(label, func(t *testing.T) {
			c, err := NewCompare().Compare(pair[0], pair[1])
			if err != nil {
				t.Fatalf("err = %v, want a message instead", err)
			}
			if c.Categories == nil || len(c.Categories) != 0 {
				t.Errorf("categories = %v, want empty and non-nil", c.Categories)
			}
			assertNoNullArrays(t, c, "categories")
			for _, p := range pair {
				if p != name && !strings.Contains(c.Message, `"`+p+`"`) {
					t.Errorf("message %q does not name %q", c.Message, p)
				}
			}
			if strings.Count(c.Message, `"ghost"`) > 1 {
				t.Errorf("message names ghost twice: %q", c.Message)
			}
		})
	}
}

// A malformed settings.json is an error, with a safe DTO alongside it.
func TestCompareMalformedSettingsIsAnError(t *testing.T) {
	name := syntheticProfile(t)
	addSyntheticProfile(t, "beta")
	breakProfileSettings(t, name)
	c, err := NewCompare().Compare("beta", name)
	if err == nil || !strings.Contains(err.Error(), "settings.json") {
		t.Errorf("err = %v, want the settings.json parse error", err)
	}
	if c == nil {
		t.Fatal("nil DTO alongside the error")
	}
	assertNoNullArrays(t, c, "categories")
}
