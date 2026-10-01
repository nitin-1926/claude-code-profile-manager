//go:build !windows

package atomicwrite

// isTransientShareErr is always false off Windows: POSIX rename replaces a
// file other processes have open, and reads never conflict with it.
func isTransientShareErr(error) bool { return false }
