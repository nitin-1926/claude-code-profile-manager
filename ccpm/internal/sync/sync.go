package sync

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/manifest"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
)

// kindDirs maps each dedupable asset kind to the share-store root resolver and
// the per-profile subdirectory name. Kinds that materialize via settings (MCP,
// setting, plugin) are intentionally absent and handled below.
var kindDirs = map[manifest.AssetKind]struct {
	storeDir      func() (string, error)
	profileSubdir string
}{
	manifest.KindSkill:   {share.SkillsDir, "skills"},
	manifest.KindAgent:   {share.AgentsDir, "agents"},
	manifest.KindCommand: {share.CommandsDir, "commands"},
	manifest.KindRule:    {share.RulesDir, "rules"},
	manifest.KindHook:    {share.HooksDir, "hooks"},
}

// Options controls the behavior of ApplyGlobals.
type Options struct {
	// SkipHostAdoption disables the ~/.claude scan + auto-link step. Used by
	// `--no-auto-adopt` flags on `ccpm run` and `ccpm sync` for one-shot
	// opt-out without flipping the persistent setting. The persistent
	// `cascade_auto_adopt` config setting is checked separately by callers
	// before constructing Options.
	SkipHostAdoption bool
	// QuietAdoption suppresses the stderr "adopted N items" line. Set on
	// non-interactive callers (programmatic sync from add.go) so test
	// output and scripts stay clean.
	QuietAdoption bool
}

// ApplyGlobals links every cascading manifest entry (Global + Host) into the
// given profile, then optionally scans the host ~/.claude tree for newly-
// arrived assets and adopts them. Finally materializes settings + MCP so
// brand-new profiles launch with global fragments already applied.
//
// Order matters:
//
//  1. Link known cascade entries first — this is cheap and idempotent and
//     ensures a freshly-created profile gets every previously-adopted host
//     entry without the scanner having to re-do work.
//  2. Scan host for unadopted entries and adopt them — this is the cascade
//     step described in plans/asset-cascade.md.
//  3. MaterializeAll — must run last so any settings.json fragments touched
//     by adoption (none today, but future-proofing) are picked up.
//
// Backwards-compat: the no-arg signature ApplyGlobals(dir, name) is kept as
// a thin wrapper for callers that don't yet pass Options.
func ApplyGlobals(profileDir, profileName string) error {
	return ApplyGlobalsWithOptions(profileDir, profileName, Options{})
}

// ApplyGlobalsWithOptions is the option-taking variant. New code should
// prefer this so callers can disable adoption for one-shot scripts.
func ApplyGlobalsWithOptions(profileDir, profileName string, opts Options) error {
	m, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	// Step 1 — link existing cascade manifest entries (Global + Host) into
	// the profile. Host entries point straight at ~/.claude/<plural>/<name>;
	// global entries route through the share store.
	for _, inst := range m.CascadeInstalls() {
		if err := linkCascadeEntry(profileDir, profileName, inst); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not link %s %q to profile %q: %v\n", inst.Kind, inst.ID, profileName, err)
		}
	}

	// Step 2 — host adoption. Gated by both the persistent setting and the
	// per-call opt-out.
	if !opts.SkipHostAdoption && cascadeAutoAdoptEnabled() {
		newDirEntries, err := scanHostUnadopted(m)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: scanning host assets failed: %v\n", err)
		} else if _, err := adoptHostEntries(profileDir, profileName, newDirEntries, m); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: adopting host assets failed: %v\n", err)
		} else if !opts.QuietAdoption {
			reportAdoption(profileName, newDirEntries)
		}

		newPlugins, err := scanHostPlugins(m)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: scanning host plugins failed: %v\n", err)
		} else if err := adoptHostPlugins(profileDir, profileName, newPlugins, m); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: adopting host plugins failed: %v\n", err)
		} else if !opts.QuietAdoption && len(newPlugins) > 0 {
			fmt.Fprintf(os.Stderr,
				"ccpm: adopted %d host plugin(s) into profile %q — disable with `ccpm config set cascade_auto_adopt false`\n",
				len(newPlugins), profileName,
			)
		}

		if err := manifest.Save(m); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: saving manifest after adoption failed: %v\n", err)
		}
	}

	if err := settingsmerge.MaterializeAll(profileDir, profileName, ""); err != nil {
		return fmt.Errorf("materializing profile settings: %w", err)
	}
	return nil
}

// PreviewApplyGlobals reports what ApplyGlobalsWithOptions would do for one
// profile without mutating anything: the cascade manifest entries that would
// be (re-)linked and the host entries not yet adopted. Used by
// `ccpm sync --dry-run`.
func PreviewApplyGlobals(profileName string) (cascade []string, adoptable []string, err error) {
	m, err := manifest.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("loading manifest: %w", err)
	}
	for _, inst := range m.CascadeInstalls() {
		cascade = append(cascade, fmt.Sprintf("%s %q (%s)", inst.Kind, inst.ID, inst.Scope))
	}
	if cascadeAutoAdoptEnabled() {
		entries, scanErr := scanHostUnadopted(m)
		if scanErr != nil {
			return cascade, nil, scanErr
		}
		for _, e := range entries {
			adoptable = append(adoptable, fmt.Sprintf("%s %q (from ~/.claude)", e.Kind, e.Name))
		}
		plugins, perr := scanHostPlugins(m)
		if perr == nil {
			for _, pl := range plugins {
				adoptable = append(adoptable, fmt.Sprintf("plugin %q (from ~/.claude)", pl.ID))
			}
		}
	}
	_ = profileName // per-profile linking is identical across profiles today
	return cascade, adoptable, nil
}

// EnsureHostAdoption runs only the host scan + adopt step (and the cascade
// link pass) without re-running MaterializeAll. Called from `ccpm run`
// before the materialize step so a newly-installed host skill becomes
// visible in the next session without requiring an explicit `ccpm sync`.
func EnsureHostAdoption(profileDir, profileName string, skip bool) error {
	if skip || !cascadeAutoAdoptEnabled() {
		return nil
	}
	m, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	newDirEntries, scanErr := scanHostUnadopted(m)
	if scanErr != nil {
		return fmt.Errorf("scanning host assets: %w", scanErr)
	}
	slog.Debug("host adoption scan", "profile", profileName, "unadopted", len(newDirEntries))
	dirMutated, err := adoptHostEntries(profileDir, profileName, newDirEntries, m)
	if err != nil {
		return fmt.Errorf("adopting host assets: %w", err)
	}
	if len(newDirEntries) > 0 {
		reportAdoption(profileName, newDirEntries)
	}

	newPlugins, perr := scanHostPlugins(m)
	if perr != nil {
		return fmt.Errorf("scanning host plugins: %w", perr)
	}
	if err := adoptHostPlugins(profileDir, profileName, newPlugins, m); err != nil {
		return fmt.Errorf("adopting host plugins: %w", err)
	}
	if len(newPlugins) > 0 {
		fmt.Fprintf(os.Stderr,
			"ccpm: adopted %d host plugin(s) into profile %q — disable with `ccpm config set cascade_auto_adopt false`\n",
			len(newPlugins), profileName,
		)
	}

	// Re-link existing host entries too — covers the case where the user
	// removed the symlink manually but the manifest still owns it.
	for _, inst := range m.CascadeInstalls() {
		if inst.Scope == manifest.ScopeHost {
			if err := linkCascadeEntry(profileDir, profileName, inst); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: relinking %s %q failed: %v\n", inst.Kind, inst.ID, err)
			}
		}
	}

	// Persist on actual manifest mutation, not entry counts — a Profiles-list
	// append with zero new entries was previously lost here (M16).
	if !dirMutated && len(newPlugins) == 0 {
		return nil
	}
	return manifest.Save(m)
}

// linkCascadeEntry routes one manifest entry to either the host link path
// (host scope) or the share-store link path (global scope). Idempotent —
// share.Link short-circuits when the destination already points at the
// right source.
func linkCascadeEntry(profileDir, profileName string, inst manifest.Install) error {
	switch inst.Scope {
	case manifest.ScopeHost:
		if inst.Kind == manifest.KindPlugin {
			return linkHostPlugin(profileDir, profileName, inst)
		}
		return linkHostEntry(profileDir, inst)
	case manifest.ScopeGlobal:
		dirs, ok := kindDirs[inst.Kind]
		if !ok {
			// MCP / setting / plugin (global scope) flow through
			// MaterializeAll. Plugins under Global scope are handled by
			// internal/plugins.LinkIntoProfile, called from cmd/plugin.go.
			return nil
		}
		storeRoot, err := dirs.storeDir()
		if err != nil {
			return err
		}
		entry := resolveStoreEntry(storeRoot, inst.ID)
		src := filepath.Join(storeRoot, entry)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			return nil
		}
		dst := filepath.Join(profileDir, dirs.profileSubdir, entry)
		return share.Link(src, dst)
	}
	return nil
}

// cascadeAutoAdoptEnabled reads the persistent setting. Treated as enabled
// (true) if the config file is missing or unreadable, so a brand-new
// install behaves like "cascade on" before the user has touched config.
func cascadeAutoAdoptEnabled() bool {
	cfg, err := config.Load()
	if err != nil {
		return true
	}
	return cfg.Settings.CascadeAutoAdoptEnabled()
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
