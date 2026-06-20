package cmd

import "fmt"

// Exit codes form ccpm's scripting contract. Documented here as the single
// source of truth:
//
//	0 — success (warnings may have been printed to stderr)
//	1 — command failed (default for any returned error, incl. cobra flag/arg
//	    parse failures)
//	2 — usage error (RESERVED — not yet wired. cobra flag/arg parse failures
//	    currently still exit 1; a command opts a specific path into 2 by
//	    returning exitWithCode(exitUsage, err). Kept in the contract so the
//	    code is reserved, not so scripts can rely on it today.)
//	3 — partial failure: the command did some of its work but at least one
//	    item failed (used by `ccpm import` and `ccpm sync`)
//	4 — health check found issues (ccpm doctor)
//
// Rule of thumb: warnings never affect the exit code; errors always do.
const (
	exitOK             = 0
	exitErr            = 1
	exitUsage          = 2
	exitPartialFailure = 3
	exitUnhealthy      = 4
)

// codedError carries a specific process exit code through cobra's error
// return path. Execute unwraps it; plain errors keep exiting 1.
type codedError struct {
	code int
	err  error
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

// exitWithCode wraps err so Execute exits with the given code. A nil err
// yields nil (no-op) so call sites can pass through their error value.
func exitWithCode(code int, err error) error {
	if err == nil {
		return nil
	}
	return &codedError{code: code, err: err}
}

// partialFailure builds a coded error for commands that completed some work
// before failing — scripts can distinguish "nothing happened" (1) from
// "some items landed" (3).
func partialFailure(format string, args ...interface{}) error {
	return &codedError{code: exitPartialFailure, err: fmt.Errorf(format, args...)}
}
