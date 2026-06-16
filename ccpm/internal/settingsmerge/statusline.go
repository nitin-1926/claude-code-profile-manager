package settingsmerge

import (
	"fmt"
	"path/filepath"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
)

// DefaultStatusLineCommand is the statusLine shell command ccpm injects into a
// profile that has none, so a launched Claude Code session shows the active
// profile (and, for subscription accounts, usage/limit windows) in its TUI.
// It points at `ccpm statusline`, which reads Claude Code's status JSON on
// stdin and renders the line. ccpm must be on PATH — it is, since that's how
// the session was launched.
const DefaultStatusLineCommand = "ccpm statusline"

// EnsureDefaultStatusLine writes a default statusLine into the profile's ccpm
// settings fragment when no statusLine is configured at the profile, host
// (~/.claude/settings.json), or managed layer. It returns true when it wrote a
// new statusLine.
//
// It never overwrites an existing statusLine — a user/host/managed one always
// wins, and a trusted project's statusLine still beats this in the final merge
// (project layer has higher precedence than the profile fragment). It is
// idempotent: once it has written and marked the key owned, subsequent calls
// short-circuit on the profile-fragment check. That idempotence is what makes
// `ccpm run`'s auto-injection safe to call on every launch.
func EnsureDefaultStatusLine(profileName, command string) (bool, error) {
	shareDir, err := share.SettingsDir()
	if err != nil {
		return false, err
	}
	fragPath := filepath.Join(shareDir, profileName+".json")

	frag, err := LoadJSON(fragPath)
	if err != nil {
		return false, fmt.Errorf("loading profile settings fragment: %w", err)
	}
	if _, ok := frag["statusLine"]; ok {
		// Profile already has one — user-set or previously injected by ccpm.
		return false, nil
	}

	host, err := loadHostClaudeSettings()
	if err != nil {
		return false, fmt.Errorf("loading host ~/.claude/settings.json: %w", err)
	}
	if _, ok := host["statusLine"]; ok {
		// Respect the user's global statusLine.
		return false, nil
	}

	managed, err := LoadManagedSettings()
	if err != nil {
		return false, fmt.Errorf("loading managed settings: %w", err)
	}
	if _, ok := managed["statusLine"]; ok {
		// Respect an admin-published statusLine.
		return false, nil
	}

	if err := share.EnsureDirs(); err != nil {
		return false, err
	}
	frag["statusLine"] = map[string]interface{}{
		"type":    "command",
		"command": command,
	}
	if err := WriteJSON(fragPath, frag); err != nil {
		return false, fmt.Errorf("writing fragment: %w", err)
	}
	// Mark owned so the materialize step re-asserts our statusLine even if a
	// lower layer (host) later sets a different one — matching how
	// `ccpm settings statusline` records the key.
	if err := MarkOwned(fragPath, "statusLine"); err != nil {
		return false, fmt.Errorf("recording owned key: %w", err)
	}
	return true, nil
}
