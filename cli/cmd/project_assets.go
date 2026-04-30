package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nitin-1926/ccpm/internal/settingsmerge"
)

// projectAssetEntry is one asset discovered inside <projectRoot>/.claude/<plural>/.
// It holds just enough to render a list row consistent with manifest-backed entries.
type projectAssetEntry struct {
	ID   string
	Path string
}

// discoverProjectAssets walks CWD up to the nearest project root (same rules as
// settingsmerge.FindProjectRoot) and returns the entries inside
// <root>/.claude/<plural>/. Directories and files both count; IDs are the
// basename with the extension stripped so they align with manifest IDs.
//
// Returns an empty slice (and "" root) when no project root is found.
func discoverProjectAssets(plural string) (root string, entries []projectAssetEntry) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", nil
	}
	root = settingsmerge.FindProjectRoot(cwd)
	if root == "" {
		return "", nil
	}
	dir := filepath.Join(root, ".claude", plural)
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return root, nil
	}
	for _, e := range dirEntries {
		name := e.Name()
		id := name
		if !e.IsDir() {
			id = strings.TrimSuffix(name, filepath.Ext(name))
		}
		entries = append(entries, projectAssetEntry{ID: id, Path: filepath.Join(dir, name)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return root, entries
}
