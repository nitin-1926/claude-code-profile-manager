//go:build !windows

package claude

import "syscall"

// Exec replaces the current process with claude so that signals and TTY pass
// through cleanly. Only available on Unix-like OSes.
func Exec(profileDir string, apiKey string, args []string) error {
	bin, _, env, err := execEnv(profileDir, apiKey)
	if err != nil {
		return err
	}
	argv := append([]string{bin}, args...)
	return syscall.Exec(bin, argv, env)
}
