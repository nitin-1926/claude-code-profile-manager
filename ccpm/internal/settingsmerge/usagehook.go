package settingsmerge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
)

// UsageSyncCommand is the command the injected SessionEnd hook runs to keep a
// profile's usage store warm. ccpm must be on PATH — it is, since the session
// was launched via `ccpm run` (same assumption as DefaultStatusLineCommand).
const UsageSyncCommand = "ccpm usage sync"

// EnsureUsageHook makes the profile's settings fragment carry a SessionEnd
// hook running `ccpm usage sync`, and returns true when it changed the file.
//
// The fragment OWNS hooks.SessionEnd — materialize re-asserts it over the host
// layer, and DeepMerge replaces arrays rather than concatenating them. So the
// fragment must carry the host's SessionEnd entries too; a fragment holding
// only our hook silently erases every SessionEnd hook the user declared in
// ~/.claude/settings.json. The array is therefore re-derived from the host
// layer on every call (this runs on each `ccpm run`), which also keeps it
// current when the user edits their host settings later.
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
	host, err := loadHostClaudeSettings()
	if err != nil {
		return false, fmt.Errorf("loading host ~/.claude/settings.json: %w", err)
	}

	want := desiredSessionEnd(frag, host)
	if equalJSON(sessionEndEntries(frag), want) {
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
	hooksRoot["SessionEnd"] = want

	if err := WriteJSON(fragPath, frag); err != nil {
		return false, fmt.Errorf("writing fragment: %w", err)
	}
	// Mark owned so the materialize step re-asserts our hook on every launch.
	if err := MarkOwned(fragPath, "hooks.SessionEnd"); err != nil {
		return false, fmt.Errorf("recording owned key: %w", err)
	}
	return true, nil
}

// RemoveUsageHook drops our SessionEnd entry from the fragment, keeping every
// other entry, and releases ownership of hooks.SessionEnd once nothing is left.
// Without this, `ccpm config set usage_tracking false` stopped future injection
// but the already-written hook kept firing forever.
func RemoveUsageHook(profileName string) (bool, error) {
	shareDir, err := share.SettingsDir()
	if err != nil {
		return false, err
	}
	fragPath := filepath.Join(shareDir, profileName+".json")

	frag, err := LoadJSON(fragPath)
	if err != nil {
		return false, fmt.Errorf("loading profile settings fragment: %w", err)
	}
	hooksRoot, _ := frag["hooks"].(map[string]interface{})
	if hooksRoot == nil {
		return false, nil
	}
	kept := make([]interface{}, 0, len(sessionEndEntries(frag)))
	for _, e := range sessionEndEntries(frag) {
		if !entryRunsUsageSync(e) {
			kept = append(kept, e)
		}
	}
	if equalJSON(sessionEndEntries(frag), kept) {
		return false, nil
	}
	if len(kept) == 0 {
		delete(hooksRoot, "SessionEnd")
		if len(hooksRoot) == 0 {
			delete(frag, "hooks")
		}
		if err := UnmarkOwned(fragPath, "hooks.SessionEnd"); err != nil {
			return false, fmt.Errorf("releasing owned key: %w", err)
		}
	} else {
		hooksRoot["SessionEnd"] = kept
	}
	if err := WriteJSON(fragPath, frag); err != nil {
		return false, fmt.Errorf("writing fragment: %w", err)
	}
	return true, nil
}

// desiredSessionEnd is the SessionEnd array the fragment should carry: every
// entry the host declares, then any profile-only entry already in the
// fragment, then our usage-sync hook — deduplicated by canonical JSON so
// re-running never grows the list.
func desiredSessionEnd(frag, host map[string]interface{}) []interface{} {
	seen := map[string]bool{}
	out := make([]interface{}, 0, len(sessionEndEntries(frag))+1)
	add := func(entries []interface{}) {
		for _, e := range entries {
			key, err := json.Marshal(e)
			if err != nil || seen[string(key)] {
				continue
			}
			seen[string(key)] = true
			out = append(out, e)
		}
	}
	add(sessionEndEntries(host))
	add(sessionEndEntries(frag))
	add([]interface{}{usageHookEntry()})
	return out
}

func usageHookEntry() map[string]interface{} {
	return map[string]interface{}{
		"matcher": "",
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": UsageSyncCommand,
			},
		},
	}
}

func sessionEndEntries(doc map[string]interface{}) []interface{} {
	root, _ := doc["hooks"].(map[string]interface{})
	entries, _ := root["SessionEnd"].([]interface{})
	return entries
}

// equalJSON compares two decoded JSON values structurally. Map keys are sorted
// by encoding/json, so the marshalled forms are directly comparable.
func equalJSON(a, b interface{}) bool {
	ab, aerr := json.Marshal(a)
	bb, berr := json.Marshal(b)
	return aerr == nil && berr == nil && bytes.Equal(ab, bb)
}

func entryRunsUsageSync(e interface{}) bool {
	em, ok := e.(map[string]interface{})
	if !ok {
		return false
	}
	hooks, _ := em["hooks"].([]interface{})
	for _, h := range hooks {
		if hm, ok := h.(map[string]interface{}); ok {
			if cmd, _ := hm["command"].(string); cmd == UsageSyncCommand {
				return true
			}
		}
	}
	return false
}

// usageHookPresent reports whether the fragment already carries a SessionEnd
// hook running UsageSyncCommand.
func usageHookPresent(frag map[string]interface{}) bool {
	return slices.ContainsFunc(sessionEndEntries(frag), entryRunsUsageSync)
}
