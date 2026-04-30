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
