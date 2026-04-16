package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/ccpm/internal/defaultclaude"
)

var defaultCmd = &cobra.Command{
	Use:   "default",
	Short: "Inspect or snapshot the default ~/.claude config",
}

var defaultFingerprintCmd = &cobra.Command{
	Use:   "fingerprint",
	Short: "Manage the ~/.claude drift fingerprint",
}

var defaultFingerprintUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Record the current ~/.claude state as the drift baseline",
	RunE:  runDefaultFingerprintUpdate,
}

var defaultFingerprintCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Report drift between ~/.claude and the last fingerprint",
	RunE:  runDefaultFingerprintCheck,
}

func init() {
	defaultFingerprintCmd.AddCommand(defaultFingerprintUpdateCmd)
	defaultFingerprintCmd.AddCommand(defaultFingerprintCheckCmd)
	defaultCmd.AddCommand(defaultFingerprintCmd)
	rootCmd.AddCommand(defaultCmd)
}

func runDefaultFingerprintUpdate(cmd *cobra.Command, args []string) error {
	if !defaultclaude.Exists() {
		dir, _ := defaultclaude.DefaultDir()
		return fmt.Errorf("no default Claude config at %s", dir)
	}
	snap, err := defaultclaude.Snapshot(defaultclaude.DefaultTargets())
	if err != nil {
		return err
	}
	if err := defaultclaude.SaveFingerprint(snap); err != nil {
		return err
	}
	color.New(color.FgGreen, color.Bold).Printf("✓ Fingerprint updated (%d files tracked)\n", len(snap.Files))
	return nil
}

func runDefaultFingerprintCheck(cmd *cobra.Command, args []string) error {
	if !defaultclaude.Exists() {
		dir, _ := defaultclaude.DefaultDir()
		return fmt.Errorf("no default Claude config at %s", dir)
	}
	stored, err := defaultclaude.LoadFingerprint()
	if err != nil {
		return err
	}
	current, err := defaultclaude.Snapshot(defaultclaude.DefaultTargets())
	if err != nil {
		return err
	}

	green := color.New(color.FgGreen, color.Bold)
	yellow := color.New(color.FgYellow, color.Bold)
	dim := color.New(color.Faint)

	if stored == nil {
		yellow.Println("No fingerprint recorded yet.")
		fmt.Println("Run 'ccpm default fingerprint update' or 'ccpm import default' to establish a baseline.")
		return nil
	}

	drift := defaultclaude.Compare(stored, current)
	if !drift.HasChanges() {
		green.Println("✓ ~/.claude matches the last fingerprint — no drift.")
		dim.Printf("  Last snapshot: %s (%d files)\n", stored.TakenAt, len(stored.Files))
		return nil
	}

	yellow.Println("Drift detected in ~/.claude since last fingerprint:")
	printDriftSection("added", drift.Added, color.New(color.FgGreen))
	printDriftSection("modified", drift.Modified, color.New(color.FgYellow))
	printDriftSection("removed", drift.Removed, color.New(color.FgRed))
	fmt.Println()
	fmt.Println("To sync these into a profile:   ccpm import default --profile <name>")
	fmt.Println("To accept without importing:    ccpm default fingerprint update")
	return nil
}

func printDriftSection(label string, paths []string, c *color.Color) {
	if len(paths) == 0 {
		return
	}
	c.Printf("  %s (%d):\n", label, len(paths))
	for _, p := range paths {
		fmt.Printf("    %s\n", p)
	}
}
