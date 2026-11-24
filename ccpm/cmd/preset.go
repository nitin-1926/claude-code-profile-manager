package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/presets"
)

func init() {
	root := &cobra.Command{
		Use:   "preset",
		Short: "Third-party provider presets for `ccpm add --preset`",
	}
	root.AddCommand(&cobra.Command{
		Use:     "list",
		Short:   "List the provider presets",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			bold := color.New(color.Bold)
			for _, p := range presets.All() {
				key := "key required"
				if !p.KeyRequired {
					key = "no key needed"
				}
				bold.Printf("%s", p.ID)
				fmt.Printf("  %s, %s, %s\n", p.Label, key, p.BaseURL())
				fmt.Printf("  %s\n  %s\n\n", p.Description, p.SourceURL)
			}
			fmt.Println("Each provider's compatibility is its own claim, and some Claude Code features may behave differently.")
			fmt.Println("You use that provider's API key and billing. Create a profile with: ccpm add <name> --preset <id>")
			return nil
		},
	})
	rootCmd.AddCommand(root)
}
