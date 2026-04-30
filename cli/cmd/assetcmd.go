package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/filetree"
	"github.com/nitin-1926/ccpm/internal/manifest"
	"github.com/nitin-1926/ccpm/internal/picker"
	"github.com/nitin-1926/ccpm/internal/share"
)

// AssetSpec describes a dedupable ccpm-managed asset kind (agents, commands,
// rules, and — by construction — skills). One spec drives an entire `ccpm
// <name>` command tree via NewAssetCmd.
type AssetSpec struct {
	// Name is the user-facing singular noun used in the subcommand (e.g.
	// "agent", "command", "rule"). The matching plural is Name + "s" unless
	// Plural is set explicitly.
	Name string
	// Plural overrides the default pluralization when it differs from Name+"s".
	Plural string
	// Kind is the manifest AssetKind recorded for installs.
	Kind manifest.AssetKind
	// MarkerFile, if set, must exist inside the source directory for an add to
	// succeed (e.g. "SKILL.md" for skills). Empty means no marker is required
	// and the source may be either a file or a directory.
	MarkerFile string
	// SharedDir returns the share-store root for this asset kind
	// (e.g. share.AgentsDir).
	SharedDir func() (string, error)
	// ProfileSubdir is the subdirectory name inside a profile dir where
	// symlinks for this asset live (e.g. "agents"). Defaults to Plural.
	ProfileSubdir string
}

func (s AssetSpec) plural() string {
	if s.Plural != "" {
		return s.Plural
	}
	return s.Name + "s"
}

func (s AssetSpec) profileSubdir() string {
	if s.ProfileSubdir != "" {
		return s.ProfileSubdir
	}
	return s.plural()
}

// assetState holds the cobra flag-bound variables for one asset command tree.
// Each NewAssetCmd invocation creates its own state so commands don't share
// flag values by accident.
type assetState struct {
	profile     string
	global      bool
	liveSymlink bool
	copy        bool
}

// NewAssetCmd builds and returns a cobra command tree for the given asset
// spec: `ccpm <name> {add,remove,list,link}`. The caller is responsible for
// registering the returned command with rootCmd.
func NewAssetCmd(spec AssetSpec) *cobra.Command {
	state := &assetState{}

	root := &cobra.Command{
		Use:   spec.Name,
		Short: fmt.Sprintf("Manage Claude Code %s across profiles", spec.plural()),
	}

	addCmd := &cobra.Command{
		Use:   "add <path>",
		Short: fmt.Sprintf("Install a %s from a local path", spec.Name),
		Long: fmt.Sprintf(`Install a %s into one or all profiles.

Use --global to install for all profiles, or --profile to target one.
When the source is a symlink to a directory (e.g. inside an external repo),
ccpm asks whether to keep it as a live symlink or copy a snapshot. Pass
--live-symlink or --copy to skip the prompt.`, spec.Name),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAssetAdd(spec, state, args[0])
		},
	}
	addCmd.Flags().BoolVar(&state.global, "global", false, "install for all profiles")
	addCmd.Flags().StringVar(&state.profile, "profile", "", "install for a specific profile")
	addCmd.Flags().BoolVar(&state.liveSymlink, "live-symlink", false, "if the source is a symlinked directory, keep the link")
	addCmd.Flags().BoolVar(&state.copy, "copy", false, "always copy the source, even when it is a symlink (snapshot)")

	removeCmd := &cobra.Command{
		Use:     "remove <name>",
		Short:   fmt.Sprintf("Remove a %s", spec.Name),
		Aliases: []string{"rm"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAssetRemove(spec, state, args[0])
		},
	}
	removeCmd.Flags().BoolVar(&state.global, "global", false, "remove from all profiles")
	removeCmd.Flags().StringVar(&state.profile, "profile", "", "remove from a specific profile")

	listCmd := &cobra.Command{
		Use:     "list",
		Short:   fmt.Sprintf("List installed %s", spec.plural()),
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAssetList(spec)
		},
	}

	linkCmd := &cobra.Command{
		Use:   "link <name> --profile <name>",
		Short: fmt.Sprintf("Link a shared %s into a specific profile", spec.Name),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAssetLink(spec, state, args[0])
		},
	}
	linkCmd.Flags().StringVar(&state.profile, "profile", "", "target profile (required)")
	_ = linkCmd.MarkFlagRequired("profile")

	root.AddCommand(addCmd)
	root.AddCommand(removeCmd)
	root.AddCommand(listCmd)
	root.AddCommand(linkCmd)
	return root
}

func runAssetAdd(spec AssetSpec, state *assetState, srcPath string) error {
	abs, err := filepath.Abs(srcPath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("source path %q does not exist: %w", abs, err)
	}

	if spec.MarkerFile != "" {
		if !info.IsDir() {
			return fmt.Errorf("source %q must be a directory containing %s", abs, spec.MarkerFile)
		}
		marker := filepath.Join(abs, spec.MarkerFile)
		if _, err := os.Stat(marker); os.IsNotExist(err) {
			return fmt.Errorf("no %s found in %q — not a valid %s directory", spec.MarkerFile, abs, spec.Name)
		}
	}

	assetID := strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))
	if info.IsDir() {
		assetID = filepath.Base(abs)
	}

	if state.liveSymlink && state.copy {
		return fmt.Errorf("--live-symlink and --copy are mutually exclusive")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if !state.global && state.profile == "" {
		if err := pickAssetScope(spec, state, cfg); err != nil {
			return err
		}
	}

	if err := config.EnsureDirs(); err != nil {
		return err
	}
	if err := share.EnsureDirs(); err != nil {
		return err
	}

	storeRoot, err := spec.SharedDir()
	if err != nil {
		return err
	}
	// Preserve the file extension in the stored entry for file-based assets
	// so Claude Code still recognizes them (.md, etc.).
	storeEntry := filepath.Base(abs)
	sharedDst := filepath.Join(storeRoot, storeEntry)

	live, err := resolveAssetStrategy(spec, state, abs)
	if err != nil {
		return err
	}

	if _, err := filetree.SeedStoreEntry(abs, sharedDst, live); err != nil {
		return fmt.Errorf("populating shared store: %w", err)
	}

	m, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	green := color.New(color.FgGreen, color.Bold)

	if state.global {
		profiles := config.ProfileNames(cfg)
		for _, name := range profiles {
			p := cfg.Profiles[name]
			if err := linkAssetToProfile(spec, sharedDst, p.Dir, storeEntry); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not link %s to profile %q: %v\n", spec.Name, name, err)
			}
		}

		if existing := m.Find(assetID, spec.Kind); existing != nil {
			m.Remove(assetID, spec.Kind)
		}
		m.Add(manifest.Install{
			ID:       assetID,
			Kind:     spec.Kind,
			Scope:    manifest.ScopeGlobal,
			Source:   abs,
			Profiles: profiles,
		})

		if err := manifest.Save(m); err != nil {
			return fmt.Errorf("saving manifest: %w", err)
		}

		green.Printf("✓ %s %q installed globally (%d profiles)\n", titleCase(spec.Name), assetID, len(profiles))
		if live {
			color.New(color.Faint).Println("  stored as a symlink — edits in the source will be picked up automatically")
		}
		return nil
	}

	p, exists := cfg.Profiles[state.profile]
	if !exists {
		return fmt.Errorf("profile %q not found", state.profile)
