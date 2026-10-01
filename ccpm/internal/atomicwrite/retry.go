package atomicwrite

import (
	"os"
	"time"
)

// retryWindow bounds how long a Windows sharing conflict is ridden out.
// ponytail: fixed 2s budget with backoff, the same policy cmd/go's robustio
// uses; raise it only if real contention ever outlasts it.
const retryWindow = 2 * time.Second

// retry runs fn until it succeeds, fails with an error that is not a
// transient Windows sharing conflict, or retryWindow elapses.
func retry(fn func() error) error {
	deadline := time.Now().Add(retryWindow)
	delay := time.Millisecond
	for {
		err := fn()
		if err == nil || !isTransientShareErr(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(delay)
		if delay < 100*time.Millisecond {
			delay *= 2
		}
	}
}

// rename is os.Rename that survives another process reading the target on
// Windows (see isTransientShareErr). Elsewhere it is exactly os.Rename.
func rename(oldpath, newpath string) error {
	return retry(func() error { return os.Rename(oldpath, newpath) })
}

// ReadFile is os.ReadFile that survives a concurrent Apply renaming over the
// same path on Windows. Use it for files other processes rewrite with Apply,
// such as config.json.
func ReadFile(path string) ([]byte, error) {
	var data []byte
	err := retry(func() error {
		var err error
		data, err = os.ReadFile(path)
		return err
	})
	return data, err
}
