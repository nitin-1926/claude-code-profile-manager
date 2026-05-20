package cmd

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/profile"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
)

// credentialFiles are profile-dir files that hold (or directly reveal) secrets.
// Excluded from export by default — keychain tokens are machine-bound and can't
// migrate anyway, and bundling them would turn a shareable archive into a
// secret-leaking one.
var credentialFiles = map[string]bool{
	".credentials.json": true,
	".claude.json":      true,
}

var (
	exportOut                string
	exportIncludeCredentials bool
)

var exportCmd = &cobra.Command{
	Use:   "export <profile>",
	Short: "Export a profile (assets + settings) to a portable .tar.gz bundle",
	Long: `Bundles a profile's directory — skills, agents, commands, rules, hooks, MCP
fragments, plugin metadata, and settings — into a single .tar.gz you can copy
to another machine and restore with 'ccpm import-bundle'.

Credentials are NOT included by default: OS-keychain tokens are machine-bound,
and .credentials.json / .claude.json hold secrets you usually don't want in a
shareable file. Use --include-credentials only for a trusted same-user move
(e.g. Linux machine migration), and treat the resulting bundle as sensitive.`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeProfileNames,
	RunE:              runExport,
}

var importBundleCmd = &cobra.Command{
	Use:   "import-bundle <file.tar.gz>",
	Short: "Restore a profile from a 'ccpm export' bundle",
	Long: `Creates a new profile from a .tar.gz produced by 'ccpm export'. By default the
new profile takes the bundle's original name; override with --profile.

If the bundle did not include credentials (the default), authenticate the
restored profile afterwards with 'ccpm auth refresh <name>'.`,
	Args: cobra.ExactArgs(1),
	RunE: runImportBundle,
}

func init() {
	exportCmd.Flags().StringVarP(&exportOut, "output", "o", "", "output path (default: <profile>.ccpm.tar.gz)")
	exportCmd.Flags().BoolVar(&exportIncludeCredentials, "include-credentials", false, "include credential files (sensitive — trusted same-user moves only)")
	rootCmd.AddCommand(exportCmd)

	importBundleCmd.Flags().String("profile", "", "name for the restored profile (default: the bundle's original name)")
	rootCmd.AddCommand(importBundleCmd)
}

func runExport(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		return fmt.Errorf("profile %q not found", name)
	}

	outPath := exportOut
	if outPath == "" {
		outPath = name + ".ccpm.tar.gz"
	}

	out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, config.FilePerm)
	if err != nil {
		return fmt.Errorf("creating bundle: %w", err)
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	tw := tar.NewWriter(gz)

	skipped := 0
	walkErr := filepath.Walk(p.Dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(p.Dir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// Exclude credential files unless explicitly opted in.
		if !exportIncludeCredentials && credentialFiles[filepath.Base(path)] {
			skipped++
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip symlinks: they point into the machine-local share store and
		// won't resolve on the target. Restore re-materializes assets instead.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	if walkErr != nil {
		_ = tw.Close()
		_ = gz.Close()
		_ = os.Remove(outPath)
		return fmt.Errorf("archiving profile: %w", walkErr)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("finalizing archive: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("finalizing gzip: %w", err)
	}

	green := color.New(color.FgGreen, color.Bold)
	green.Printf("✓ Exported %q to %s\n", name, outPath)
	if !exportIncludeCredentials {
		fmt.Println("Credentials were excluded. Restore, then run `ccpm auth refresh <name>`.")
	} else {
		color.New(color.FgYellow).Println("Bundle contains credentials — keep it private.")
	}
	return nil
}


func runImportBundle(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("import-bundle: not yet available")
}
