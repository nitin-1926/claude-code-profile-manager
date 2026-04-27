package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/manifest"
)

// TestApplyGlobalsLinksEveryDedupableKind regression-tests the bug reported as
// sync H1: prior to the fix, only KindSkill was re-linked into new profiles;
// KindAgent / KindCommand / KindRule / KindHook were silently dropped.
func TestApplyGlobalsLinksEveryDedupableKind(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	// Seed the share store with one entry per dedupable kind.
	base := filepath.Join(tmp, ".ccpm")
	shareBase := filepath.Join(base, "share")
	for _, sub := range []string{"skills", "agents", "commands", "rules", "hooks"} {
		entryDir := filepath.Join(shareBase, sub, "entry-"+sub)
		if err := os.MkdirAll(entryDir, 0700); err != nil {
			t.Fatalf("seed %s: %v", sub, err)
		}
		// Leave a sentinel file so the symlink target resolves to something.
		if err := os.WriteFile(filepath.Join(entryDir, "payload"), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	// Register each entry as a Global install in the manifest.
	m := &manifest.Manifest{}
	for _, k := range []manifest.AssetKind{
		manifest.KindSkill, manifest.KindAgent, manifest.KindCommand,
		manifest.KindRule, manifest.KindHook,
	} {
		// The manifest ID must match the store-entry name or resolveStoreEntry's
		// fallback logic will miss it.
		id := "entry-" + kindToSubdir(t, k)
		m.Add(manifest.Install{ID: id, Kind: k, Scope: manifest.ScopeGlobal})
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	if err := manifest.Save(m); err != nil {
		t.Fatalf("manifest save: %v", err)
	}

	// New profile directory; expect every subdir to end up with a link pointing
	// into the share store after ApplyGlobals.
	profileDir := filepath.Join(base, "profiles", "new")
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		t.Fatal(err)
	}

	if err := ApplyGlobals(profileDir, "new"); err != nil {
		t.Fatalf("ApplyGlobals: %v", err)
	}

	for _, sub := range []string{"skills", "agents", "commands", "rules", "hooks"} {
		linkPath := filepath.Join(profileDir, sub, "entry-"+sub)
		info, err := os.Lstat(linkPath)
		if err != nil {
			t.Errorf("%s/entry-%s: expected link, got %v", sub, sub, err)
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			// Windows copy fallback is acceptable on that platform; on
			// unix/darwin the test runs as a real symlink. Just ensure the
			// file exists and resolves to the share store source.
			resolved, err := filepath.EvalSymlinks(linkPath)
			if err != nil {
				t.Errorf("%s: cannot resolve: %v", linkPath, err)
				continue
			}
			if filepath.Base(resolved) != "entry-"+sub {
				t.Errorf("%s: resolved target %s does not point to share store", linkPath, resolved)
			}
			continue
		}
		target, err := os.Readlink(linkPath)
		if err != nil {
			t.Errorf("%s: cannot readlink: %v", linkPath, err)
			continue
		}
		if filepath.Base(target) != "entry-"+sub {
			t.Errorf("%s: symlink target %s does not point to share store entry", linkPath, target)
		}
	}
}

// kindToSubdir mirrors the kindDirs map in sync.go so the test doesn't have
// to reach into the private var.
func kindToSubdir(t *testing.T, k manifest.AssetKind) string {
	t.Helper()
	switch k {
	case manifest.KindSkill:
		return "skills"
	case manifest.KindAgent:
		return "agents"
	case manifest.KindCommand:
		return "commands"
	case manifest.KindRule:
		return "rules"
	case manifest.KindHook:
		return "hooks"
	default:
		t.Fatalf("unexpected kind: %s", k)
		return ""
	}
}

// Keep unused-import lints quiet when build tags change.
var _ = config.BaseDir
