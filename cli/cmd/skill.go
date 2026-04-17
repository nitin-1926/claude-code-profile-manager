package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/manifest"
	"github.com/nitin-1926/ccpm/internal/share"
)

var (
	skillProfile string
	skillGlobal  bool
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage Claude Code skills across profiles",
}

var skillAddCmd = &cobra.Command{
	Use:   "add <path>",
	Short: "Install a skill from a local directory",
	Long: `Install a skill into one or all profiles.

The source must be a directory containing a SKILL.md file.
Use --global to install for all profiles, or --profile to target one.`,
	Args: cobra.ExactArgs(1),
	RunE: runSkillAdd,
}

var skillRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Short:   "Remove a skill",
	Aliases: []string{"rm"},
	Args:    cobra.ExactArgs(1),
	RunE:    runSkillRemove,
}

var skillListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List installed skills",
	Aliases: []string{"ls"},
	RunE:    runSkillList,
}

var skillLinkCmd = &cobra.Command{
	Use:   "link <name> --profile <name>",
	Short: "Link a shared skill into a specific profile",
	Args:  cobra.ExactArgs(1),
	RunE:  runSkillLink,
}

func init() {
	skillAddCmd.Flags().BoolVar(&skillGlobal, "global", false, "install for all profiles")
	skillAddCmd.Flags().StringVar(&skillProfile, "profile", "", "install for a specific profile")
	skillRemoveCmd.Flags().BoolVar(&skillGlobal, "global", false, "remove from all profiles")
	skillRemoveCmd.Flags().StringVar(&skillProfile, "profile", "", "remove from a specific profile")
	skillLinkCmd.Flags().StringVar(&skillProfile, "profile", "", "target profile (required)")
	_ = skillLinkCmd.MarkFlagRequired("profile")

	skillCmd.AddCommand(skillAddCmd)
	skillCmd.AddCommand(skillRemoveCmd)
	skillCmd.AddCommand(skillListCmd)
	skillCmd.AddCommand(skillLinkCmd)
	rootCmd.AddCommand(skillCmd)
}

func runSkillAdd(cmd *cobra.Command, args []string) error {
	srcPath := args[0]

	abs, err := filepath.Abs(srcPath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("source path %q does not exist: %w", abs, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("source %q must be a directory containing SKILL.md", abs)
	}

	skillMD := filepath.Join(abs, "SKILL.md")
	if _, err := os.Stat(skillMD); os.IsNotExist(err) {
		return fmt.Errorf("no SKILL.md found in %q — not a valid skill directory", abs)
	}

	skillID := filepath.Base(abs)

	if !skillGlobal && skillProfile == "" {
		return fmt.Errorf("specify --global or --profile <name>")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if err := config.EnsureDirs(); err != nil {
		return err
	}
	if err := share.EnsureDirs(); err != nil {
		return err
	}

	skillsDir, err := share.SkillsDir()
	if err != nil {
		return err
	}
	sharedDst := filepath.Join(skillsDir, skillID)

	if err := copySkillToStore(abs, sharedDst); err != nil {
		return fmt.Errorf("copying skill to shared store: %w", err)
	}

	m, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	green := color.New(color.FgGreen, color.Bold)

	if skillGlobal {
		profiles := config.ProfileNames(cfg)
		for _, name := range profiles {
			p := cfg.Profiles[name]
			if err := linkSkillToProfile(sharedDst, p.Dir, skillID); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not link skill to profile %q: %v\n", name, err)
			}
		}

		if existing := m.Find(skillID, manifest.KindSkill); existing != nil {
			m.Remove(skillID, manifest.KindSkill)
		}
		m.Add(manifest.Install{
			ID:       skillID,
			Kind:     manifest.KindSkill,
			Scope:    manifest.ScopeGlobal,
			Source:   abs,
			Profiles: profiles,
		})

		if err := manifest.Save(m); err != nil {
			return fmt.Errorf("saving manifest: %w", err)
		}

		green.Printf("✓ Skill %q installed globally (%d profiles)\n", skillID, len(profiles))
	} else {
		p, exists := cfg.Profiles[skillProfile]
		if !exists {
			return fmt.Errorf("profile %q not found", skillProfile)
		}

		if err := linkSkillToProfile(sharedDst, p.Dir, skillID); err != nil {
			return fmt.Errorf("linking skill to profile: %w", err)
		}

		if existing := m.Find(skillID, manifest.KindSkill); existing != nil {
			m.Remove(skillID, manifest.KindSkill)
		}
		m.Add(manifest.Install{
			ID:       skillID,
			Kind:     manifest.KindSkill,
			Scope:    manifest.ScopeProfile,
			Source:   abs,
			Profiles: []string{skillProfile},
		})

		if err := manifest.Save(m); err != nil {
			return fmt.Errorf("saving manifest: %w", err)
		}

		green.Printf("✓ Skill %q installed for profile %q\n", skillID, skillProfile)
	}

	return nil
}

func runSkillRemove(cmd *cobra.Command, args []string) error {
	skillID := args[0]

	if !skillGlobal && skillProfile == "" {
		return fmt.Errorf("specify --global or --profile <name>")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	m, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	green := color.New(color.FgGreen, color.Bold)

	if skillGlobal {
		for _, name := range config.ProfileNames(cfg) {
			p := cfg.Profiles[name]
			unlinkSkillFromProfile(p.Dir, skillID)
		}
		skillsDir, _ := share.SkillsDir()
		os.RemoveAll(filepath.Join(skillsDir, skillID))
	} else {
		p, exists := cfg.Profiles[skillProfile]
		if !exists {
			return fmt.Errorf("profile %q not found", skillProfile)
		}
		unlinkSkillFromProfile(p.Dir, skillID)
	}

	m.Remove(skillID, manifest.KindSkill)
	if err := manifest.Save(m); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	green.Printf("✓ Skill %q removed\n", skillID)
	return nil
}

func runSkillList(cmd *cobra.Command, args []string) error {
	m, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	skills := m.ListByKind(manifest.KindSkill)
	if len(skills) == 0 {
		fmt.Println("No skills installed. Install one with: ccpm skill add <path> --global")
		return nil
	}

	bold := color.New(color.Bold).SprintFunc()
	fmt.Printf("  %-20s %-10s %s\n", bold("SKILL"), bold("SCOPE"), bold("PROFILES"))
	fmt.Printf("  %s\n", strings.Repeat("─", 50))

	for _, s := range skills {
		profiles := strings.Join(s.Profiles, ", ")
		if s.Scope == manifest.ScopeGlobal {
			profiles = "all"
		}
		fmt.Printf("  %-20s %-10s %s\n", s.ID, s.Scope, profiles)
	}

	return nil
}

func runSkillLink(cmd *cobra.Command, args []string) error {
	skillID := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	p, exists := cfg.Profiles[skillProfile]
	if !exists {
		return fmt.Errorf("profile %q not found", skillProfile)
	}

	skillsDir, err := share.SkillsDir()
	if err != nil {
		return err
	}
	sharedSrc := filepath.Join(skillsDir, skillID)

	if _, err := os.Stat(sharedSrc); os.IsNotExist(err) {
		return fmt.Errorf("skill %q not found in shared store. Install it first with: ccpm skill add <path> --global", skillID)
	}

	if err := linkSkillToProfile(sharedSrc, p.Dir, skillID); err != nil {
		return fmt.Errorf("linking skill: %w", err)
	}

	color.New(color.FgGreen, color.Bold).Printf("✓ Skill %q linked to profile %q\n", skillID, skillProfile)
	return nil
}

func linkSkillToProfile(sharedSrc, profileDir, skillID string) error {
	dst := filepath.Join(profileDir, "skills", skillID)
	return share.Link(sharedSrc, dst)
}

func unlinkSkillFromProfile(profileDir, skillID string) {
	dst := filepath.Join(profileDir, "skills", skillID)
	share.Unlink(dst)
}

func copySkillToStore(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
	}
	return copyDirRecursive(src, dst)
}

func copyDirRecursive(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
