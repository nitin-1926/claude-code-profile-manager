//go:build !windows

package claude

import "syscall"

// Exec replaces the current process with claude so that signals and TTY pass
// through cleanly. Only available on Unix-like OSes.
//
// profileEnv carries KEY=VALUE pairs persisted on the profile (e.g. via
// `ccpm env set`); extraEnv carries one-shot overrides from the caller (e.g.
// `ccpm run --env KEY=VAL`). Both may be nil.
func Exec(profileDir string, apiKey string, profileEnv, extraEnv map[string]string, args []string) error {
	bin, _, env, err := execEnv(profileDir, apiKey, profileEnv, extraEnv)
	if err != nil {
		return err
	}
	argv := append([]string{bin}, args...)
	return syscall.Exec(bin, argv, env)
}
