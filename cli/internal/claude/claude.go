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
