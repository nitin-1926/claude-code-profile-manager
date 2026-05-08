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
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("create dest: %w", err)
	}

	if err := fs.WalkDir(embeddedSkill, embedRoot, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel := strings.TrimPrefix(p, embedRoot)
		rel = strings.TrimPrefix(rel, "/")
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		f, err := embeddedSkill.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err := io.Copy(out, f); err != nil {
			return err
		}
		// Bundled scripts need exec bit.
		if isExecutable(rel) {
			if err := os.Chmod(target, 0o755); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("extract skill: %w", err)
	}

	fmt.Printf("Installed: %s\n", dest)
	fmt.Println("Run `ccpm sync` to cascade into all profiles, then invoke /consolidate-claude-assets in Claude Code.")
	return nil
}

func isExecutable(rel string) bool {
	if !strings.HasPrefix(rel, "scripts/") {
		return false
	}
	return strings.HasSuffix(rel, ".sh") || strings.HasSuffix(rel, ".py")
}
