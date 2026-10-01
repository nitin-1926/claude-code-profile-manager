//go:build windows

package atomicwrite

import (
	"errors"
	"syscall"
)

const errSharingViolation syscall.Errno = 32 // ERROR_SHARING_VIOLATION

// isTransientShareErr reports the errors Windows returns while another
// handle is open on the file: Go's os.Open never sets FILE_SHARE_DELETE, so a
// rename over a file someone is reading fails with ERROR_ACCESS_DENIED, and a
// read that lands mid-rename fails with ERROR_SHARING_VIOLATION. Both clear
// as soon as the other handle closes.
func isTransientShareErr(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	return errno == syscall.ERROR_ACCESS_DENIED || errno == errSharingViolation
}
