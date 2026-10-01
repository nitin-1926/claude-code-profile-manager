package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

// sandboxHome points HOME/USERPROFILE at a fresh temp dir (and CWD too, so no
// project layer from the repo leaks in) and seeds the named profiles.
func sandboxHome(t *testing.T, profiles ...string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Chdir(t.TempDir())
	if err := config.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range profiles {
		cfg.AddProfile(name, seedProfileDir(t, name), "oauth")
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	return home
}

func seedProfileDir(t *testing.T, name string) string {
	t.Helper()
	profilesDir, err := config.ProfilesDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(profilesDir, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

// runCobra executes a fresh command tree with args, the way cobra would for a
// user, so the lock wiring in RunE is exercised.
func runCobra(t *testing.T, c *cobra.Command, args ...string) error {
	t.Helper()
	c.SetArgs(args)
	c.SilenceUsage = true
	c.SilenceErrors = true
	return c.Execute()
}

// registerProfileLocked simulates `ccpm clone`/`ccpm add` in parallel
// terminals: each registers a profile the correct way (locked, re-loaded).
func registerProfileLocked(t *testing.T, name string) {
	t.Helper()
	if err := withConfigLock(func() error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		cfg.AddProfile(name, filepath.Join(os.TempDir(), name), "oauth")
		return config.Save(cfg)
	}); err != nil {
		t.Error(err)
	}
}

func assertProfilesPresent(t *testing.T, names []string) {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	var lost []string
	for _, n := range names {
		if _, ok := cfg.Profiles[n]; !ok {
			lost = append(lost, n)
		}
	}
	if len(lost) > 0 {
		t.Fatalf("lost %d/%d concurrently registered profiles: %v", len(lost), len(names), lost)
	}
}

// TestEnvSetConcurrentKeepsEveryKey is the regression for `ccpm env set`
// saving config.json outside the lock: 40 parallel sets kept 1/40 keys.
func TestEnvSetConcurrentKeepsEveryKey(t *testing.T) {
	sandboxHome(t, "a")
	const n = 30
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := runCobra(t, newEnvCmd(), "set", fmt.Sprintf("K%d=v", i), "--profile", "a"); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(cfg.Profiles["a"].Env); got != n {
		t.Fatalf("kept %d/%d env keys: %v", got, n, cfg.Profiles["a"].Env)
	}

	// unset takes the same path.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := runCobra(t, newEnvCmd(), "unset", fmt.Sprintf("K%d", i), "--profile", "a"); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if cfg, _ = config.Load(); len(cfg.Profiles["a"].Env) != 0 {
		t.Fatalf("parallel unset left %v", cfg.Profiles["a"].Env)
	}
}

// TestRunLastUsedStampKeepsConcurrentRegistrations is the regression for the
// LastUsed stamp in `ccpm run` saving a stale config.json unlocked: a parallel
// `ccpm run` during `ccpm clone` erased the clone's registration.
func TestRunLastUsedStampKeepsConcurrentRegistrations(t *testing.T) {
	sandboxHome(t, "a")
	origExec := execClaude
	execClaude = func(string, string, map[string]string, map[string]string, []string) error { return nil }
	t.Cleanup(func() { execClaude = origExec })

	var names []string
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		name := fmt.Sprintf("clone%d", i)
		names = append(names, name)
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := runRun(runCmd, []string{"a"}); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			registerProfileLocked(t, name)
		}()
	}
	wg.Wait()
	assertProfilesPresent(t, names)
	if cfg, _ := config.Load(); cfg.Profiles["a"].LastUsed == "" {
		t.Fatal("run no longer stamps LastUsed")
	}
}

// TestUseLastUsedStampKeepsConcurrentRegistrations: same race via `ccpm use`,
// whose stamp ran after MaterializeAll and so held a stale config even longer.
func TestUseLastUsedStampKeepsConcurrentRegistrations(t *testing.T) {
	sandboxHome(t, "a")

	var names []string
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		name := fmt.Sprintf("clone%d", i)
		names = append(names, name)
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := runUse(useCmd, []string{"a"}); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			registerProfileLocked(t, name)
		}()
	}
	wg.Wait()
	assertProfilesPresent(t, names)
	if cfg, _ := config.Load(); cfg.Profiles["a"].LastUsed == "" {
		t.Fatal("use no longer stamps LastUsed")
	}
}
