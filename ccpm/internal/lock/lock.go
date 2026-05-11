// Package lock provides a cross-platform advisory file lock used to serialize
// ccpm's credential- and config-mutating operations.
//
// Why this exists: commands like `set-default`, `rename`, `add`, `remove`, and
// the credential save-back logic perform read-modify-write sequences on shared
// state (~/.ccpm/config.json, installs.json, the OS keychain's default slot,
// ~/.claude.json). Two ccpm processes racing on these — e.g. `ccpm run a` in
// one terminal while `ccpm set-default b` runs in another — can interleave a
// stale read with a fresh write and clobber a valid refresh token, producing
// the exact 401/re-login failures the surrounding code works hard to avoid.
//
// An OS advisory lock is the right primitive here: it is released
// automatically when the holding process dies (unlike a hand-rolled lockfile,
// which leaves a stale lock after a crash), and it is cheap. The lock is
// advisory — it only coordinates ccpm-with-ccpm, which is all we need.
package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DefaultTimeout bounds how long Acquire waits for a contended lock before
// giving up. Generous enough to ride out another ccpm command finishing, short
// enough that a wedged process doesn't hang the user indefinitely.
const DefaultTimeout = 15 * time.Second

// pollInterval is how often Acquire retries the non-blocking lock attempt.
const pollInterval = 50 * time.Millisecond

// Handle represents an acquired advisory lock. Call Release (or Close) to drop
// it; the lock is also released if the process exits.
type Handle struct {
	f *os.File
}

// Acquire opens (creating if needed) lockPath and takes an exclusive advisory
// lock on it, polling until it succeeds or timeout elapses.
func Acquire(lockPath string, timeout time.Duration) (*Handle, error) {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return nil, fmt.Errorf("lock: creating lock directory: %w", err)
	}
	// 0600: the lockfile carries no secrets, but keep it owner-only to match
	// the rest of ~/.ccpm.
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock: opening lock file %q: %w", lockPath, err)
	}

	deadline := time.Now().Add(timeout)
	for {
		ok, err := tryLock(f)
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("lock: acquiring %q: %w", lockPath, err)
		}
		if ok {
			return &Handle{f: f}, nil
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("lock: timed out after %s waiting for %q (another ccpm command may be running)", timeout, lockPath)
		}
		time.Sleep(pollInterval)
	}
}

// Release drops the lock and closes the underlying file. Safe to call once.
func (h *Handle) Release() error {
	if h == nil || h.f == nil {
		return nil
	}
	err := unlock(h.f)
	closeErr := h.f.Close()
	h.f = nil
	if err != nil {
		return err
	}
	return closeErr
}

// Guard runs fn while holding an exclusive advisory lock on lockPath, releasing
// it afterward regardless of how fn returns.
func Guard(lockPath string, timeout time.Duration, fn func() error) error {
	h, err := Acquire(lockPath, timeout)
	if err != nil {
		return err
	}
	defer func() { _ = h.Release() }()
	return fn()
}
