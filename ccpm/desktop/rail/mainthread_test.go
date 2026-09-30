//go:build darwin

package rail

import "runtime"

// Pins the main goroutine to the main thread, so every test runs off it, as on
// a CI runner. Without this, whether a test hit the main thread was luck, and a
// main-queue wait that only deadlocks off the main thread passed locally while
// hanging CI for ten minutes.
func init() { runtime.LockOSThread() }
