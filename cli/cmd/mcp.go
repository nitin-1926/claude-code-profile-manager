package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/manifest"
	"github.com/nitin-1926/ccpm/internal/picker"
	"github.com/nitin-1926/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/ccpm/internal/share"
)

var (
	mcpProfile string
	mcpGlobal  bool
	mcpCommand string
	mcpArgs    []string
	mcpEnv     []string
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Manage MCP servers across profiles",
}

var mcpAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add an MCP server to one or all profiles",
	Long: `Add an MCP server definition.

The server is stored as a JSON fragment and merged into the profile's
settings.json at launch time.

Examples:
  ccpm mcp add github --command npx --args '-y,@modelcontextprotocol/server-github' --global
  ccpm mcp add filesystem --command npx --args '-y,@modelcontextprotocol/server-filesystem,/home' --profile work`,
	Args: cobra.ExactArgs(1),
	RunE: runMCPAdd,
}

var mcpRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Short:   "Remove an MCP server",
	Aliases: []string{"rm"},
	Args:    cobra.ExactArgs(1),
	RunE:    runMCPRemove,
}

var mcpListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List installed MCP servers",
	Aliases: []string{"ls"},
	RunE:    runMCPList,
}

var mcpImportCmd = &cobra.Command{
	Use:   "import <file.json>",
	Short: "Import MCP server definitions from a JSON file",
	Long: `Import one or more MCP server definitions from a JSON file.

The file should contain an object with server names as keys:
  {
    "github": { "command": "npx", "args": ["-y", "@modelcontextprotocol/server-github"] },
    "slack":  { "command": "npx", "args": ["-y", "@modelcontextprotocol/server-slack"] }
  }`,
	Args: cobra.ExactArgs(1),
	RunE: runMCPImport,
}

func init() {
	mcpAddCmd.Flags().BoolVar(&mcpGlobal, "global", false, "add for all profiles")
	mcpAddCmd.Flags().StringVar(&mcpProfile, "profile", "", "add for a specific profile")
	mcpAddCmd.Flags().StringVar(&mcpCommand, "command", "", "server command (e.g. npx, node)")
	mcpAddCmd.Flags().StringSliceVar(&mcpArgs, "args", nil, "command arguments (comma-separated)")
	mcpAddCmd.Flags().StringSliceVar(&mcpEnv, "env", nil, "environment variables (KEY=VALUE, comma-separated)")
	_ = mcpAddCmd.MarkFlagRequired("command")

	mcpRemoveCmd.Flags().BoolVar(&mcpGlobal, "global", false, "remove from all profiles")
	mcpRemoveCmd.Flags().StringVar(&mcpProfile, "profile", "", "remove from a specific profile")

	mcpImportCmd.Flags().BoolVar(&mcpGlobal, "global", false, "import for all profiles")
	mcpImportCmd.Flags().StringVar(&mcpProfile, "profile", "", "import for a specific profile")

	mcpCmd.AddCommand(mcpAddCmd)
	mcpCmd.AddCommand(mcpRemoveCmd)
	mcpCmd.AddCommand(mcpListCmd)
	mcpCmd.AddCommand(mcpImportCmd)
	rootCmd.AddCommand(mcpCmd)
}

func runMCPAdd(cmd *cobra.Command, args []string) error {
	serverName := args[0]

	if !mcpGlobal && mcpProfile == "" {
		if err := pickMCPScope(); err != nil {
			return err
		}
	}

	serverDef := map[string]interface{}{
		"command": mcpCommand,
	}
	if len(mcpArgs) > 0 {
		serverDef["args"] = mcpArgs
	}
	if len(mcpEnv) > 0 {
		envMap := make(map[string]string)
		for _, e := range mcpEnv {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				envMap[parts[0]] = parts[1]
			}
		}
		serverDef["env"] = envMap
	}

	if err := share.EnsureDirs(); err != nil {
		return err
	}

	green := color.New(color.FgGreen, color.Bold)

	if mcpGlobal {
		if err := writeMCPFragment("global", serverName, serverDef); err != nil {
			return err
		}

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		m, err := manifest.Load()
		if err != nil {
			return fmt.Errorf("loading manifest: %w", err)
		}
		if existing := m.Find(serverName, manifest.KindMCP); existing != nil {
			m.Remove(serverName, manifest.KindMCP)
		}
		m.Add(manifest.Install{
			ID:       serverName,
			Kind:     manifest.KindMCP,
			Scope:    manifest.ScopeGlobal,
			Profiles: config.ProfileNames(cfg),
		})
		if err := manifest.Save(m); err != nil {
			return fmt.Errorf("saving manifest: %w", err)
		}

		green.Printf("✓ MCP server %q added globally\n", serverName)
	} else {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		if _, exists := cfg.Profiles[mcpProfile]; !exists {
			return fmt.Errorf("profile %q not found", mcpProfile)
		}

		if err := writeMCPFragment(mcpProfile, serverName, serverDef); err != nil {
			return err
		}

		m, err := manifest.Load()
		if err != nil {
			return fmt.Errorf("loading manifest: %w", err)
		}
		if existing := m.Find(serverName, manifest.KindMCP); existing != nil {
			m.Remove(serverName, manifest.KindMCP)
		}
		m.Add(manifest.Install{
			ID:       serverName,
			Kind:     manifest.KindMCP,
			Scope:    manifest.ScopeProfile,
			Profiles: []string{mcpProfile},
		})
		if err := manifest.Save(m); err != nil {
			return fmt.Errorf("saving manifest: %w", err)
		}

		green.Printf("✓ MCP server %q added for profile %q\n", serverName, mcpProfile)
	}

	return nil
}

func runMCPRemove(cmd *cobra.Command, args []string) error {
	serverName := args[0]

	if !mcpGlobal && mcpProfile == "" {
		if err := pickMCPScope(); err != nil {
			return err
		}
	}

	mcpDir, err := share.MCPDir()
	if err != nil {
		return err
	}

	if mcpGlobal {
		removeMCPFromFragment(filepath.Join(mcpDir, "global.json"), serverName)
	} else {
		removeMCPFromFragment(filepath.Join(mcpDir, mcpProfile+".json"), serverName)
	}

	m, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}
	m.Remove(serverName, manifest.KindMCP)
	if err := manifest.Save(m); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	color.New(color.FgGreen, color.Bold).Printf("✓ MCP server %q removed\n", serverName)
	return nil
}

func runMCPList(cmd *cobra.Command, args []string) error {
	m, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	servers := m.ListByKind(manifest.KindMCP)
	if len(servers) == 0 {
		fmt.Println("No MCP servers installed. Add one with: ccpm mcp add <name> --command <cmd> --global")
		return nil
	}

	bold := color.New(color.Bold).SprintFunc()
	fmt.Printf("  %-20s %-10s %s\n", bold("SERVER"), bold("SCOPE"), bold("PROFILES"))
	fmt.Printf("  %s\n", strings.Repeat("─", 50))

	for _, s := range servers {
		profiles := strings.Join(s.Profiles, ", ")
		if s.Scope == manifest.ScopeGlobal {
			profiles = "all"
		}
		fmt.Printf("  %-20s %-10s %s\n", s.ID, s.Scope, profiles)
	}

	return nil
}

func runMCPImport(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	if !mcpGlobal && mcpProfile == "" {
		if err := pickMCPScope(); err != nil {
			return err
		}
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filePath, err)
	}

	var servers map[string]interface{}
	if err := json.Unmarshal(data, &servers); err != nil {
		return fmt.Errorf("parsing %s: %w", filePath, err)
	}

	if err := share.EnsureDirs(); err != nil {
		return err
	}

	target := "global"
	if !mcpGlobal {
		target = mcpProfile
	}

	for serverName, serverDef := range servers {
		defMap, ok := serverDef.(map[string]interface{})
		if !ok {
			fmt.Fprintf(os.Stderr, "Warning: skipping %q (not a valid server definition)\n", serverName)
			continue
		}
		if err := writeMCPFragment(target, serverName, defMap); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not write %q: %v\n", serverName, err)
		}
	}

	m, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	scope := manifest.ScopeGlobal
	profiles := config.ProfileNames(cfg)
	if !mcpGlobal {
		scope = manifest.ScopeProfile
		profiles = []string{mcpProfile}
	}

	for serverName := range servers {
		if existing := m.Find(serverName, manifest.KindMCP); existing != nil {
			m.Remove(serverName, manifest.KindMCP)
		}
		m.Add(manifest.Install{
			ID:       serverName,
			Kind:     manifest.KindMCP,
			Scope:    scope,
			Profiles: profiles,
		})
	}

	if err := manifest.Save(m); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	color.New(color.FgGreen, color.Bold).Printf("✓ Imported %d MCP servers from %s\n", len(servers), filePath)
	return nil
}

// pickMCPScope resolves --global / --profile when neither was given, using an
// interactive picker in a TTY and the existing required-flag error in CI.
func pickMCPScope() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
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
		mcpGlobal = true
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
	mcpProfile = name
	return nil
}

// writeMCPFragment writes or updates a single server definition inside
// the named fragment file (e.g. share/mcp/global.json or share/mcp/<profile>.json).
func writeMCPFragment(target, serverName string, serverDef map[string]interface{}) error {
	mcpDir, err := share.MCPDir()
	if err != nil {
		return err
	}

	fragPath := filepath.Join(mcpDir, target+".json")
	frag, err := settingsmerge.LoadJSON(fragPath)
	if err != nil {
		return fmt.Errorf("loading MCP fragment: %w", err)
	}

	frag[serverName] = serverDef

	return settingsmerge.WriteJSON(fragPath, frag)
}

func removeMCPFromFragment(fragPath, serverName string) {
	frag, err := settingsmerge.LoadJSON(fragPath)
	if err != nil {
		return
	}
	delete(frag, serverName)
	settingsmerge.WriteJSON(fragPath, frag)
}
