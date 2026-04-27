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

	claudepkg "github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/claude"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/manifest"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/picker"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
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
			return fmt.Errorf("--profile is required for --scope profile")
		}
		if err := removeMCPFromFragment(filepath.Join(mcpDir, state.profile+".json"), serverName); err != nil {
			return fmt.Errorf("removing from profile fragment: %w", err)
		}
	case mcpScopeProject:
		root, err := resolveProjectRoot(state)
		if err != nil {
			return err
		}
		if err := removeProjectMCPServer(root, serverName); err != nil {
			return err
		}
	}

	if state.scope != mcpScopeProject {
		m, err := manifest.Load()
		if err != nil {
			return fmt.Errorf("loading manifest: %w", err)
		}
		m.Remove(serverName, manifest.KindMCP)
		if err := manifest.Save(m); err != nil {
			return fmt.Errorf("saving manifest: %w", err)
		}
	}

	color.New(color.FgGreen, color.Bold).Printf("✓ MCP server %q removed\n", serverName)
	return nil
}

// mcpListRow is one row in the `ccpm mcp list` output.
type mcpListRow struct {
	Name     string
	Sources  []string
	Profiles []string
	Type     string
}

func runMCPList() error {
	rows := map[string]*mcpListRow{}

	addRow := func(name, source, typ string, profiles []string) {
		row, exists := rows[name]
		if !exists {
			row = &mcpListRow{Name: name}
			rows[name] = row
		}
		row.Sources = append(row.Sources, source)
		if row.Type == "" {
			row.Type = typ
		}
		for _, p := range profiles {
			if !stringSliceContains(row.Profiles, p) {
				row.Profiles = append(row.Profiles, p)
			}
		}
	}

	m, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}
	for _, s := range m.ListByKind(manifest.KindMCP) {
		source := "ccpm-profile"
		if s.Scope == manifest.ScopeGlobal {
			source = "ccpm-global"
		}
		addRow(s.ID, source, "", s.Profiles)
	}

	hostMCP := loadHostMCPSafe()
	for name, raw := range hostMCP {
		addRow(name, "host", typeOfMCPDef(raw), nil)
	}

	if cwd, werr := os.Getwd(); werr == nil {
		if root := settingsmerge.FindProjectRoot(cwd); root != "" {
			if projectMCP, perr := settingsmerge.LoadProjectMCP(root); perr == nil {
				for name, raw := range projectMCP {
					addRow(name, "project", typeOfMCPDef(raw), nil)
				}
			}
		}
	}

	if len(rows) == 0 {
		fmt.Println("No MCP servers found. Add one with: ccpm mcp add <name> --command <cmd> --scope global")
		return nil
	}

	names := make([]string, 0, len(rows))
	for name := range rows {
		names = append(names, name)
	}
	sort.Strings(names)

	bold := color.New(color.Bold).SprintFunc()
	fmt.Printf("  %-20s %-6s %-25s %s\n", bold("SERVER"), bold("TYPE"), bold("SOURCE"), bold("PROFILES"))
	fmt.Printf("  %s\n", strings.Repeat("─", 72))

	for _, name := range names {
		row := rows[name]
		typ := row.Type
		if typ == "" {
			typ = "—"
		}
		profiles := "—"
		if len(row.Profiles) > 0 {
			profiles = strings.Join(row.Profiles, ", ")
		}
		fmt.Printf("  %-20s %-6s %-25s %s\n", row.Name, typ, strings.Join(row.Sources, ", "), profiles)
	}
	return nil
}

// typeOfMCPDef extracts the transport type from an opaque server definition.
func typeOfMCPDef(raw interface{}) string {
	def, ok := raw.(map[string]interface{})
	if !ok {
		return "—"
	}
	if t, ok := def["type"].(string); ok && t != "" {
		return t
	}
	if _, ok := def["command"]; ok {
		return "stdio"
	}
	if _, ok := def["url"]; ok {
		return "http"
	}
	return "—"
}

func stringSliceContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func runMCPImport(state *mcpState, args []string) error {
	filePath := args[0]

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filePath, err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parsing %s: %w", filePath, err)
	}
	servers, _ := raw["mcpServers"].(map[string]interface{})
	if servers == nil {
		servers = map[string]interface{}{}
		for k, v := range raw {
			if _, ok := v.(map[string]interface{}); ok {
				servers[k] = v
			}
		}
	}

	if err := share.EnsureDirs(); err != nil {
		return err
	}

	for serverName, serverDef := range servers {
		defMap, ok := serverDef.(map[string]interface{})
		if !ok {
			fmt.Fprintf(os.Stderr, "Warning: skipping %q (not a valid server definition)\n", serverName)
			continue
		}
		if !hasMCPConnector(defMap) {
			fmt.Fprintf(os.Stderr, "Warning: %q has neither a command nor url field — importing anyway, but claude may not be able to launch it\n", serverName)
		}

		switch state.scope {
		case mcpScopeGlobal:
			if err := writeMCPFragment("global", serverName, defMap); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not write %q: %v\n", serverName, err)
			}
		case mcpScopeProfile:
			if err := writeMCPFragment(state.profile, serverName, defMap); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not write %q: %v\n", serverName, err)
			}
		case mcpScopeProject:
			root, err := resolveProjectRoot(state)
			if err != nil {
				return err
			}
			if err := writeProjectMCPServer(root, serverName, defMap); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not write %q to project: %v\n", serverName, err)
			}
		}
	}

	if state.scope != mcpScopeProject {
		m, err := manifest.Load()
		if err != nil {
			return fmt.Errorf("loading manifest: %w", err)
		}
		scope := manifest.ScopeGlobal
		var profiles []string
		if state.scope == mcpScopeProfile {
			scope = manifest.ScopeProfile
			profiles = []string{state.profile}
		} else {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			profiles = config.ProfileNames(cfg)
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
	}

	color.New(color.FgGreen, color.Bold).Printf("✓ Imported %d MCP servers from %s\n", len(servers), filePath)
	return nil
}

func hasMCPConnector(def map[string]interface{}) bool {
	if _, ok := def["command"]; ok {
		return true
	}
	if _, ok := def["url"]; ok {
		return true
	}
	if _, ok := def["type"]; ok {
		return true
	}
	return false
}

func runMCPAuth(state *mcpState, args []string) error {
	serverName := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	p, exists := cfg.Profiles[state.profile]
	if !exists {
		return fmt.Errorf("profile %q not found", state.profile)
	}

	projectRoot := ""
	if cwd, werr := os.Getwd(); werr == nil {
		projectRoot = settingsmerge.FindProjectRoot(cwd)
	}
	if err := settingsmerge.Materialize(p.Dir, state.profile, projectRoot); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not materialize settings before auth: %v\n", err)
	}
	if err := settingsmerge.MaterializeMCP(p.Dir, state.profile, projectRoot); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not materialize MCP config before auth: %v\n", err)
	}

	fmt.Printf("Starting native claude in profile %q to authenticate MCP server %q...\n", state.profile, serverName)
	fmt.Println("If claude's CLI does not expose an `mcp auth` subcommand, the spawned process will report an error; in that case, run `ccpm run " + state.profile + "` and trigger auth interactively (/mcp).")
	return claudepkg.Spawn(p.Dir, "MCP_AUTH_SERVER="+serverName)
}

// pickMCPScope resolves scope when nothing was given.
func pickMCPScope(state *mcpState) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	scope, err := picker.Select("Install scope", []picker.Option{
		{Value: mcpScopeGlobal, Label: "Global", Description: "all profiles now and any created later"},
		{Value: mcpScopeProfile, Label: "A single profile", Description: "pick one profile"},
		{Value: mcpScopeProject, Label: "Project (.mcp.json)", Description: "git-committed to the current project"},
	})
	if err != nil {
		if errors.Is(err, picker.ErrNonInteractive) {
			return fmt.Errorf("specify --scope global|profile|project")
		}
		return err
	}
	state.scope = scope
	if scope == mcpScopeProfile {
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
	}
	return nil
}

// writeMCPFragment writes or updates a single server definition.
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

// removeMCPFromFragment deletes a server from a fragment file and rewrites it.
// Surfaces both the load and write errors so the caller can decide what to do
// when the user lacks write permission on ~/.ccpm/share/mcp.
func removeMCPFromFragment(fragPath, serverName string) error {
	frag, err := settingsmerge.LoadJSON(fragPath)
	if err != nil {
		return err
	}
	if _, present := frag[serverName]; !present {
		return nil
	}
	delete(frag, serverName)
	return settingsmerge.WriteJSON(fragPath, frag)
}

// writeProjectMCPServer upserts a server inside <root>/.mcp.json.
func writeProjectMCPServer(root, serverName string, serverDef map[string]interface{}) error {
	path := filepath.Join(root, ".mcp.json")
	doc, err := settingsmerge.LoadJSON(path)
	if err != nil {
		return fmt.Errorf("loading %s: %w", path, err)
	}
	servers, _ := doc["mcpServers"].(map[string]interface{})
	if servers == nil {
		servers = map[string]interface{}{}
	}
	servers[serverName] = serverDef
	doc["mcpServers"] = servers
	return settingsmerge.WriteJSON(path, doc)
}

func removeProjectMCPServer(root, serverName string) error {
	path := filepath.Join(root, ".mcp.json")
	doc, err := settingsmerge.LoadJSON(path)
	if err != nil {
		return fmt.Errorf("loading %s: %w", path, err)
	}
	servers, _ := doc["mcpServers"].(map[string]interface{})
	if servers != nil {
		delete(servers, serverName)
		doc["mcpServers"] = servers
	}
	return settingsmerge.WriteJSON(path, doc)
}

// loadHostMCPSafe reads ~/.claude.json#mcpServers, swallowing errors.
func loadHostMCPSafe() map[string]interface{} {
	home, err := os.UserHomeDir()
	if err != nil {
		return map[string]interface{}{}
	}
	doc, err := settingsmerge.LoadJSON(filepath.Join(home, ".claude.json"))
	if err != nil {
		return map[string]interface{}{}
	}
	servers, _ := doc["mcpServers"].(map[string]interface{})
	if servers == nil {
		return map[string]interface{}{}
	}
	return servers
}
