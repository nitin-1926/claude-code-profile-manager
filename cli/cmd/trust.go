package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/ccpm/internal/trust"
)

func newTrustCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "trust",
		Short: "Manage which project directories are allowed to contribute hooks/permissions/MCP",
		Long: `Control the project-trust allowlist.

ccpm treats every project as untrusted by default: a project's
.claude/settings.json / .claude/settings.local.json / .mcp.json cannot register
hooks, override permissions, declare a statusLine command, or add MCP servers
unless you have explicitly trusted the project root.

Trust granted here persists across sessions in ~/.ccpm/trusted-projects.json
(mode 0600, owned by the invoking user). Trust is by absolute path — renaming
or moving a project directory invalidates the entry.`,
	}

	addCmd := &cobra.Command{
		Use:     "add [path]",
		Aliases: []string{"grant"},
		Short:   "Grant trust to the given project directory (defaults to CWD)",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTrustAdd(args)
		},
	}

	removeCmd := &cobra.Command{
		Use:     "remove [path]",
		Aliases: []string{"rm", "forget", "revoke"},
		Short:   "Revoke trust for the given project directory (defaults to CWD)",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTrustRemove(args)
		},
	}

	listCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List trusted project directories",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTrustList()
		},
	}

	root.AddCommand(addCmd, removeCmd, listCmd)
	return root
}

func init() {
	rootCmd.AddCommand(newTrustCmd())
}
