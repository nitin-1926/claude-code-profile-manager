package consolidate

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// embeddedSkill ships the consolidate-claude-assets skill source inside the
// ccpm binary so `ccpm consolidate --install-skill` works without an
// internet connection or a checked-out repo.
//
//go:embed all:skill/consolidate-claude-assets
var embeddedSkill embed.FS

const skillName = "consolidate-claude-assets"
const embedRoot = "skill/" + skillName

// InstallSkill extracts the embedded skill into ~/.claude/skills/<skillName>/.
// Refuses to overwrite an existing install (user must remove first). Preserves
// executable bits on bundled scripts.
func InstallSkill() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	dest := filepath.Join(home, ".claude", "skills", skillName)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("refusing to overwrite existing install at %s — remove it first", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create skills dir: %w", err)
	}

	// Stage the whole skill into a sibling temp dir and rename into place on
	// success, so an extraction failure can't leave a half-written install
	// that the existing-install guard above would then refuse to repair.
	stage, err := os.MkdirTemp(filepath.Dir(dest), "."+skillName+"-stage-")
	if err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()

	if err := fs.WalkDir(embeddedSkill, embedRoot, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel := strings.TrimPrefix(p, embedRoot)
		rel = strings.TrimPrefix(rel, "/")
		target := filepath.Join(stage, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return extractEmbeddedFile(p, target, isExecutable(rel))
	}); err != nil {
		return fmt.Errorf("extract skill: %w", err)
	}

	if err := os.Rename(stage, dest); err != nil {
		return fmt.Errorf("activating skill install: %w", err)
	}

	fmt.Printf("Installed: %s\n", dest)
	fmt.Println("Run `ccpm sync` to cascade into all profiles, then invoke /consolidate-claude-assets in Claude Code.")
	return nil
}

// extractEmbeddedFile copies one embedded file to target with explicit Close
// error checks — a swallowed Close on a full disk would report success for a
// truncated file.
func extractEmbeddedFile(embedPath, target string, exec bool) error {
	f, err := embeddedSkill.Open(embedPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, f); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", target, err)
	}
	if exec {
		if err := os.Chmod(target, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func isExecutable(rel string) bool {
	if !strings.HasPrefix(rel, "scripts/") {
		return false
	}
	return strings.HasSuffix(rel, ".sh") || strings.HasSuffix(rel, ".py")
}
