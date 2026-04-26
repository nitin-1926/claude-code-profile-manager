package defaultclaude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// HookEntry is a single hook definition discovered in ~/.claude/settings.json.
// One matcher can declare multiple hook commands; LoadHookEntries flattens
// them so each executable command is previewable on its own.
type HookEntry struct {
	Event   string
	Matcher string
	Index   int    // 0-based position within the Event's matcher array
	SubIdx  int    // 0-based position within the matcher's hooks array
	Type    string // typically "command"
	Command string
}

// ID returns a stable identifier used by the interactive picker. Uniqueness is
// guaranteed by combining event + index + sub-index.
func (e HookEntry) ID() string {
	return fmt.Sprintf("%s#%d.%d", e.Event, e.Index, e.SubIdx)
}

// LoadHookEntries reads ~/.claude/settings.json and flattens every hook
// command it can find. Missing file returns (nil, nil) so callers can treat
// absence as "nothing to import". Malformed JSON is surfaced so the user can
// fix their host settings before importing.
func LoadHookEntries() ([]HookEntry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}
	path := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
