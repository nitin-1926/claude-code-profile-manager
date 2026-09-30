package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/credentials"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/keystore"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/lock"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/manifest"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/picker"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/profile"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
)

// recordingStore is an in-memory keystore that also records vault-key deletes.
type recordingStore struct {
	keystore.Store
	vaultDeleted bool
}

func (r *recordingStore) DeleteVaultMasterKey() error {
	r.vaultDeleted = true
	return r.Store.DeleteVaultMasterKey()
}

// lifecycle records every keychain / launchd side effect a command attempted.
type lifecycle struct {
	home          string
	store         *recordingStore
	oauth         map[string]string // profile dir -> keychain payload
	oauthDeleted  []string
	systemDefault []string // "set:<dir>" or "clear"
}

// lifecycleSandbox isolates HOME and swaps every side-effect seam for a
// recorder, so lifecycle commands can run end-to-end without touching the real
// keychain, launchd, or terminal.
func lifecycleSandbox(t *testing.T) *lifecycle {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := share.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	lc := &lifecycle{home: home, store: &recordingStore{Store: keystore.NewMemoryStore()}, oauth: map[string]string{}}

	saved := []func(){}
	swap := func(restore func()) { saved = append(saved, restore) }
	{
		a, b, c, d, e, f, g, h, i, j := newKeystore, readOAuthKeychain, writeOAuthKeychain, deleteOAuthKeychain, setSystemDefault, clearSystemDefault, selectOption, stdinIsTerminal, checkCredentials, readAnswer
		swap(func() {
			newKeystore, readOAuthKeychain, writeOAuthKeychain, deleteOAuthKeychain, setSystemDefault, clearSystemDefault, selectOption, stdinIsTerminal, checkCredentials, readAnswer = a, b, c, d, e, f, g, h, i, j
		})
	}
	fr, cn := forceRemove, cloneNoAuth
	swap(func() { forceRemove, cloneNoAuth = fr, cn })
	t.Cleanup(func() {
		for _, r := range saved {
			r()
		}
	})

	newKeystore = func() keystore.Store { return lc.store }
	readOAuthKeychain = func(dir string) (*credentials.MacKeychainOAuth, error) {
		if raw, ok := lc.oauth[dir]; ok {
			return &credentials.MacKeychainOAuth{Raw: raw, AccessToken: "tok"}, nil
		}
		return nil, nil
	}
	writeOAuthKeychain = func(dir, raw string) error { lc.oauth[dir] = raw; return nil }
	deleteOAuthKeychain = func(dir string) error {
		lc.oauthDeleted = append(lc.oauthDeleted, dir)
		delete(lc.oauth, dir)
		return nil
	}
	setSystemDefault = func(dir string) error { lc.systemDefault = append(lc.systemDefault, "set:"+dir); return nil }
	clearSystemDefault = func() error { lc.systemDefault = append(lc.systemDefault, "clear"); return nil }
	selectOption = func(string, []picker.Option) (string, error) { return "", picker.ErrNonInteractive }
	stdinIsTerminal = func() bool { return false }
	readAnswer = func() string { t.Fatal("unexpected confirmation prompt"); return "" }
	checkCredentials = func(string, string, string) credentials.CredStatus {
		t.Fatal("unexpected credential check")
		return credentials.CredStatus{}
	}
	return lc
}

// addProfile creates and registers a profile in the sandbox.
func (lc *lifecycle) addProfile(t *testing.T, name, auth string) string {
	t.Helper()
	dir, err := profile.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.AddProfile(name, dir, auth)
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	return dir
}

func (lc *lifecycle) setDefault(t *testing.T, name string) {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.DefaultProfile = name
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
}

// writeFragments gives name a settings fragment, owned sidecar and MCP fragment.
func writeFragments(t *testing.T, name string) {
	t.Helper()
	settingsDir, _ := share.SettingsDir()
	mcpDir, _ := share.MCPDir()
	writeFile(t, filepath.Join(settingsDir, name+".json"), `{"model":"`+name+`-model","env":{"TOKEN":"x"}}`)
	writeFile(t, filepath.Join(settingsDir, name+".owned.json"), `{"keys":["model"]}`)
	writeFile(t, filepath.Join(mcpDir, name+".json"), `{"`+name+`-server":{"command":"srv","env":{"API_TOKEN":"secret"}}}`)
}

func fragmentFiles(name string) []string {
	settingsDir, _ := share.SettingsDir()
	mcpDir, _ := share.MCPDir()
	return []string{
		filepath.Join(settingsDir, name+".json"),
		filepath.Join(settingsDir, name+".owned.json"),
		filepath.Join(mcpDir, name+".json"),
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func saveInstalls(t *testing.T, installs ...manifest.Install) {
	t.Helper()
	if err := manifest.Save(&manifest.Manifest{Version: "1", Installs: installs}); err != nil {
		t.Fatal(err)
	}
}

func loadInstalls(t *testing.T) []manifest.Install {
	t.Helper()
	m, err := manifest.Load()
	if err != nil {
		t.Fatal(err)
	}
	return m.Installs
}

func findInstall(installs []manifest.Install, id string) *manifest.Install {
	for i := range installs {
		if installs[i].ID == id {
			return &installs[i]
		}
	}
	return nil
}

// lockHeld reports whether ccpm's global config lock is currently held.
func lockHeld(t *testing.T) bool {
	t.Helper()
	path, err := config.LockPath()
	if err != nil {
		t.Fatal(err)
	}
	h, err := lock.Acquire(path, 50*time.Millisecond)
	if err != nil {
		return true
	}
	_ = h.Release()
	return false
}

func readJSONFile(t *testing.T, path string) map[string]interface{} {
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

// --- Finding 1: remove / rename must carry every per-profile store ---------

func TestRemove_DeletesFragmentsAndManifestRefs(t *testing.T) {
	lc := lifecycleSandbox(t)
	lc.addProfile(t, "old", "api_key")
	lc.addProfile(t, "other", "api_key")
	writeFragments(t, "old")
	saveInstalls(t,
		manifest.Install{ID: "solo", Kind: manifest.KindSkill, Scope: manifest.ScopeProfile, Profiles: []string{"old"}},
		manifest.Install{ID: "shared", Kind: manifest.KindSkill, Scope: manifest.ScopeProfile, Profiles: []string{"old", "other"}},
		manifest.Install{ID: "glob", Kind: manifest.KindSkill, Scope: manifest.ScopeGlobal, Profiles: []string{"old", "other"}},
	)
	forceRemove = true

	if err := runRemove(removeCmd, []string{"old"}); err != nil {
		t.Fatal(err)
	}

	for _, f := range fragmentFiles("old") {
		if exists(f) {
			t.Errorf("%s survived remove; a later `ccpm add old` would inherit it", f)
		}
	}
	installs := loadInstalls(t)
	if findInstall(installs, "solo") != nil {
		t.Error("profile-scoped entry owned only by the removed profile still in manifest")
	}
	for _, id := range []string{"shared", "glob"} {
		inst := findInstall(installs, id)
		if inst == nil || !slices.Equal(inst.Profiles, []string{"other"}) {
			t.Errorf("%s: want profiles [other], got %+v", id, inst)
		}
	}
}

func TestRename_MovesFragmentsAndManifestRefs(t *testing.T) {
	lc := lifecycleSandbox(t)
	lc.addProfile(t, "old", "api_key")
	if err := lc.store.SetAPIKey("old", "sk-ant-test"); err != nil {
		t.Fatal(err)
	}
	writeFragments(t, "old")
	saveInstalls(t, manifest.Install{ID: "solo", Kind: manifest.KindMCP, Scope: manifest.ScopeProfile, Profiles: []string{"old"}})

	if err := renameCmd.RunE(renameCmd, []string{"old", "new"}); err != nil {
		t.Fatal(err)
	}

	oldFiles, newFiles := fragmentFiles("old"), fragmentFiles("new")
	for i := range oldFiles {
		if exists(oldFiles[i]) {
			t.Errorf("%s left behind under the old name", oldFiles[i])
		}
		if !exists(newFiles[i]) {
			t.Errorf("%s missing after rename: settings/MCP lost", newFiles[i])
		}
	}
	if got := readJSONFile(t, newFiles[2]); got["old-server"] == nil {
		t.Errorf("MCP fragment content not carried over: %v", got)
	}
	if inst := findInstall(loadInstalls(t), "solo"); inst == nil || !slices.Equal(inst.Profiles, []string{"new"}) {
		t.Errorf("manifest ref not renamed: %+v", inst)
	}
}

// A fragment orphaned under the new name (e.g. by an older ccpm's remove) must
// not leak into the renamed profile when the old one has none.
func TestRename_DropsOrphanFragmentsUnderNewName(t *testing.T) {
	lc := lifecycleSandbox(t)
	lc.addProfile(t, "old", "api_key")
	if err := lc.store.SetAPIKey("old", "sk-ant-test"); err != nil {
		t.Fatal(err)
	}
	writeFragments(t, "new")

	if err := renameCmd.RunE(renameCmd, []string{"old", "new"}); err != nil {
		t.Fatal(err)
	}
	for _, f := range fragmentFiles("new") {
		if exists(f) {
			t.Errorf("orphan %s inherited by renamed profile", f)
		}
	}
}

// --- Finding 3: remove deletes the path-namespaced OAuth keychain entry ---

// `ccpm add <same name>` recreates the same dir → same keychain hash, so a
// surviving entry would log the new profile in as the old account.
func TestRemove_DeletesOAuthKeychainEntry(t *testing.T) {
	lc := lifecycleSandbox(t)
	dir := lc.addProfile(t, "old", "oauth")
	lc.oauth[dir] = `{"claudeAiOauth":{"accessToken":"old-account"}}`
	forceRemove = true

	if err := runRemove(removeCmd, []string{"old"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := lc.oauth[dir]; ok || !slices.Contains(lc.oauthDeleted, dir) {
		t.Fatalf("OAuth keychain entry for %s survived remove (deleted: %v)", dir, lc.oauthDeleted)
	}
}

// --- Finding 4: remove/rename/uninstall keep the system default consistent -

func TestRemove_DefaultOAuthProfileClearsSystemDefault(t *testing.T) {
	lc := lifecycleSandbox(t)
	lc.addProfile(t, "old", "oauth")
	lc.setDefault(t, "old")
	forceRemove = true

	if err := runRemove(removeCmd, []string{"old"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(lc.systemDefault, []string{"clear"}) {
		t.Fatalf("launchd CLAUDE_CONFIG_DIR still points at the deleted dir: %v", lc.systemDefault)
	}
}

func TestRemove_DefaultAPIKeyProfileStripsKeyFromHostSettings(t *testing.T) {
	lc := lifecycleSandbox(t)
	lc.addProfile(t, "old", "api_key")
	lc.setDefault(t, "old")
	hostSettings := filepath.Join(lc.home, ".claude", "settings.json")
	writeFile(t, hostSettings, `{"theme":"dark","env":{"ANTHROPIC_API_KEY":"sk-ant-removed"}}`)
	forceRemove = true

	if err := runRemove(removeCmd, []string{"old"}); err != nil {
		t.Fatal(err)
	}
	got := readJSONFile(t, hostSettings)
	if got["env"] != nil || got["theme"] != "dark" {
		t.Fatalf("removed default's API key left in ~/.claude/settings.json: %v", got)
	}
}

func TestRemove_NonDefaultLeavesSystemDefaultAlone(t *testing.T) {
	lc := lifecycleSandbox(t)
	lc.addProfile(t, "keep", "oauth")
	lc.addProfile(t, "old", "oauth")
	lc.setDefault(t, "keep")
	forceRemove = true

	if err := runRemove(removeCmd, []string{"old"}); err != nil {
		t.Fatal(err)
	}
	if len(lc.systemDefault) != 0 {
		t.Fatalf("system default touched when removing a non-default profile: %v", lc.systemDefault)
	}
}

func TestRename_DefaultOAuthProfileRepointsSystemDefault(t *testing.T) {
	lc := lifecycleSandbox(t)
	lc.addProfile(t, "old", "oauth")
	lc.setDefault(t, "old")

	if err := renameCmd.RunE(renameCmd, []string{"old", "new"}); err != nil {
		t.Fatal(err)
	}
	newDir, _ := profile.GetDir("new")
	if !slices.Equal(lc.systemDefault, []string{"set:" + newDir}) {
		t.Fatalf("launchd CLAUDE_CONFIG_DIR not re-pointed at %s: %v", newDir, lc.systemDefault)
	}
}

func TestUninstall_ClearsSystemDefaultOAuthEntriesAndVaultKey(t *testing.T) {
	lc := lifecycleSandbox(t)
	oauthDir := lc.addProfile(t, "work", "oauth")
	lc.addProfile(t, "keys", "api_key")
	lc.setDefault(t, "work")
	lc.oauth[oauthDir] = "payload"
	forceRemove = true

	if err := runUninstall(uninstallCmd, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := lc.oauth[oauthDir]; ok {
		t.Error("OAuth keychain entry survived uninstall")
	}
	if !slices.Contains(lc.systemDefault, "clear") {
		t.Errorf("launchd CLAUDE_CONFIG_DIR left pointing into the deleted ~/.ccpm: %v", lc.systemDefault)
	}
	if !lc.store.vaultDeleted {
		t.Error("vault master key not deleted, though the help says uninstall removes it")
	}
	if exists(filepath.Join(lc.home, ".ccpm")) {
		t.Error("~/.ccpm not removed")
	}
}

// --- Finding 2: clone / import from-profile carry profile-scoped state -----

// seedProfileScopedSkill links share/skills/<id> into dir/skills/<id> and
// records it as a profile-scoped install for name — the shape `ccpm skill add
// --profile` produces.
func seedProfileScopedSkill(t *testing.T, name, dir, id string) string {
	t.Helper()
	skills, _ := share.SkillsDir()
	storeEntry := filepath.Join(skills, id)
	writeFile(t, filepath.Join(storeEntry, "SKILL.md"), "# "+id)
	if err := share.Link(storeEntry, filepath.Join(dir, "skills", id)); err != nil {
		t.Fatal(err)
	}
	saveInstalls(t, manifest.Install{ID: id, Kind: manifest.KindSkill, Scope: manifest.ScopeProfile, Source: storeEntry, Profiles: []string{name}})
	return storeEntry
}

func TestClone_CarriesProfileScopedSkillsFragmentsAndManifestRefs(t *testing.T) {
	lc := lifecycleSandbox(t)
	srcDir := lc.addProfile(t, "old", "api_key")
	store := seedProfileScopedSkill(t, "old", srcDir, "my-skill")
	writeFragments(t, "old")
	cloneNoAuth = true

	if err := cloneCmd.RunE(cloneCmd, []string{"old", "c1"}); err != nil {
		t.Fatal(err)
	}

	c1Dir, _ := profile.GetDir("c1")
	if target, err := os.Readlink(filepath.Join(c1Dir, "skills", "my-skill")); err != nil || target != store {
		t.Errorf("profile-scoped skill not linked into clone: target=%q err=%v", target, err)
	}
	if inst := findInstall(loadInstalls(t), "my-skill"); inst == nil || !slices.Contains(inst.Profiles, "c1") {
		t.Errorf("clone not added to profile-scoped manifest entry: %+v", inst)
	}
	for _, f := range fragmentFiles("c1") {
		if !exists(f) {
			t.Errorf("%s not copied to clone", f)
		}
	}
}

// Every auto-adopted profile has share/ symlinks in its asset dirs; the strict
// copy refused them, so `import from-profile` failed in almost every setup.
func TestImportFromProfile_AcceptsSharedSymlinksAndCopiesStores(t *testing.T) {
	lc := lifecycleSandbox(t)
	srcDir := lc.addProfile(t, "work", "api_key")
	lc.addProfile(t, "play", "api_key")
	store := seedProfileScopedSkill(t, "work", srcDir, "my-skill")
	writeFragments(t, "work")

	state := &importFromProfileState{src: "work", target: "play", only: []string{"skills", "settings", "mcp"}}
	if err := runImportFromProfile(state); err != nil {
		t.Fatalf("import from-profile: %v", err)
	}

	playDir, _ := profile.GetDir("play")
	if target, err := os.Readlink(filepath.Join(playDir, "skills", "my-skill")); err != nil || target != store {
		t.Errorf("skill not linked: target=%q err=%v", target, err)
	}
	if inst := findInstall(loadInstalls(t), "my-skill"); inst == nil || !slices.Contains(inst.Profiles, "play") {
		t.Errorf("target not added to manifest entry: %+v", inst)
	}
	mcpDir, _ := share.MCPDir()
	if got := readJSONFile(t, filepath.Join(mcpDir, "play.json")); got["work-server"] == nil {
		t.Errorf("MCP fragment not copied: %v", got)
	}
}
