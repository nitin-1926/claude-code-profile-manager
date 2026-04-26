package settingsmerge

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/nitin-1926/ccpm/internal/config"
)

// OwnedKeysFile is the sidecar JSON file stored next to each fragment that
// tracks which leaf key paths were explicitly set by the user through
// `ccpm settings set` / `ccpm settings apply`. Keys listed here are
// re-applied on top of the merge so that local edits to settings.json
// cannot silently shadow them.
type OwnedKeysFile struct {
	Keys []string `json:"keys"`
}

// ownedKeysPath returns "<fragment>.owned.json" for a given fragment path.
func ownedKeysPath(fragmentPath string) string {
	return strings.TrimSuffix(fragmentPath, ".json") + ".owned.json"
}

// LoadOwnedKeys reads the sidecar for a fragment path. Missing file is not
// an error.
func LoadOwnedKeys(fragmentPath string) (map[string]struct{}, error) {
	data, err := os.ReadFile(ownedKeysPath(fragmentPath))
	if os.IsNotExist(err) {
		return map[string]struct{}{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading owned-keys sidecar: %w", err)
	}
	var file OwnedKeysFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parsing owned-keys sidecar: %w", err)
	}
	set := make(map[string]struct{}, len(file.Keys))
	for _, k := range file.Keys {
		set[k] = struct{}{}
	}
	return set, nil
}

// SaveOwnedKeys persists the sidecar for the given fragment.
func SaveOwnedKeys(fragmentPath string, keys map[string]struct{}) error {
	list := make([]string, 0, len(keys))
	for k := range keys {
		list = append(list, k)
	}
	sort.Strings(list)
	data, err := json.MarshalIndent(OwnedKeysFile{Keys: list}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(ownedKeysPath(fragmentPath), data, config.FilePerm)
}

// MarkOwned adds a dot-notation key path to the sidecar for a fragment.
func MarkOwned(fragmentPath, key string) error {
	set, err := LoadOwnedKeys(fragmentPath)
	if err != nil {
		return err
	}
	set[key] = struct{}{}
	return SaveOwnedKeys(fragmentPath, set)
}

// MarkOwnedFromPatch adds every leaf key path in `patch` to the sidecar.
// Arrays and scalars count as leaves; nested objects recurse.
func MarkOwnedFromPatch(fragmentPath string, patch map[string]interface{}) error {
	set, err := LoadOwnedKeys(fragmentPath)
	if err != nil {
