package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nitin-1926/ccpm/internal/manifest"
	"github.com/nitin-1926/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/ccpm/internal/share"
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
