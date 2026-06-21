package cmd

import (
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/lock"
)

// Lock policies — every command that touches shared state picks ONE of these
// three, so the "never block a launch" and "never lose a shared-state update"
// goals don't silently fight each other:
//
//	must-lock          credential/config mutations (add, remove, rename,
//	                   set-default, asset add/remove, import execute, sync).
//	                   Wrap with lockedRunE or withConfigLock and FAIL on a lock
//	                   error — the mutation must not proceed unserialized.
//	best-effort-lock   `ccpm run` prelaunch: serialize the shared-state writes
//	                   (host adoption, statusLine injection) but on a lock error
//	                   SKIP them with a warning rather than run unlocked. The
//	                   profile-local materialize then always runs unlocked.
//	read-only-preview  dry-runs: never take the lock at all (no writes happen).
//
// withConfigLock runs fn while holding ccpm's global advisory lock, serializing
// credential- and config-mutating commands against each other so concurrent
// invocations (e.g. `ccpm set-default` in one terminal while `ccpm rename` runs
// in another) can't interleave a stale read with a fresh write and corrupt
// config.json / installs.json / the keychain default slot.
//
// It is deliberately coarse-grained: the operations it guards are fast and
// infrequent, so a single process-wide lock is simpler and safer than
// per-file locks. Long-running work (notably launching `claude` in `ccpm run`)
// must NOT be wrapped — only the credential/config mutation that precedes it.
func withConfigLock(fn func() error) error {
	lockPath, err := config.LockPath()
	if err != nil {
		return err
	}
	slog.Debug("acquiring config lock", "path", lockPath)
	defer slog.Debug("released config lock", "path", lockPath)
	return lock.Guard(lockPath, lock.DefaultTimeout, fn)
}

// lockedRunE wraps a cobra RunE so the whole command runs under the global
// advisory lock. Use it for short, fully-mutating commands (set-default,
// rename, remove, add). Do NOT use it for commands that exec a long-running
// child like `claude` — those must lock only their mutation phase.
func lockedRunE(fn func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return withConfigLock(func() error { return fn(cmd, args) })
	}
}
