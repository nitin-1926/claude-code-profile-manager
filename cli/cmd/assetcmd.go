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
	}

	if err := linkAssetToProfile(spec, sharedDst, p.Dir, storeEntry); err != nil {
		return fmt.Errorf("linking %s to profile: %w", spec.Name, err)
	}

	if existing := m.Find(assetID, spec.Kind); existing != nil {
		m.Remove(assetID, spec.Kind)
	}
	m.Add(manifest.Install{
		ID:       assetID,
		Kind:     spec.Kind,
		Scope:    manifest.ScopeProfile,
		Source:   abs,
		Profiles: []string{state.profile},
	})

	if err := manifest.Save(m); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	green.Printf("✓ %s %q installed for profile %q\n", titleCase(spec.Name), assetID, state.profile)
	if live {
		color.New(color.Faint).Println("  stored as a symlink — edits in the source will be picked up automatically")
	}
	return nil
}

func runAssetRemove(spec AssetSpec, state *assetState, assetID string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if !state.global && state.profile == "" {
		if err := pickAssetScope(spec, state, cfg); err != nil {
			return err
		}
	}

	m, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	// Resolve the on-disk entry name. The manifest stores the logical ID
	// (no extension for file-based assets); the store uses the full basename.
	storeEntry := findStoreEntry(spec, assetID)

	if state.global {
		for _, name := range config.ProfileNames(cfg) {
			p := cfg.Profiles[name]
			unlinkAssetFromProfile(spec, p.Dir, storeEntry)
		}
		storeRoot, _ := spec.SharedDir()
		os.RemoveAll(filepath.Join(storeRoot, storeEntry))
	} else {
		p, exists := cfg.Profiles[state.profile]
		if !exists {
			return fmt.Errorf("profile %q not found", state.profile)
		}
		unlinkAssetFromProfile(spec, p.Dir, storeEntry)
	}

	m.Remove(assetID, spec.Kind)
	if err := manifest.Save(m); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	color.New(color.FgGreen, color.Bold).Printf("✓ %s %q removed\n", titleCase(spec.Name), assetID)
	return nil
}

func runAssetList(spec AssetSpec) error {
	m, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	items := m.ListByKind(spec.Kind)
	_, projectEntries := discoverProjectAssets(spec.profileSubdir())

	if len(items) == 0 && len(projectEntries) == 0 {
		fmt.Printf("No %s installed. Install one with: ccpm %s add <path> --global\n", spec.plural(), spec.Name)
		return nil
	}

	bold := color.New(color.Bold).SprintFunc()
	header := strings.ToUpper(spec.Name)
	fmt.Printf("  %-20s %-10s %-10s %s\n", bold(header), bold("SOURCE"), bold("SCOPE"), bold("PROFILES"))
	fmt.Printf("  %s\n", strings.Repeat("─", 60))

	for _, it := range items {
		profiles := strings.Join(it.Profiles, ", ")
		if it.Scope == manifest.ScopeGlobal {
			profiles = "all"
		}
		fmt.Printf("  %-20s %-10s %-10s %s\n", it.ID, "ccpm", it.Scope, profiles)
	}
	for _, e := range projectEntries {
		fmt.Printf("  %-20s %-10s %-10s %s\n", e.ID, "project", "—", "—")
	}
	return nil
}

func runAssetLink(spec AssetSpec, state *assetState, assetID string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	p, exists := cfg.Profiles[state.profile]
	if !exists {
		return fmt.Errorf("profile %q not found", state.profile)
	}

	storeRoot, err := spec.SharedDir()
	if err != nil {
		return err
	}
	storeEntry := findStoreEntry(spec, assetID)
	sharedSrc := filepath.Join(storeRoot, storeEntry)

	if _, err := os.Stat(sharedSrc); os.IsNotExist(err) {
		return fmt.Errorf("%s %q not found in shared store. Install it first with: ccpm %s add <path> --global", spec.Name, assetID, spec.Name)
	}

	if err := linkAssetToProfile(spec, sharedSrc, p.Dir, storeEntry); err != nil {
		return fmt.Errorf("linking %s: %w", spec.Name, err)
	}

	color.New(color.FgGreen, color.Bold).Printf("✓ %s %q linked to profile %q\n", titleCase(spec.Name), assetID, state.profile)
	return nil
}

func linkAssetToProfile(spec AssetSpec, sharedSrc, profileDir, storeEntry string) error {
	dst := filepath.Join(profileDir, spec.profileSubdir(), storeEntry)
	return share.Link(sharedSrc, dst)
}

func unlinkAssetFromProfile(spec AssetSpec, profileDir, storeEntry string) {
	dst := filepath.Join(profileDir, spec.profileSubdir(), storeEntry)
	share.Unlink(dst)
}

// findStoreEntry resolves the manifest ID back to the on-disk entry name.
// File-based assets are stored with their extension (e.g. "foo.md"), while
// the manifest ID strips the extension ("foo"). If a directory entry exists
// with the same basename, prefer it; otherwise scan the store for the first
// entry whose stem matches.
func findStoreEntry(spec AssetSpec, assetID string) string {
	storeRoot, err := spec.SharedDir()
	if err != nil {
		return assetID
	}
	// Exact-name hit (covers directories and files where ID == basename).
	if _, err := os.Stat(filepath.Join(storeRoot, assetID)); err == nil {
		return assetID
	}
	entries, err := os.ReadDir(storeRoot)
	if err != nil {
		return assetID
	}
	for _, e := range entries {
		stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		if stem == assetID {
			return e.Name()
		}
	}
	return assetID
}

func pickAssetScope(spec AssetSpec, state *assetState, cfg *config.Config) error {
	scope, err := picker.Select("Install scope", []picker.Option{
		{Value: "global", Label: "Global", Description: "all profiles now and any created later"},
		{Value: "profile", Label: "A single profile", Description: "pick one profile"},
	})
	if err != nil {
		if errors.Is(err, picker.ErrNonInteractive) {
			return fmt.Errorf("specify --global or --profile <name>")
		}
		return err
	}
	if scope == "global" {
		state.global = true
		return nil
	}
	names := config.ProfileNames(cfg)
	if len(names) == 0 {
		return fmt.Errorf("no profiles exist yet — create one with `ccpm add <name>`")
	}
	opts := make([]picker.Option, len(names))
	for i, n := range names {
		opts[i] = picker.Option{Value: n, Label: n}
	}
	name, err := picker.Select("Target profile", opts)
	if err != nil {
		return err
	}
	state.profile = name
	return nil
}

func resolveAssetStrategy(spec AssetSpec, state *assetState, src string) (bool, error) {
	isLinkDir, err := filetree.SymlinkToDirectory(src)
	if err != nil {
		return false, fmt.Errorf("inspecting source: %w", err)
	}
	if !isLinkDir {
		if state.liveSymlink {
			fmt.Fprintln(os.Stderr, "  Note: source is not a symlinked directory — copying instead.")
		}
		return false, nil
	}
	if state.liveSymlink {
		return true, nil
	}
	if state.copy {
		return false, nil
	}

	choice, err := picker.Select(
		"The source is a symlinked directory. How should ccpm install it?",
		[]picker.Option{
			{Value: "symlink", Label: "Symlink (recommended)", Description: "edits in the source repo stay live across profiles"},
			{Value: "copy", Label: "Copy", Description: "snapshot the current tree; future edits stay local"},
		},
	)
	if err != nil {
		if errors.Is(err, picker.ErrNonInteractive) {
			return false, nil
		}
		return false, err
	}
	return choice == "symlink", nil
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
