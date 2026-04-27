package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	claudepkg "github.com/nitin-1926/ccpm/internal/claude"
	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/manifest"
	"github.com/nitin-1926/ccpm/internal/picker"
	"github.com/nitin-1926/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/ccpm/internal/share"
)

const (
	mcpScopeGlobal  = "global"
	mcpScopeProfile = "profile"
	mcpScopeProject = "project"
)

// mcpState holds the cobra flag-bound values for the `ccpm mcp` command tree.
// One state is created per invocation of newMCPCmd so tests and library uses
// don't share flag values.
type mcpState struct {
	profile    string
	global     bool
	scope      string
	projectDir string
	transport  string
	url        string
	headers    []string
	command    string
	args       []string
	env        []string
}

func newMCPCmd() *cobra.Command {
	state := &mcpState{}

	root := &cobra.Command{
		Use:   "mcp",
		Short: "Manage MCP servers across profiles and projects",
	}

	addCmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add an MCP server (stdio, http, or sse)",
		Long: `Add an MCP server to a profile, all profiles, or a project .mcp.json.

The --scope flag selects the destination:
  --scope global   — all ccpm profiles (shared fragment)
  --scope profile  — one profile (requires --profile)
  --scope project  — project-local .mcp.json (discovered from CWD or --project-dir)

--transport selects the connection shape:
  stdio (default) — local process; use --command and --args
  http            — remote HTTP MCP; use --url (and optional --header)
  sse             — remote SSE MCP; use --url (and optional --header)

Examples:
  ccpm mcp add github --scope global --command npx --args '-y,@modelcontextprotocol/server-github'
  ccpm mcp add supabase --scope profile --profile work --transport http --url https://mcp.supabase.com/mcp --header 'Authorization=Bearer $SUPABASE_TOKEN'
  ccpm mcp add repo-tools --scope project --command node --args './mcp/index.js'`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return mcpPreRun(state)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPAdd(state, args)
		},
	}
	addCmd.Flags().StringVar(&state.scope, "scope", "", "scope: global | profile | project")
	addCmd.Flags().BoolVar(&state.global, "global", false, "alias for --scope global")
	addCmd.Flags().StringVar(&state.profile, "profile", "", "profile name (required for --scope profile)")
	addCmd.Flags().StringVar(&state.projectDir, "project-dir", "", "project root for --scope project (defaults to nearest .claude/.mcp.json ancestor of CWD)")
	addCmd.Flags().StringVar(&state.transport, "transport", "stdio", "transport: stdio | http | sse")
	addCmd.Flags().StringVar(&state.url, "url", "", "server URL (required for http/sse)")
	addCmd.Flags().StringSliceVar(&state.headers, "header", nil, "HTTP header for http/sse transports (KEY=VALUE, repeatable)")
	addCmd.Flags().StringVar(&state.command, "command", "", "server command (required for stdio; e.g. npx, node)")
	addCmd.Flags().StringSliceVar(&state.args, "args", nil, "command arguments for stdio (comma-separated)")
	addCmd.Flags().StringSliceVar(&state.env, "env", nil, "environment variables for stdio (KEY=VALUE, comma-separated)")

	removeCmd := &cobra.Command{
		Use:     "remove <name>",
		Short:   "Remove an MCP server",
		Aliases: []string{"rm"},
		Args:    cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return mcpPreRun(state)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPRemove(state, args)
		},
	}
	removeCmd.Flags().StringVar(&state.scope, "scope", "", "scope: global | profile | project")
	removeCmd.Flags().BoolVar(&state.global, "global", false, "alias for --scope global")
	removeCmd.Flags().StringVar(&state.profile, "profile", "", "profile name (required for --scope profile)")
	removeCmd.Flags().StringVar(&state.projectDir, "project-dir", "", "project root for --scope project")

	listCmd := &cobra.Command{
		Use:     "list",
		Short:   "List MCP servers from every source (ccpm, host, project)",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPList()
		},
	}

	importCmd := &cobra.Command{
		Use:   "import <file.json>",
		Short: "Import MCP server definitions from a JSON file",
		Long: `Import one or more MCP server definitions from a JSON file.

The file should contain either { "mcpServers": {...} } or a top-level object
whose keys are server names:
  {
    "github": { "command": "npx", "args": ["-y", "@modelcontextprotocol/server-github"] },
    "supabase": { "type": "http", "url": "https://mcp.supabase.com/mcp" }
  }`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return mcpPreRun(state)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPImport(state, args)
		},
	}
	importCmd.Flags().StringVar(&state.scope, "scope", "", "scope: global | profile | project")
	importCmd.Flags().BoolVar(&state.global, "global", false, "alias for --scope global")
	importCmd.Flags().StringVar(&state.profile, "profile", "", "profile name (required for --scope profile)")
	importCmd.Flags().StringVar(&state.projectDir, "project-dir", "", "project root for --scope project")

	authCmd := &cobra.Command{
		Use:   "auth <server-name>",
		Short: "Complete OAuth for a remote MCP server in a profile's scope",
		Long: `Trigger the native Claude Code OAuth flow for a remote MCP server.

ccpm doesn't own OAuth tokens — Claude Code stores them in the OS keychain or
CLAUDE_CONFIG_DIR/.credentials.json. This command spawns the native claude
binary with CLAUDE_CONFIG_DIR pinned to the profile so tokens land in the
right place and only that profile can use them.

Example:
  ccpm mcp auth supabase --profile work`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPAuth(state, args)
		},
	}
	authCmd.Flags().StringVar(&state.profile, "profile", "", "profile to authenticate under (required)")
	_ = authCmd.MarkFlagRequired("profile")

	root.AddCommand(addCmd, removeCmd, listCmd, importCmd, authCmd)
	return root
}

func init() {
	rootCmd.AddCommand(newMCPCmd())
}

// mcpPreRun normalizes --scope / --global / --profile into state.scope, asking
// interactively when nothing is provided.
func mcpPreRun(state *mcpState) error {
	if state.global && state.profile != "" {
		return fmt.Errorf("--global and --profile are mutually exclusive")
	}
	if state.scope == "" {
		switch {
		case state.global:
			state.scope = mcpScopeGlobal
		case state.profile != "":
			state.scope = mcpScopeProfile
		}
	} else {
		if state.global && state.scope != mcpScopeGlobal {
			return fmt.Errorf("--global conflicts with --scope %q", state.scope)
		}
		if state.profile != "" && state.scope != mcpScopeProfile {
			return fmt.Errorf("--profile conflicts with --scope %q", state.scope)
		}
	}

	if state.scope == "" {
		return pickMCPScope(state)
	}
	switch state.scope {
	case mcpScopeGlobal, mcpScopeProfile, mcpScopeProject:
		return nil
	default:
		return fmt.Errorf("invalid --scope %q (expected global|profile|project)", state.scope)
	}
}

// resolveProjectRoot figures out where to write .mcp.json for project scope.
func resolveProjectRoot(state *mcpState) (string, error) {
	if state.projectDir != "" {
		abs, err := filepath.Abs(state.projectDir)
		if err != nil {
			return "", fmt.Errorf("resolving --project-dir: %w", err)
		}
		return abs, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting CWD for project scope: %w", err)
	}
	root := settingsmerge.FindProjectRoot(cwd)
	if root == "" {
		return cwd, nil
	}
	return root, nil
}

func runMCPAdd(state *mcpState, args []string) error {
	serverName := args[0]

	serverDef, err := buildServerDef(state)
	if err != nil {
		return err
	}

	if err := share.EnsureDirs(); err != nil {
		return err
	}

	green := color.New(color.FgGreen, color.Bold)

	switch state.scope {
	case mcpScopeGlobal:
		if err := writeMCPFragment("global", serverName, serverDef); err != nil {
			return err
		}
		if err := recordMCPInstall(serverName, manifest.ScopeGlobal, nil); err != nil {
			return err
		}
		green.Printf("✓ MCP server %q added globally\n", serverName)

	case mcpScopeProfile:
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		if _, exists := cfg.Profiles[state.profile]; !exists {
			return fmt.Errorf("profile %q not found", state.profile)
		}
		if err := writeMCPFragment(state.profile, serverName, serverDef); err != nil {
			return err
		}
		if err := recordMCPInstall(serverName, manifest.ScopeProfile, []string{state.profile}); err != nil {
			return err
		}
		green.Printf("✓ MCP server %q added for profile %q\n", serverName, state.profile)

	case mcpScopeProject:
		root, err := resolveProjectRoot(state)
		if err != nil {
			return err
		}
		if err := writeProjectMCPServer(root, serverName, serverDef); err != nil {
			return err
		}
		green.Printf("✓ MCP server %q added to %s/.mcp.json\n", serverName, root)
	}

	return nil
}

// buildServerDef constructs the MCP server definition payload.
func buildServerDef(state *mcpState) (map[string]interface{}, error) {
	transport := strings.ToLower(strings.TrimSpace(state.transport))
	if transport == "" {
		transport = "stdio"
	}

	switch transport {
	case "stdio":
		if state.command == "" {
			return nil, fmt.Errorf("--command is required for --transport stdio")
		}
		if state.url != "" {
			return nil, fmt.Errorf("--url is only valid for --transport http|sse")
		}
		def := map[string]interface{}{
			"type":    "stdio",
			"command": state.command,
		}
		if len(state.args) > 0 {
			def["args"] = toInterfaceSlice(state.args)
		}
		if len(state.env) > 0 {
			envMap, err := parseKVSlice(state.env, "--env")
			if err != nil {
				return nil, err
			}
			def["env"] = envMap
		}
		return def, nil

	case "http", "sse":
		if state.url == "" {
			return nil, fmt.Errorf("--url is required for --transport %s", transport)
		}
		if state.command != "" {
			return nil, fmt.Errorf("--command is only valid for --transport stdio")
		}
		def := map[string]interface{}{
			"type": transport,
			"url":  state.url,
		}
		if len(state.headers) > 0 {
			headers, err := parseKVSlice(state.headers, "--header")
			if err != nil {
				return nil, err
			}
			def["headers"] = headers
		}
		return def, nil

	default:
		return nil, fmt.Errorf("invalid --transport %q (expected stdio|http|sse)", transport)
	}
}

func parseKVSlice(pairs []string, flagName string) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(pairs))
	for _, raw := range pairs {
		idx := strings.IndexByte(raw, '=')
		if idx <= 0 {
			return nil, fmt.Errorf("%s entry %q must be KEY=VALUE", flagName, raw)
		}
		out[raw[:idx]] = raw[idx+1:]
	}
	return out, nil
}

func toInterfaceSlice(in []string) []interface{} {
	out := make([]interface{}, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

func recordMCPInstall(id string, scope manifest.InstallScope, profiles []string) error {
	m, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}
	if existing := m.Find(id, manifest.KindMCP); existing != nil {
		m.Remove(id, manifest.KindMCP)
	}
	if scope == manifest.ScopeGlobal && profiles == nil {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		profiles = config.ProfileNames(cfg)
	}
	m.Add(manifest.Install{
		ID:       id,
		Kind:     manifest.KindMCP,
		Scope:    scope,
		Profiles: profiles,
	})
	return manifest.Save(m)
}

func runMCPRemove(state *mcpState, args []string) error {
	serverName := args[0]

	mcpDir, err := share.MCPDir()
	if err != nil {
		return err
	}

	switch state.scope {
	case mcpScopeGlobal:
		if err := removeMCPFromFragment(filepath.Join(mcpDir, "global.json"), serverName); err != nil {
			return fmt.Errorf("removing from global fragment: %w", err)
		}
	case mcpScopeProfile:
		if state.profile == "" {
