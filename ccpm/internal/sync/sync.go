package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/manifest"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
)

// kindDirs maps each dedupable asset kind to the share-store root resolver and
// the per-profile subdirectory name. Kinds that materialize via settings (MCP,
// setting, plugin) are intentionally absent and handled below.
var kindDirs = map[manifest.AssetKind]struct {
	storeDir       func() (string, error)
	profileSubdir  string
}{
	manifest.KindSkill:   {share.SkillsDir, "skills"},
	manifest.KindAgent:   {share.AgentsDir, "agents"},
	manifest.KindCommand: {share.CommandsDir, "commands"},
	manifest.KindRule:    {share.RulesDir, "rules"},
	manifest.KindHook:    {share.HooksDir, "hooks"},
}

// ApplyGlobals links all global asset installs (skills, agents, commands,
// rules, hooks) into the given profile directory, then materializes settings
// and MCP so brand-new profiles launch with global fragments already applied.
func ApplyGlobals(profileDir, profileName string) error {
	m, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	for _, inst := range m.GlobalInstalls() {
		dirs, ok := kindDirs[inst.Kind]
		if !ok {
			// MCP / setting / plugin flow through materialization below.
			continue
		}
		storeRoot, err := dirs.storeDir()
		if err != nil {
			return err
		}
		entry := resolveStoreEntry(storeRoot, inst.ID)
		src := filepath.Join(storeRoot, entry)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}
		dst := filepath.Join(profileDir, dirs.profileSubdir, entry)
		if err := share.Link(src, dst); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not link %s %q to profile %q: %v\n", inst.Kind, inst.ID, profileName, err)
		}
	}

	if err := settingsmerge.MaterializeAll(profileDir, profileName, ""); err != nil {
		return fmt.Errorf("materializing profile settings: %w", err)
	}
	return nil
}

// resolveStoreEntry mirrors cmd.findStoreEntry: the manifest stores the logical
// ID (no extension for file-based assets), while the store uses the full
// basename (e.g. "foo.md"). Prefer an exact-name match, then fall back to a
// stem match inside the store root.
func resolveStoreEntry(storeRoot, assetID string) string {
	if _, err := os.Stat(filepath.Join(storeRoot, assetID)); err == nil {
		return assetID
	}
	entries, err := os.ReadDir(storeRoot)
	if err != nil {
		return assetID
	}
	for _, e := range entries {
		stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		if stem == assetID {
			return e.Name()
		}
	}
	return assetID
}
