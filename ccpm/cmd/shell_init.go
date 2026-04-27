package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/shell"
)

var shellFlag string

var shellInitCmd = &cobra.Command{
	Use:   "shell-init",
	Short: "Print shell hook for .zshrc/.bashrc",
	Long:  "Prints a shell function that wraps ccpm to enable 'ccpm use' to set environment variables in the current shell.\n\nAdd to your shell config:\n  eval \"$(ccpm shell-init)\"",
	RunE:  runShellInit,
}

func init() {
	shellInitCmd.Flags().StringVar(&shellFlag, "shell", "", "shell type (bash, zsh, fish, powershell). Auto-detected if empty.")
	rootCmd.AddCommand(shellInitCmd)
}

func runShellInit(cmd *cobra.Command, args []string) error {
	s := shellFlag
	if s == "" {
		s = shell.DetectShell()
	}

	hook := shell.GenerateHook(s)
	fmt.Println(hook)
	return nil
}
