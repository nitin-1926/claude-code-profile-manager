package claude

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
)

func FindBinary() (string, error) {
	// 1. CLAUDE_BINARY env var
	if bin := os.Getenv("CLAUDE_BINARY"); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return bin, nil
		}
	}

	// 2. PATH lookup
	if path, err := exec.LookPath("claude"); err == nil {
		return path, nil
	}

	// 3. Common paths
	home, _ := os.UserHomeDir()
	var commonPaths []string

	if home != "" {
		// macOS desktop app (highest priority after PATH)
		if runtime.GOOS == "darwin" {
			claudeCodeDir := filepath.Join(home, "Library", "Application Support", "Claude", "claude-code")
			if entries, err := os.ReadDir(claudeCodeDir); err == nil {
				// Try versions in reverse order (newest first)
				for i := len(entries) - 1; i >= 0; i-- {
					candidate := filepath.Join(claudeCodeDir, entries[i].Name(), "claude.app", "Contents", "MacOS", "claude")
					commonPaths = append(commonPaths, candidate)
				}
			}
		}

		// nvm paths
		nvmDir := filepath.Join(home, ".nvm", "versions", "node")
		if entries, err := os.ReadDir(nvmDir); err == nil {
			for _, e := range entries {
				commonPaths = append(commonPaths, filepath.Join(nvmDir, e.Name(), "bin", "claude"))
			}
		}
		// npm global
		commonPaths = append(commonPaths, filepath.Join(home, ".npm-global", "bin", "claude"))

		if runtime.GOOS == "windows" {
			appData := os.Getenv("APPDATA")
			if appData != "" {
				commonPaths = append(commonPaths, filepath.Join(appData, "npm", "claude.cmd"))
			}
		}
	}

	commonPaths = append(commonPaths, "/usr/local/bin/claude", "/opt/homebrew/bin/claude")

	for _, p := range commonPaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("claude binary not found. Install with: npm i -g @anthropic-ai/claude-code")
}

// Spawn runs claude as a child process and waits for it to exit.
// Used during `ccpm add` for the OAuth login flow.
func Spawn(profileDir string, extraEnv ...string) error {
	bin, err := FindBinary()
	if err != nil {
		return err
	}

	abs, err := filepath.Abs(profileDir)
	if err != nil {
		return fmt.Errorf("resolving profile path: %w", err)
	}

	cmd := exec.Command(bin)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), fmt.Sprintf("CLAUDE_CONFIG_DIR=%s", abs))
	cmd.Env = append(cmd.Env, extraEnv...)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("claude exited with error: %w", err)
	}
	return nil
}

// Exec replaces the current process with claude.
// Used for `ccpm run` so signals and TTY pass through cleanly.
func Exec(profileDir string, apiKey string, args []string) error {
	bin, err := FindBinary()
	if err != nil {
		return err
	}

	abs, err := filepath.Abs(profileDir)
	if err != nil {
		return fmt.Errorf("resolving profile path: %w", err)
	}

	env := os.Environ()
	env = append(env, fmt.Sprintf("CLAUDE_CONFIG_DIR=%s", abs))
	if apiKey != "" {
		env = append(env, fmt.Sprintf("ANTHROPIC_API_KEY=%s", apiKey))
	}

	argv := append([]string{bin}, args...)
	return syscall.Exec(bin, argv, env)
}
