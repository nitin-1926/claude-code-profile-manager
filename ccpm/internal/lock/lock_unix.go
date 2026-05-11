//go:build !windows

package lock

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// tryLock attempts a non-blocking exclusive flock. Returns (true, nil) on
// success, (false, nil) if another process holds the lock, or (false, err) on
// an unexpected error.
func tryLock(f *os.File) (bool, error) {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, unix.EWOULDBLOCK) {
		return false, nil
	}
	return false, err
}

func unlock(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}
