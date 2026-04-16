package sync

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nitin-1926/ccpm/internal/manifest"
	"github.com/nitin-1926/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/ccpm/internal/share"
)

// ApplyGlobals links all global skill installs into the given profile directory.
// MCP and settings globals are handled at launch via materialization.
func ApplyGlobals(profileDir, profileName string) error {
	m, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	skillsDir, err := share.SkillsDir()
	if err != nil {
		return err
	}

	for _, inst := range m.GlobalInstalls() {
		switch inst.Kind {
		case manifest.KindSkill:
			src := filepath.Join(skillsDir, inst.ID)
			if _, err := os.Stat(src); os.IsNotExist(err) {
				continue
			}
			dst := filepath.Join(profileDir, "skills", inst.ID)
			if err := share.Link(src, dst); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not link skill %q to profile %q: %v\n", inst.ID, profileName, err)
			}

		case manifest.KindMCP, manifest.KindSetting:
			// Handled below via settingsmerge.Materialize.
		}
	}

	// Materialize settings + MCP now so that brand-new profiles launch with
	// global fragments already merged, not on the first `ccpm run`.
	if err := settingsmerge.Materialize(profileDir, profileName); err != nil {
		return fmt.Errorf("materializing settings: %w", err)
	}
	if err := settingsmerge.MaterializeMCP(profileDir, profileName); err != nil {
		return fmt.Errorf("materializing MCP: %w", err)
	}

	return nil
}
