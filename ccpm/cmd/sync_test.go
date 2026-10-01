package cmd

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/lock"
)

// TestSyncDryRunWithNoProfilesErrors: `ccpm sync --all --dry-run` with no
// profiles indexed targets[0] and panicked.
func TestSyncDryRunWithNoProfilesErrors(t *testing.T) {
	out, code := runCCPMForTest(t, "sync --all --dry-run")
	if code != exitErr || strings.Contains(out, "panic") || !strings.Contains(out, "no profiles") {
		t.Fatalf("exit %d, want %d with a 'no profiles' error; output:\n%s", code, exitErr, out)
	}
}

// TestSyncSkipsProfileRemovedBeforeLock: sync read config before the picker
// and the lock, then synced every chosen name from that stale copy. A profile
// removed in between (by `ccpm remove` in another terminal, while the picker
// was open) was resurrected on disk: its directory re-created and populated.
func TestSyncSkipsProfileRemovedBeforeLock(t *testing.T) {
	sandboxHome(t, "a", "b")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	removedDir := cfg.Profiles["a"].Dir

	oldProfile, oldAll, oldDry := syncProfile, syncAll, syncDryRun
	t.Cleanup(func() { syncProfile, syncAll, syncDryRun = oldProfile, oldAll, oldDry })
	syncProfile, syncAll, syncDryRun = "", true, false

	lockPath, err := config.LockPath()
	if err != nil {
		t.Fatal(err)
	}
	h, err := lock.Acquire(lockPath, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var syncErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		syncErr = runSync(syncCmd, nil)
	}()
	// Let sync read config and block on the lock, then remove "a" the way
	// `ccpm remove` does while holding it.
	time.Sleep(300 * time.Millisecond)
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	delete(cfg.Profiles, "a")
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(removedDir); err != nil {
		t.Fatal(err)
	}
	_ = h.Release()
	wg.Wait()

	if syncErr != nil {
		t.Fatalf("sync: %v", syncErr)
	}
	if _, err := os.Stat(removedDir); !os.IsNotExist(err) {
		t.Fatalf("sync re-created removed profile dir %s (stat err %v)", removedDir, err)
	}
	// An empty Dir for the missing name would sync into CWD instead.
	if entries, _ := os.ReadDir("."); len(entries) != 0 {
		t.Fatalf("sync wrote %d entries into CWD (e.g. %s)", len(entries), entries[0].Name())
	}
	if cfg, _ := config.Load(); len(cfg.Profiles) != 1 {
		t.Fatalf("profiles after sync = %v, want only b", config.ProfileNames(cfg))
	}
}
