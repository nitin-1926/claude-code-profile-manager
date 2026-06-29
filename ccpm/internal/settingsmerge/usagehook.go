package settingsmerge

import (
	"fmt"
	"path/filepath"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
)

// UsageSyncCommand is the command the injected SessionEnd hook runs to keep a
// profile's usage store warm. ccpm must be on PATH — it is, since the session
// was launched via `ccpm run` (same assumption as DefaultStatusLineCommand).
const UsageSyncCommand = "ccpm usage sync"

// EnsureUsageHook injects a SessionEnd hook running `ccpm usage sync` into the
// profile's settings fragment when no equivalent hook is already present, and
// returns true when it wrote one.
//
// Unlike EnsureDefaultStatusLine, it does not defer to a host/managed layer:
// hooks are additive, so a user's own SessionEnd hooks coexist with ours. It is
// idempotent — it only checks for OUR command, so a second call short-circuits,
// making it safe to call on every `ccpm run`.
func EnsureUsageHook(profileName string) (bool, error) {
	shareDir, err := share.SettingsDir()
	if err != nil {
		return false, err
	}
	fragPath := filepath.Join(shareDir, profileName+".json")

	frag, err := LoadJSON(fragPath)
	if err != nil {
		return false, fmt.Errorf("loading profile settings fragment: %w", err)
	}
	if usageHookPresent(frag) {
		return false, nil
	}

	if err := share.EnsureDirs(); err != nil {
		return false, err
	}

	hooksRoot, _ := frag["hooks"].(map[string]interface{})
	if hooksRoot == nil {
		hooksRoot = map[string]interface{}{}
		frag["hooks"] = hooksRoot
	}
	events, _ := hooksRoot["SessionEnd"].([]interface{})
	events = append(events, map[string]interface{}{
		"matcher": "",
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": UsageSyncCommand,
			},
		},
	})
	hooksRoot["SessionEnd"] = events

	if err := WriteJSON(fragPath, frag); err != nil {
		return false, fmt.Errorf("writing fragment: %w", err)
	}
	// Mark owned so the materialize step re-asserts our hook on every launch.
	if err := MarkOwned(fragPath, "hooks.SessionEnd"); err != nil {
		return false, fmt.Errorf("recording owned key: %w", err)
	}
	return true, nil
}

// usageHookPresent reports whether the fragment already carries a SessionEnd
// hook running UsageSyncCommand, so we never inject a duplicate.
func usageHookPresent(frag map[string]interface{}) bool {
	hooksRoot, ok := frag["hooks"].(map[string]interface{})
	if !ok {
		return false
	}
	events, ok := hooksRoot["SessionEnd"].([]interface{})
	if !ok {
		return false
	}
	for _, e := range events {
		em, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		hooks, ok := em["hooks"].([]interface{})
		if !ok {
			continue
		}
		for _, h := range hooks {
			if hm, ok := h.(map[string]interface{}); ok {
				if cmd, _ := hm["command"].(string); cmd == UsageSyncCommand {
					return true
				}
			}
		}
	}
	return false
}
