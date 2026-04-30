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

func runTrustAdd(args []string) error {
	root, err := resolveTrustArg(args)
	if err != nil {
		return err
	}
	if err := trust.MarkTrusted(root); err != nil {
		return fmt.Errorf("granting trust: %w", err)
	}
	color.New(color.FgGreen, color.Bold).Printf("✓ Trusted %s\n", root)
	fmt.Println("  This project's hooks, permissions, statusLine, mcpServers, env, and enabledPlugins keys")
	fmt.Println("  will now contribute to the profile merge at ccpm run.")
	return nil
}

func runTrustRemove(args []string) error {
	root, err := resolveTrustArg(args)
	if err != nil {
		return err
	}
	removed, err := trust.Forget(root)
	if err != nil {
		return fmt.Errorf("revoking trust: %w", err)
	}
	if !removed {
		fmt.Printf("%s was not in the trust list.\n", root)
		return nil
	}
	color.New(color.FgGreen, color.Bold).Printf("✓ Revoked trust for %s\n", root)
	return nil
}

func runTrustList() error {
	records, err := trust.All()
	if err != nil {
		return err
	}
	if len(records) == 0 {
		fmt.Println("No trusted projects. Grant trust with: ccpm trust add [path]")
		return nil
	}
	bold := color.New(color.Bold).SprintFunc()
	fmt.Printf("  %-50s %s\n", bold("PATH"), bold("GRANTED AT"))
	fmt.Printf("  %s\n", strings.Repeat("─", 72))
	for _, r := range records {
		fmt.Printf("  %-50s %s\n", r.Path, r.GrantedAt)
	}
	return nil
}

// resolveTrustArg resolves the optional trust target — either the first
// positional arg or CWD — to an absolute path.
func resolveTrustArg(args []string) (string, error) {
	if len(args) == 1 {
		abs, err := filepath.Abs(args[0])
		if err != nil {
			return "", fmt.Errorf("resolving %q: %w", args[0], err)
		}
		return abs, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting CWD: %w", err)
	}
	return cwd, nil
}
