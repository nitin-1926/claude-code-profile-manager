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
func Exec(profileDir string, apiKey string, args []string) error {
	bin, _, env, err := execEnv(profileDir, apiKey)
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
