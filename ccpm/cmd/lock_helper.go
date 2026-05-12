package cmd

import (
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/lock"
)

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
