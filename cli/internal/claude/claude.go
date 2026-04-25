package claude

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// FindBinary resolves the claude executable. Discovery order is tuned to
// prefer high-trust locations first and fall back to less-trusted ones:
//
//  1. CLAUDE_BINARY — explicit override. If set but missing, warn loudly
//     instead of silently falling through (L9 — otherwise a typo in a shell
//     rc file silently shipped the user to whichever claude PATH happened
//     to resolve).
//  2. PATH via exec.LookPath — Go's stdlib refuses "."-relative execs since
//     Go 1.19, so PATH poisoning via a cwd entry is not exploitable.
//  3. macOS system-managed app bundle (darwin only).
//  4. Homebrew locations.
//  5. /usr/local/bin.
//  6. ~/.npm-global/bin.
//  7. nvm node versions — intentionally last. Any npm package installed in
//     any nvm version can drop a `claude` binary there; we reach it only
//     when every higher-trust location is absent (F7).
func FindBinary() (string, error) {
	if bin := os.Getenv("CLAUDE_BINARY"); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return bin, nil
		}
		fmt.Fprintf(os.Stderr, "Warning: CLAUDE_BINARY=%q does not exist; falling back to PATH discovery.\n", bin)
	}

	if path, err := exec.LookPath("claude"); err == nil {
		return path, nil
	}

	home, _ := os.UserHomeDir()
	var commonPaths []string

	if home != "" && runtime.GOOS == "darwin" {
		claudeCodeDir := filepath.Join(home, "Library", "Application Support", "Claude", "claude-code")
		if entries, err := os.ReadDir(claudeCodeDir); err == nil {
			// Try versions in reverse order (newest first).
			for i := len(entries) - 1; i >= 0; i-- {
				candidate := filepath.Join(claudeCodeDir, entries[i].Name(), "claude.app", "Contents", "MacOS", "claude")
				commonPaths = append(commonPaths, candidate)
			}
		}
	}

	commonPaths = append(commonPaths, "/opt/homebrew/bin/claude", "/usr/local/bin/claude")

	if home != "" {
		commonPaths = append(commonPaths, filepath.Join(home, ".npm-global", "bin", "claude"))

		if runtime.GOOS == "windows" {
			appData := os.Getenv("APPDATA")
			if appData != "" {
				commonPaths = append(commonPaths, filepath.Join(appData, "npm", "claude.cmd"))
			}
		}

		// nvm fallback comes last — it enumerates every installed node
		// version, and any of those could have a shadowed binary.
		nvmDir := filepath.Join(home, ".nvm", "versions", "node")
		if entries, err := os.ReadDir(nvmDir); err == nil {
			for _, e := range entries {
				commonPaths = append(commonPaths, filepath.Join(nvmDir, e.Name(), "bin", "claude"))
			}
		}
	}

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

// execEnv builds the env slice for launching claude with a given profile.
//
// Precedence (later wins): parent process env → profile-persisted env
// (ProfileConfig.Env) → CLAUDE_CONFIG_DIR → ANTHROPIC_API_KEY → ad-hoc CLI
// overrides (extraEnv). CLAUDE_CONFIG_DIR and ANTHROPIC_API_KEY always come
// from ccpm so a stray value in the parent or profile env can't redirect the
// launch at the ccpm layer.
func execEnv(profileDir, apiKey string, profileEnv, extraEnv map[string]string) (bin, absProfileDir string, env []string, err error) {
	bin, err = FindBinary()
	if err != nil {
		return "", "", nil, err
	}

	absProfileDir, err = filepath.Abs(profileDir)
	if err != nil {
		return "", "", nil, fmt.Errorf("resolving profile path: %w", err)
	}

	env = os.Environ()
	env = appendEnvMap(env, profileEnv)
	env = append(env, fmt.Sprintf("CLAUDE_CONFIG_DIR=%s", absProfileDir))
	if apiKey != "" {
		env = append(env, fmt.Sprintf("ANTHROPIC_API_KEY=%s", apiKey))
	}
	env = appendEnvMap(env, extraEnv)
	return bin, absProfileDir, env, nil
}

// appendEnvMap writes KEY=VALUE pairs to the end of env in a stable order so
// test output (and `ccpm run -v` debug traces) stay deterministic. Later
// entries beat earlier ones because Go's exec path honors the last occurrence
// of a key.
func appendEnvMap(env []string, m map[string]string) []string {
	if len(m) == 0 {
		return env
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = append(env, fmt.Sprintf("%s=%s", k, m[k]))
	}
	return env
}

// Version runs `<bin> --version` and returns the first line of output,
// trimmed. Returns an empty string if the binary is not found or fails.
func Version() string {
	bin, err := FindBinary()
	if err != nil {
		return ""
	}
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(out))
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		line = line[:idx]
	}
	return line
}
