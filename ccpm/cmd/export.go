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

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/atomicwrite"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/profile"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
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
	exportIncludeHistory     bool
)

// sessionData are top-level profile entries Claude Code writes as you work:
// transcripts, todos, shell snapshots, prompt history. They can hold pasted
// secrets and aren't needed to reproduce a setup, so export leaves them out
// unless --include-history.
var sessionData = map[string]bool{
	"projects":        true,
	"todos":           true,
	"shell-snapshots": true,
	"history.jsonl":   true,
	"file-history":    true,
	"session-env":     true,
	"paste-cache":     true,
	"statsig":         true,
	"debug":           true,
	"ide":             true,
	"telemetry":       true,
}

// linkedAssetDirs hold the profile's symlinks into the share store. Export
// bundles the linked content so the archive is self-contained.
var linkedAssetDirs = map[string]bool{
	"skills":   true,
	"agents":   true,
	"commands": true,
	"rules":    true,
	"hooks":    true,
}

// bundleFragmentsDir is the archive directory that carries the profile's
// share-store fragments, which live outside the profile dir.
const bundleFragmentsDir = ".ccpm-bundle"

// bundleFragments maps each archive name under bundleFragmentsDir to the
// share-store fragment of profileName it carries.
func bundleFragments(profileName string) (map[string]string, error) {
	settingsDir, err := share.SettingsDir()
	if err != nil {
		return nil, err
	}
	mcpDir, err := share.MCPDir()
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"settings.json":       filepath.Join(settingsDir, profileName+".json"),
		"settings.owned.json": filepath.Join(settingsDir, profileName+".owned.json"),
		"mcp.json":            filepath.Join(mcpDir, profileName+".json"),
	}, nil
}

// Extraction limits: a bundle is untrusted input and gzip compresses runs of
// zeros ~1000:1, so a small file could otherwise fill the disk. Vars so
// tests can lower them.
var (
	maxBundleFileBytes  int64 = 512 << 20 // 512 MiB per entry
	maxBundleTotalBytes int64 = 4 << 30   // 4 GiB per bundle
)

var exportCmd = &cobra.Command{
	Use:   "export <profile>",
	Short: "Export a profile (assets + settings) to a portable .tar.gz bundle",
	Long: `Bundles a profile into a single .tar.gz you can copy to another machine and
restore with 'ccpm import-bundle'.

Included: the profile's settings.json and plugin metadata; its skills, agents,
commands, rules and hooks (the content behind symlinks into ~/.ccpm/share is
copied in; links to anything outside the share store, such as ~/.claude host
assets, are left out); and its ccpm settings and MCP fragments
(~/.ccpm/share/settings/<profile>.json and share/mcp/<profile>.json).

Left out by default:
  - Session history (projects/ transcripts, todos/, shell-snapshots/,
    history.jsonl, ...). It can contain secrets you pasted into a session.
    Use --include-history for a same-user machine move.
  - Credentials. OS-keychain tokens are machine-bound, and .credentials.json /
    .claude.json hold secrets. Use --include-credentials only for a trusted
    same-user move (e.g. Linux machine migration).

A bundle made with either flag is sensitive: keep it private. Even a default
bundle carries your hooks, MCP server definitions and settings env values, so
review it before sharing.`,
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
	exportCmd.Flags().BoolVar(&exportIncludeHistory, "include-history", false, "include session history: transcripts, todos, shell snapshots (may contain pasted secrets)")
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

	// Resolved so symlink targets (also resolved) compare correctly when the
	// home dir itself sits behind a link (e.g. macOS /var → /private/var).
	shareRoot := ""
	if d, err := share.Dir(); err == nil {
		if r, err := filepath.EvalSymlinks(d); err == nil {
			shareRoot = r
		}
	}

	var skippedHistory, skippedLinks []string
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
		arcName := filepath.ToSlash(rel)
		top, _, _ := strings.Cut(arcName, "/")
		skip := top == bundleFragmentsDir ||
			(!exportIncludeHistory && sessionData[top]) ||
			(!exportIncludeCredentials && credentialFiles[filepath.Base(path)])
		if skip {
			if sessionData[top] && arcName == top {
				skippedHistory = append(skippedHistory, top)
			}
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// Profile assets are symlinks into the share store: bundle the
			// linked content. Other links (plugin caches, anything pointing
			// outside ~/.ccpm/share) are left out — following them could
			// sweep arbitrary files into the archive.
			if linkedAssetDirs[filepath.Dir(rel)] {
				if target, err := filepath.EvalSymlinks(path); err == nil && shareRoot != "" && isWithinDir(shareRoot, target) {
					return addTreeToTar(tw, target, arcName)
				}
				skippedLinks = append(skippedLinks, arcName)
			}
			return nil
		}
		return addToTar(tw, path, info, arcName)
	})
	if walkErr == nil {
		walkErr = addFragmentsToTar(tw, name)
	}
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
	if len(skippedHistory) > 0 {
		fmt.Printf("Session history left out (%s); pass --include-history to bundle it.\n", strings.Join(skippedHistory, ", "))
	}
	if len(skippedLinks) > 0 {
		fmt.Printf("Links outside the ccpm share store left out (e.g. ~/.claude host assets): %s\n", strings.Join(skippedLinks, ", "))
	}
	if !exportIncludeCredentials {
		fmt.Println("Credentials were excluded. Restore, then run `ccpm auth refresh <name>`.")
	} else {
		color.New(color.FgYellow).Println("Bundle contains credentials — keep it private.")
	}
	return nil
}

func isWithinDir(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && filepath.IsLocal(rel)
}

// addToTar writes one regular file or directory header (plus content) under
// the archive name.
func addToTar(tw *tar.Writer, path string, info os.FileInfo, name string) error {
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	hdr.Name = name
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
}

// addTreeToTar archives the file or directory at root under prefix. Nested
// symlinks and special files are skipped, so a linked asset can't pull in
// anything outside itself.
func addTreeToTar(tw *tar.Writer, root, prefix string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := prefix
		if rel != "." {
			name += "/" + filepath.ToSlash(rel)
		}
		return addToTar(tw, path, info, name)
	})
}

// addFragmentsToTar carries the profile's share-store settings/MCP fragments
// under bundleFragmentsDir. Missing fragments are simply absent.
func addFragmentsToTar(tw *tar.Writer, profileName string) error {
	frags, err := bundleFragments(profileName)
	if err != nil {
		return err
	}
	for archive, path := range frags {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if err := addToTar(tw, path, info, bundleFragmentsDir+"/"+archive); err != nil {
			return err
		}
	}
	return nil
}

// restoreBundleFragments installs the fragments a bundle carries under
// bundleFragmentsDir as profileName's share-store fragments, in one
// atomicwrite transaction, then drops the staging dir. Returns the paths
// written so a later failure can undo them. Old bundles carry none.
func restoreBundleFragments(profileDir, profileName string) ([]string, error) {
	staged := filepath.Join(profileDir, bundleFragmentsDir)
	frags, err := bundleFragments(profileName)
	if err != nil {
		return nil, err
	}
	var changes []atomicwrite.FileChange
	var written []string
	for archive, dest := range frags {
		data, err := os.ReadFile(filepath.Join(staged, archive))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("reading bundled %s: %w", archive, err)
		}
		changes = append(changes, atomicwrite.WriteFile(dest, data, config.FilePerm))
		written = append(written, dest)
	}
	if err := atomicwrite.Apply(changes); err != nil {
		return nil, fmt.Errorf("restoring settings/MCP fragments: %w", err)
	}
	if err := os.RemoveAll(staged); err != nil {
		return written, fmt.Errorf("removing %s: %w", staged, err)
	}
	return written, nil
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

	fragments, err := restoreBundleFragments(dstDir, name)
	undo := func() {
		for _, f := range fragments {
			_ = os.Remove(f)
		}
		_ = profile.Remove(name)
	}
	if err != nil {
		undo()
		return err
	}

	if err := settingsmerge.MaterializeAll(dstDir, name, ""); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not materialize settings: %v\n", err)
	}

	// Always oauth: an API key lives in the OS keychain, which never travels
	// in a bundle, so a restored profile can only be re-authed via OAuth or
	// switched with `ccpm auth refresh`.
	if err := withConfigLock(func() error {
		freshCfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("reloading config: %w", err)
		}
		freshCfg.AddProfile(name, dstDir, "oauth")
		return config.Save(freshCfg)
	}); err != nil {
		undo()
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
	var total int64
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
			// Copy at most one byte past the limit so an oversize entry is
			// detected without writing it all.
			n, err := io.CopyN(out, tr, maxBundleFileBytes+1)
			out.Close()
			if err != nil && err != io.EOF {
				return fmt.Errorf("writing %q: %w", target, err)
			}
			if n > maxBundleFileBytes {
				return fmt.Errorf("bundle entry %q exceeds the %d MiB per-file limit", hdr.Name, maxBundleFileBytes>>20)
			}
			if total += n; total > maxBundleTotalBytes {
				return fmt.Errorf("bundle exceeds the %d MiB total extraction limit", maxBundleTotalBytes>>20)
			}
		default:
			// Skip symlinks, devices, etc. — bundles only carry files/dirs.
			continue
		}
	}
	return nil
}
