//go:build windows

package claude

import (
	"errors"
	"os"
	"os/exec"
)

// Exec runs claude as a child process on Windows. syscall.Exec is not
// available on Windows, so we spawn and propagate the exit code by calling
// os.Exit when the child terminates, preserving the "exec semantics" that
// callers expect (no code runs after Exec returns).
//
// profileEnv carries KEY=VALUE pairs persisted on the profile (e.g. via
// `ccpm env set`); extraEnv carries one-shot overrides from the caller (e.g.
// `ccpm run --env KEY=VAL`). Both may be nil.
func Exec(profileDir string, apiKey string, profileEnv, extraEnv map[string]string, args []string) error {
	bin, _, env, err := execEnv(profileDir, apiKey, profileEnv, extraEnv)
	if err != nil {
		return err
	}

	cmd := exec.Command(bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}

	os.Exit(0)
	return nil
}
