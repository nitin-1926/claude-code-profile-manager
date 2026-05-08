package cmd

import (
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/consolidate"
)

var (
	consolidateDryRun       bool
	consolidateFix          bool
	consolidateInstallSkill bool
	consolidateProfile      string
)

var consolidateCmd = &cobra.Command{
	Use:   "consolidate",
	Short: "Audit and consolidate Claude Code assets across scopes",
	Long: `Detect and surface duplicates, dangling symlinks, ghost manifest entries,
broken installs, plugin/direct skill overlap, hook duplication, permission
duplication across profiles, MCP/plugin scope drift, stale plugin caches, and
skill description budget overflow across ~/.claude/, ~/.ccpm/, and project
.claude/ directories.

Default: read-only audit + summary. Use --fix to apply low-risk auto-fixes
(dangling symlinks, stale plugin caches). For richer interactive proposals
(deciding canonical scopes, extracting plugin skills, promoting permissions),
install the bundled skill and run /consolidate-claude-assets inside Claude
Code:

  ccpm consolidate --install-skill   # one-time
  claude /consolidate-claude-assets  # interactive flow`,
	RunE: runConsolidate,
}

func init() {
	consolidateCmd.Flags().BoolVar(&consolidateDryRun, "dry-run", false, "audit only; never apply fixes (default behavior unless --fix is passed)")
	consolidateCmd.Flags().BoolVar(&consolidateFix, "fix", false, "apply auto-fixable issues (dangling symlinks, stale caches)")
	consolidateCmd.Flags().BoolVar(&consolidateInstallSkill, "install-skill", false, "extract embedded skill into ~/.claude/skills/")
	consolidateCmd.Flags().StringVar(&consolidateProfile, "profile", "", "narrow audit to one ccpm profile")
	rootCmd.AddCommand(consolidateCmd)
}

func runConsolidate(cmd *cobra.Command, args []string) error {
	if consolidateInstallSkill {
		return consolidate.InstallSkill()
	}
	return consolidate.Run(consolidate.Options{
		DryRun:  consolidateDryRun,
		Fix:     consolidateFix,
		Profile: consolidateProfile,
	})
}
