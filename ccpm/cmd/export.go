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
	bundlePath := args[0]

	override, _ := cmd.Flags().GetString("profile")

	name := override
	if name == "" {
		// Default the profile name to the bundle's base filename, stripping the
		// conventional suffixes.
		base := filepath.Base(bundlePath)
		base = strings.TrimSuffix(base, ".tar.gz")
		base = strings.TrimSuffix(base, ".tgz")
		base = strings.TrimSuffix(base, ".ccpm")
		name = base
	}
	if err := profile.ValidateName(name); err != nil {
		return fmt.Errorf("derived profile name %q is invalid; pass --profile: %w", name, err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if _, exists := cfg.Profiles[name]; exists {
		return fmt.Errorf("profile %q already exists (use --profile to pick another name)", name)
	}

	dstDir, err := profile.Create(name)
	if err != nil {
		return fmt.Errorf("creating profile directory: %w", err)
	}

	if err := extractBundle(bundlePath, dstDir); err != nil {
		_ = profile.Remove(name)
		return err
	}

	if err := settingsmerge.MaterializeAll(dstDir, name, ""); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not materialize settings: %v\n", err)
	}

	// Detect auth method from what restored on disk; default to oauth.
	authMethod := "oauth"
	if _, err := os.Stat(filepath.Join(dstDir, ".credentials.json")); err == nil {
		authMethod = "oauth"
	}

	if err := withConfigLock(func() error {
		freshCfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("reloading config: %w", err)
		}
		freshCfg.AddProfile(name, dstDir, authMethod)
		return config.Save(freshCfg)
	}); err != nil {
		_ = profile.Remove(name)
		return fmt.Errorf("saving config: %w", err)
	}

	green := color.New(color.FgGreen, color.Bold)
	green.Printf("✓ Restored profile %q from %s\n", name, bundlePath)
	fmt.Printf("Authenticate it with:\n  ccpm auth refresh %s\n", name)
	return nil
}

// extractBundle safely unpacks a gzipped tar into destDir. It rejects any entry
// whose path would escape destDir (path-traversal / "zip slip"), refuses
// absolute paths, and only writes regular files and directories.
func extractBundle(bundlePath, destDir string) error {
	f, err := os.Open(bundlePath)
	if err != nil {
		return fmt.Errorf("opening bundle: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("reading gzip: %w", err)
	}
	defer gz.Close()

	cleanDest := filepath.Clean(destDir)
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading archive: %w", err)
		}

		// Reject absolute paths and traversal before joining.
		clean := filepath.Clean(filepath.FromSlash(hdr.Name))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("refusing unsafe path in bundle: %q", hdr.Name)
		}
		target := filepath.Join(cleanDest, clean)
		// Defense in depth: ensure the joined path is still inside destDir.
		if target != cleanDest && !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("refusing path escaping profile dir: %q", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, config.DirPerm); err != nil {
				return fmt.Errorf("creating dir %q: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), config.DirPerm); err != nil {
				return fmt.Errorf("creating parent of %q: %w", target, err)
			}
			mode := os.FileMode(hdr.Mode).Perm()
			if mode == 0 {
				mode = config.FilePerm
			}
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
			if err != nil {
				return fmt.Errorf("creating %q: %w", target, err)
			}
			// Cap the copy to guard against a maliciously huge bundle entry.
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return fmt.Errorf("writing %q: %w", target, err)
			}
			out.Close()
		default:
			// Skip symlinks, devices, etc. — bundles only carry files/dirs.
			continue
		}
	}
	return nil
}
