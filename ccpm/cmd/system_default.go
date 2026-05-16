package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// System-default integration: make `ccpm set-default <profile>` the *single
// source of truth* for every `claude` invocation on the machine — terminal,
// IDE extensions (Cursor, VSCode, Antigravity), and any other GUI app that
// spawns claude as a subprocess.
//
// The mechanism on macOS is the user's launchd session. `launchctl setenv`
// sets an environment variable that every subsequently-launched process
// belonging to this user inherits (GUI or terminal). We pair that immediate
// effect with a user-level LaunchAgent plist so the same env var is re-applied
// at every login — surviving reboots.
//
// Why we have to do this:
//   - Claude Code v2.1.117 through v2.1.139 all contain a startup-refresh
//     path that 401s when CLAUDE_CONFIG_DIR resolves to bare ~/.claude, even
//     when the keychain default slot is correctly synced. Same binary with
//     CLAUDE_CONFIG_DIR pointing at a profile-namespaced directory works.
//   - IDE extensions bundle their own claude binary inside the extension dir;
//     they don't go through PATH and can't be intercepted with a shell
//     wrapper. The only knob that affects them all uniformly is the env they
//     inherit from launchd.
//
// Behavior:
//   - macOS: setSystemDefaultConfigDir writes
//     ~/Library/LaunchAgents/com.ccpm.default-config-dir.plist with the chosen
//     profile dir and calls `launchctl setenv` so the change takes effect for
//     newly-launched apps without requiring a reboot.
//     clearSystemDefaultConfigDir undoes both.
//   - Linux / Windows: no-op for now. (Linux equivalents would live in
//     ~/.config/environment.d/ + systemctl --user import-environment, or
//     pam_env config. Windows equivalents would be `setx`. Both are filed as
//     follow-ups; the shell wrapper already covers terminal use on those
//     platforms.)
//
// Failure mode: best-effort. If launchctl or the plist write fails, we warn
// but do not block the set-default. The keychain + ~/.claude.json identity
// sync is the primary correctness mechanism; the launchctl layer is the
// workaround for the upstream Claude Code bug, and not having it just means
// the user falls back to using ccpm-wrapped invocations.

const systemDefaultAgentLabel = "com.ccpm.default-config-dir"

// setSystemDefaultConfigDir installs the LaunchAgent and updates the current
// session's env so every claude launched after this call inherits
// CLAUDE_CONFIG_DIR=profileDir. macOS only; no-op elsewhere.
func setSystemDefaultConfigDir(profileDir string) error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	plistPath, err := systemDefaultPlistPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(plistPath), 0755); err != nil {
		return fmt.Errorf("creating LaunchAgents dir: %w", err)
	}
	content := buildSystemDefaultPlist(profileDir)
	if err := os.WriteFile(plistPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", plistPath, err)
	}
	// Apply to the current launchd session so the change is visible to
	// newly-launched GUI apps immediately, without requiring logout.
	if out, err := exec.Command("/bin/launchctl", "setenv", "CLAUDE_CONFIG_DIR", profileDir).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl setenv: %w (output: %s)", err, out)
	}
	return nil
}

// clearSystemDefaultConfigDir removes the LaunchAgent plist and unsets the
// current session's CLAUDE_CONFIG_DIR. macOS only; no-op elsewhere.
//
// Idempotent: missing plist and "env var not set" both count as success.
func clearSystemDefaultConfigDir() error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	plistPath, err := systemDefaultPlistPath()
	if err != nil {
		return err
	}
	// Best-effort unsetenv first; ignore errors since the var may already be
	// unset. The exec failure modes are noisy and not actionable.
	_ = exec.Command("/bin/launchctl", "unsetenv", "CLAUDE_CONFIG_DIR").Run()
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing %s: %w", plistPath, err)
	}
	return nil
}

func systemDefaultPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", systemDefaultAgentLabel+".plist"), nil
}

// buildSystemDefaultPlist returns the LaunchAgent property-list XML for a
// daemon that runs `launchctl setenv CLAUDE_CONFIG_DIR <profileDir>` at login.
// RunAtLoad=true ensures the env var is set every time the user logs in;
// KeepAlive is intentionally absent because `launchctl setenv` is a one-shot
// command that exits immediately.
//
// profileDir is encoded with minimal XML escaping (& < > are the only chars
// that could appear in a filesystem path on macOS).
func buildSystemDefaultPlist(profileDir string) string {
	escaped := xmlEscape(profileDir)
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>` + systemDefaultAgentLabel + `</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/launchctl</string>
        <string>setenv</string>
        <string>CLAUDE_CONFIG_DIR</string>
        <string>` + escaped + `</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
</dict>
</plist>
`
}

func xmlEscape(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			out = append(out, []byte("&amp;")...)
		case '<':
			out = append(out, []byte("&lt;")...)
		case '>':
			out = append(out, []byte("&gt;")...)
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}
