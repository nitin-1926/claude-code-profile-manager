package cmd

import (
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

// completeProfileNames is a cobra ValidArgsFunction that completes the first
// positional argument with the set of known profile names. Wired into the
// commands that take a profile name (run, use, remove, rename, set-default,
// clone, export) so `ccpm run <TAB>` offers the user's actual profiles.
//
// Cobra already ships a `completion` subcommand that emits the bash/zsh/fish/
// powershell scripts; this just feeds it real data instead of file paths.
func completeProfileNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Only complete the first positional arg; subsequent args aren't profiles.
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	names := config.ProfileNames(cfg)
	return names, cobra.ShellCompDirectiveNoFileComp
}
