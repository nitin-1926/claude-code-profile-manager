package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/credentials"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
)

const testProviderKey = "sk-or-v1-test-provider-key"

// runAddPreset runs `ccpm add <name> --preset <id>` with stdin as piped input.
func runAddPreset(t *testing.T, name, id, stdin string) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin")
	writeFile(t, path, stdin)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	savedStdin, savedPreset := os.Stdin, addPresetID
	os.Stdin, addPresetID = f, id
	t.Cleanup(func() { os.Stdin, addPresetID = savedStdin, savedPreset })
	return runAdd(addCmd, []string{name})
}

func loadProfileConfig(t *testing.T, name string) config.ProfileConfig {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Profiles[name]
}

func TestAddPresetKeepsKeyOutOfConfig(t *testing.T) {
	lc := lifecycleSandbox(t)
	// A ~/.claude makes a plain add run the import wizard, which would read the
	// piped key as its menu answer. A preset add must not.
	if err := os.MkdirAll(filepath.Join(lc.home, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := runAddPreset(t, "router", "openrouter", testProviderKey+"\n"); err != nil {
		t.Fatal(err)
	}

	p := loadProfileConfig(t, "router")
	if p.AuthMethod != "api_key" || p.KeyEnv != "ANTHROPIC_AUTH_TOKEN" || p.Preset != "openrouter" {
		t.Errorf("profile = %+v", p)
	}
	if got := p.Env["ANTHROPIC_BASE_URL"]; got != "https://openrouter.ai/api" {
		t.Errorf("base URL = %q", got)
	}
	if got, err := lc.store.GetAPIKey("router"); err != nil || got != testProviderKey {
		t.Errorf("keystore = %q, %v", got, err)
	}
	raw, err := os.ReadFile(filepath.Join(lc.home, ".ccpm", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), testProviderKey) {
		t.Error("provider key reached config.json")
	}
	cfg, _ := config.Load()
	if cfg.DefaultProfile != "" {
		t.Errorf("first provider profile became the default: %q", cfg.DefaultProfile)
	}
}

func TestAddPresetWithoutKeyStoresThePlaceholder(t *testing.T) {
	lc := lifecycleSandbox(t)
	if err := runAddPreset(t, "local", "ollama", ""); err != nil {
		t.Fatal(err)
	}
	if got, err := lc.store.GetAPIKey("local"); err != nil || got != "ollama" {
		t.Errorf("keystore = %q, %v", got, err)
	}
	if got := loadProfileConfig(t, "local").Env["ANTHROPIC_BASE_URL"]; got != "http://localhost:11434" {
		t.Errorf("base URL = %q", got)
	}
}

func TestAddUnknownPresetListsValidOnesAndTouchesNothing(t *testing.T) {
	lc := lifecycleSandbox(t)
	err := runAddPreset(t, "x", "nope", "")
	if err == nil || !strings.Contains(err.Error(), "openrouter") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(lc.home, ".ccpm", "profiles", "x")); !os.IsNotExist(statErr) {
		t.Errorf("profile dir created (stat err %v)", statErr)
	}
}

func TestAddPresetBadKeyRollsBack(t *testing.T) {
	lc := lifecycleSandbox(t)
	err := runAddPreset(t, "router", "openrouter", "short\n")
	if err == nil || !strings.Contains(err.Error(), "too short") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(lc.home, ".ccpm", "profiles", "router")); !os.IsNotExist(statErr) {
		t.Errorf("failed add left its dir behind (stat err %v)", statErr)
	}
	if cfg, _ := config.Load(); len(cfg.Profiles) != 0 {
		t.Errorf("failed add registered a profile: %v", cfg.Profiles)
	}
}

// addProvider registers a provider profile with a stored key.
func (lc *lifecycle) addProvider(t *testing.T, name string) string {
	t.Helper()
	if err := runAddPreset(t, name, "openrouter", testProviderKey+"\n"); err != nil {
		t.Fatal(err)
	}
	return loadProfileConfig(t, name).Dir
}

// The whole launch path: shell and host settings both carry a real Anthropic
// credential, and neither may survive into what claude is handed.
func TestRunProviderProfile(t *testing.T) {
	lc := lifecycleSandbox(t)
	dir := lc.addProvider(t, "router")
	writeFile(t, filepath.Join(lc.home, ".claude", "settings.json"),
		`{"env":{"ANTHROPIC_API_KEY":"sk-ant-host","ANTHROPIC_BASE_URL":"https://host.example"}}`)

	saved := execClaude
	t.Cleanup(func() { execClaude = saved })
	var gotKeyEnv, gotKey string
	var gotEnv map[string]string
	execClaude = func(_, keyEnv, key string, env, _ map[string]string, _ []string) error {
		gotKeyEnv, gotKey, gotEnv = keyEnv, key, env
		return nil
	}

	if err := runRun(runCmd, []string{"router"}); err != nil {
		t.Fatal(err)
	}
	if gotKeyEnv != "ANTHROPIC_AUTH_TOKEN" || gotKey != testProviderKey {
		t.Errorf("launched with key env %q, key %q", gotKeyEnv, gotKey)
	}
	if gotEnv["ANTHROPIC_BASE_URL"] != "https://openrouter.ai/api" {
		t.Errorf("profile env = %v", gotEnv)
	}
	settings, err := settingsmerge.LoadJSON(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if env, _ := settings["env"].(map[string]interface{}); env["ANTHROPIC_API_KEY"] != nil || env["ANTHROPIC_BASE_URL"] != nil {
		t.Errorf("host credentials reached the profile's settings.json: %v", env)
	}
}

func TestRunProviderWithoutBaseURLRefuses(t *testing.T) {
	lc := lifecycleSandbox(t)
	lc.addProvider(t, "router")
	cfg, _ := config.Load()
	p := cfg.Profiles["router"]
	delete(p.Env, "ANTHROPIC_BASE_URL")
	cfg.Profiles["router"] = p
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	saved := execClaude
	t.Cleanup(func() { execClaude = saved })
	execClaude = func(string, string, string, map[string]string, map[string]string, []string) error {
		t.Fatal("launched a provider key with no endpoint")
		return nil
	}
	if err := runRun(runCmd, []string{"router"}); err == nil || !strings.Contains(err.Error(), "ANTHROPIC_BASE_URL") {
		t.Fatalf("err = %v", err)
	}
	// A one-shot endpoint is an endpoint.
	launched := false
	execClaude = func(string, string, string, map[string]string, map[string]string, []string) error {
		launched = true
		return nil
	}
	if err := runRun(runCmd, []string{"--ccpm-env", "ANTHROPIC_BASE_URL=https://x.example", "router"}); err != nil || !launched {
		t.Fatalf("one-shot endpoint: launched=%v err=%v", launched, err)
	}
}

// set-default writes to the host settings every profile inherits, so a provider
// default would route all of them through the provider.
func TestSetDefaultRefusesProviderProfile(t *testing.T) {
	lc := lifecycleSandbox(t)
	lc.addProvider(t, "router")
	err := applySetDefault("router")
	if err == nil || !strings.Contains(err.Error(), "ccpm run router") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(lc.home, ".claude", "settings.json")); !os.IsNotExist(statErr) {
		t.Errorf("host settings written (stat err %v)", statErr)
	}
	if len(lc.systemDefault) != 0 || len(lc.oauthDeleted) != 0 {
		t.Errorf("side effects ran: %v %v", lc.systemDefault, lc.oauthDeleted)
	}
	if cfg, _ := config.Load(); cfg.DefaultProfile != "" {
		t.Errorf("default = %q", cfg.DefaultProfile)
	}
}

func TestUseRefusesProviderProfile(t *testing.T) {
	lc := lifecycleSandbox(t)
	lc.addProvider(t, "router")
	if err := runUse(useCmd, []string{"router"}); err == nil || !strings.Contains(err.Error(), "ccpm run router") {
		t.Fatalf("err = %v", err)
	}
}

func TestEnvSetReservesKeyEnvOnProviderProfilesOnly(t *testing.T) {
	lc := lifecycleSandbox(t)
	lc.addProvider(t, "router")
	lc.addProfile(t, "gateway", "oauth")

	err := runEnvSet(&envState{profile: "router"}, []string{"ANTHROPIC_AUTH_TOKEN=plaintext"})
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("err = %v", err)
	}
	if got := loadProfileConfig(t, "router").Env["ANTHROPIC_AUTH_TOKEN"]; got != "" {
		t.Errorf("token stored in config: %q", got)
	}
	// Repointing a provider is allowed: Ollama may live on another machine.
	if err := runEnvSet(&envState{profile: "router"}, []string{"ANTHROPIC_BASE_URL=http://gpu:11434"}); err != nil {
		t.Errorf("base URL override refused: %v", err)
	}
	if err := runEnvSet(&envState{profile: "gateway"}, []string{"ANTHROPIC_AUTH_TOKEN=gw"}); err != nil {
		t.Errorf("ordinary profile refused its gateway token: %v", err)
	}
}

// Clone copies the key, so it must copy the endpoint too, or the provider's key
// launches against Anthropic.
func TestCloneCarriesProviderEndpoint(t *testing.T) {
	lc := lifecycleSandbox(t)
	lc.addProvider(t, "router")
	if err := runClone(cloneCmd, []string{"router", "copy"}); err != nil {
		t.Fatal(err)
	}
	p := loadProfileConfig(t, "copy")
	if p.KeyEnv != "ANTHROPIC_AUTH_TOKEN" || p.Preset != "openrouter" || p.Env["ANTHROPIC_BASE_URL"] != "https://openrouter.ai/api" {
		t.Errorf("clone = %+v", p)
	}
	if got, err := lc.store.GetAPIKey("copy"); err != nil || got != testProviderKey {
		t.Errorf("clone key = %q, %v", got, err)
	}
}

// doctor must flag the one state `ccpm run` refuses to launch.
func TestDoctorFlagsProviderWithoutBaseURL(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub claude binary is a shell script")
	}
	lc := lifecycleSandbox(t)
	lc.addProvider(t, "router")
	stub := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\necho 9.9.9\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_BINARY", stub)
	checkCredentials = func(string, string, string) credentials.CredStatus { return credentials.CredStatus{Valid: true} }

	if err := runDoctor(doctorCmd, nil); err != nil {
		t.Fatalf("healthy provider profile: %v", err)
	}

	cfg, _ := config.Load()
	p := cfg.Profiles["router"]
	delete(p.Env, "ANTHROPIC_BASE_URL")
	cfg.Profiles["router"] = p
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := runDoctor(doctorCmd, nil); err == nil {
		t.Fatal("doctor passed a provider profile with no endpoint")
	}
}

func TestAuthRefreshRestoresPlaceholderWithoutPrompting(t *testing.T) {
	lc := lifecycleSandbox(t)
	if err := runAddPreset(t, "local", "ollama", ""); err != nil {
		t.Fatal(err)
	}
	if err := lc.store.DeleteAPIKey("local"); err != nil {
		t.Fatal(err)
	}
	if err := runAuthRefresh(authRefreshCmd, []string{"local"}); err != nil {
		t.Fatal(err)
	}
	if got, err := lc.store.GetAPIKey("local"); err != nil || got != "ollama" {
		t.Errorf("keystore = %q, %v", got, err)
	}
}

// Claude Code's own session cost uses Anthropic's rates, which are meaningless
// for another endpoint, so only an Anthropic-bound profile shows it.
func TestStatusLineHidesCostOnCustomEndpoint(t *testing.T) {
	lc := lifecycleSandbox(t)
	lc.addProvider(t, "router")
	lc.addProfile(t, "plain", "api_key")
	const payload = `{"model":{"display_name":"m"},"cost":{"total_cost_usd":1.5}}`
	if out := renderVia(t, "router", payload); strings.Contains(out, "$") {
		t.Errorf("provider status line shows a cost: %q", out)
	}
	if out := renderVia(t, "plain", payload); !strings.Contains(out, "$1.50") {
		t.Errorf("ordinary status line lost its cost: %q", out)
	}
}
