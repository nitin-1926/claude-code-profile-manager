package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/atomicwrite"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/picker"
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
	exportIncludeSecrets     bool
)

// secretPlaceholder stands in for every secret-bearing value a default export
// strips (settings env values, MCP server env and header values). The key
// stays, so import can name what to fill in. Plain ASCII: JSON encoders that
// escape <, > and & would otherwise mangle it on the next rewrite.
const secretPlaceholder = "__CCPM_REDACTED__"

// secretFiles are the archive names whose JSON can carry secret values.
// Matched exactly: a skill may ship its own settings.json.
var secretFiles = []string{
	"settings.json",
	".claude.json",
	bundleFragmentsDir + "/settings.json",
	bundleFragmentsDir + "/mcp.json",
}

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
  - Secret values. Every settings env value (in settings.json and the ccpm
    settings fragment) and every MCP server env and header value is replaced
    with the placeholder ` + secretPlaceholder + `. The keys stay, so
    'ccpm import-bundle' tells whoever restores it what to fill in. Use
    --include-secrets to keep the real values for a move between your own
    machines.

A bundle made with any of these flags is sensitive: keep it private. Even a
default bundle carries your hooks and your MCP servers' commands, args and
URLs, so review it before sharing.`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeProfileNames,
	RunE:              runExport,
}

var importBundleCmd = &cobra.Command{
	Use:   "import-bundle <file.tar.gz>",
	Short: "Restore a profile from a 'ccpm export' bundle",
	Long: `Creates a new profile from a .tar.gz produced by 'ccpm export'. By default the
new profile takes the bundle's original name; override with --profile.

Treat a bundle from someone else as untrusted code. Before anything is
restored, ccpm lists every hook command, MCP server (command/args/url),
command-valued setting (statusLine, apiKeyHelper, ...) and settings env key
in the bundle and asks for confirmation, because each runs or takes effect on
every 'ccpm run' of the profile. Without a TTY the import refuses unless
--trust-bundle is passed.

After restoring, ccpm lists every value the exporter stripped (it holds the
placeholder ` + secretPlaceholder + `) and how to fill each one in.

If the bundle did not include credentials (the default), authenticate the
restored profile afterwards with 'ccpm auth refresh <name>'.`,
	Args: cobra.ExactArgs(1),
	RunE: runImportBundle,
}

func init() {
	exportCmd.Flags().StringVarP(&exportOut, "output", "o", "", "output path (default: <profile>.ccpm.tar.gz)")
	exportCmd.Flags().BoolVar(&exportIncludeCredentials, "include-credentials", false, "include credential files (sensitive — trusted same-user moves only)")
	exportCmd.Flags().BoolVar(&exportIncludeHistory, "include-history", false, "include session history: transcripts, todos, shell snapshots (may contain pasted secrets)")
	exportCmd.Flags().BoolVar(&exportIncludeSecrets, "include-secrets", false, "keep settings env and MCP env/header values instead of placeholders (sensitive — your own machines only)")
	rootCmd.AddCommand(exportCmd)

	importBundleCmd.Flags().String("profile", "", "name for the restored profile (default: the bundle's original name)")
	importBundleCmd.Flags().Bool("trust-bundle", false, "restore hooks, MCP servers, statusLine and env from the bundle without prompting (required when not on a TTY)")
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

	// redacted counts stripped secret values; nil keeps them (--include-secrets).
	var redacted *int
	if !exportIncludeSecrets {
		redacted = new(int)
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
		return addToTar(tw, path, info, arcName, redacted)
	})
	if walkErr == nil {
		walkErr = addFragmentsToTar(tw, name, redacted)
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

	w := cmd.OutOrStdout()
	color.New(color.FgGreen, color.Bold).Fprintf(w, "✓ Exported %q to %s\n", name, outPath)
	if len(skippedHistory) > 0 {
		fmt.Fprintf(w, "Session history left out (%s); pass --include-history to bundle it.\n", strings.Join(skippedHistory, ", "))
	}
	if len(skippedLinks) > 0 {
		fmt.Fprintf(w, "Links outside the ccpm share store left out (e.g. ~/.claude host assets): %s\n", strings.Join(skippedLinks, ", "))
	}
	if !exportIncludeCredentials {
		fmt.Fprintln(w, "Credentials were excluded. Restore, then run `ccpm auth refresh <name>`.")
	} else {
		color.New(color.FgYellow).Fprintln(w, "Bundle contains credentials — keep it private.")
	}
	if redacted == nil {
		color.New(color.FgYellow).Fprintln(w, "Bundle contains secret values (settings env, MCP env and headers) — keep it private.")
	} else if *redacted > 0 {
		fmt.Fprintf(w, "Replaced %d secret value(s) (settings env, MCP env and headers) with %s; keys kept. Pass --include-secrets to keep them (your own machines only).\n", *redacted, secretPlaceholder)
	}
	return nil
}

func isWithinDir(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && filepath.IsLocal(rel)
}

// addToTar writes one regular file or directory header (plus content) under
// the archive name. With redacted non-nil, a secretFiles entry has its secret
// values replaced by secretPlaceholder first, and redacted counts them.
func addToTar(tw *tar.Writer, path string, info os.FileInfo, name string, redacted *int) error {
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	hdr.Name = name
	if redacted != nil && !info.IsDir() && slices.Contains(secretFiles, name) {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		data, n, err := redactSecrets(name, data)
		if err != nil {
			return fmt.Errorf("stripping secrets from %s: %w (fix the file, or pass --include-secrets to bundle it as is)", name, err)
		}
		*redacted += n
		hdr.Size = int64(len(data))
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	}
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
		return addToTar(tw, path, info, name, nil)
	})
}

// redactSecrets replaces every secret value in data, the bundle file at
// archive path file, with secretPlaceholder. Empty values and existing
// placeholders hide nothing and are left alone. A file with nothing to strip
// comes back byte-for-byte unchanged.
func redactSecrets(file string, data []byte) ([]byte, int, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber() // numbers keep their original literal
	var doc map[string]interface{}
	if err := dec.Decode(&doc); err != nil {
		return nil, 0, err
	}
	n := 0
	visitSecrets(file, doc, func(_ secretValue, m map[string]interface{}, key string) {
		switch m[key] {
		case nil, "", secretPlaceholder:
			return
		}
		m[key] = secretPlaceholder
		n++
	})
	if n == 0 {
		return data, 0, nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, 0, err
	}
	return buf.Bytes(), n, nil
}

// secretValue locates one secret-bearing value in a bundle: a settings env
// value (Server empty) or an MCP server's env or header value.
type secretValue struct {
	File    string `json:"file"`              // archive path, e.g. ".ccpm-bundle/mcp.json"
	Project string `json:"project,omitempty"` // .claude.json project the server belongs to
	Server  string `json:"server,omitempty"`  // MCP server name
	Field   string `json:"field"`             // "env" or "headers"
	Key     string `json:"key"`
}

// visitSecrets calls fn for every secret-bearing value in doc, the parsed
// bundle file at archive path file: settings env values, and the env and
// headers values of MCP servers (the same servers bundleRunnables lists).
// fn gets the map holding the value so it can rewrite it in place.
func visitSecrets(file string, doc map[string]interface{}, fn func(s secretValue, m map[string]interface{}, key string)) {
	visitServers := func(project string, servers map[string]interface{}) {
		for _, name := range slices.Sorted(maps.Keys(servers)) {
			def, _ := servers[name].(map[string]interface{})
			for _, field := range []string{"env", "headers"} {
				m, _ := def[field].(map[string]interface{})
				for _, k := range slices.Sorted(maps.Keys(m)) {
					fn(secretValue{File: file, Project: project, Server: name, Field: field, Key: k}, m, k)
				}
			}
		}
	}
	switch file {
	case bundleFragmentsDir + "/mcp.json":
		visitServers("", doc)
	case "settings.json", bundleFragmentsDir + "/settings.json":
		env, _ := doc["env"].(map[string]interface{})
		for _, k := range slices.Sorted(maps.Keys(env)) {
			fn(secretValue{File: file, Field: "env", Key: k}, env, k)
		}
		servers, _ := doc["mcpServers"].(map[string]interface{})
		visitServers("", servers)
	case ".claude.json":
		servers, _ := doc["mcpServers"].(map[string]interface{})
		visitServers("", servers)
		projects, _ := doc["projects"].(map[string]interface{})
		for _, proj := range slices.Sorted(maps.Keys(projects)) {
			pm, _ := projects[proj].(map[string]interface{})
			servers, _ := pm["mcpServers"].(map[string]interface{})
			visitServers(proj, servers)
		}
	}
}

// bundlePlaceholders lists every value in an extracted bundle that still
// holds secretPlaceholder, i.e. what the exporter stripped. Never nil.
func bundlePlaceholders(profileDir string) ([]secretValue, error) {
	out := []secretValue{}
	for _, file := range secretFiles {
		doc, err := settingsmerge.LoadJSON(filepath.Join(profileDir, filepath.FromSlash(file)))
		if err != nil {
			return nil, err
		}
		visitSecrets(file, doc, func(s secretValue, m map[string]interface{}, key string) {
			if m[key] == secretPlaceholder {
				out = append(out, s)
			}
		})
	}
	return out, nil
}

// printPlaceholderFixes tells whoever restored profileName which stripped
// values to fill in and how. Settings env, wherever it sat, is fixed by
// `ccpm settings set`: the fragment layer wins over the profile's own
// settings.json. MCP env/headers have no set command, so name the file.
func printPlaceholderFixes(w io.Writer, holes []secretValue, profileName, profileDir string) error {
	if len(holes) == 0 {
		return nil
	}
	restored, err := bundleFragments(profileName)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Values stripped when this bundle was exported hold %s. Fill them in:\n", secretPlaceholder)
	seenEnv := map[string]bool{}
	type serverKey struct{ path, project, server string }
	var order []serverKey
	fields := map[serverKey][]string{}
	for _, h := range holes {
		if h.Server == "" {
			if !seenEnv[h.Key] {
				seenEnv[h.Key] = true
				fmt.Fprintf(w, "  ccpm settings set env.%s <value> --profile %s\n", h.Key, profileName)
			}
			continue
		}
		path := filepath.Join(profileDir, filepath.FromSlash(h.File))
		if frag, ok := strings.CutPrefix(h.File, bundleFragmentsDir+"/"); ok {
			path = restored[frag]
		}
		k := serverKey{path, h.Project, h.Server}
		if _, ok := fields[k]; !ok {
			order = append(order, k)
		}
		fields[k] = append(fields[k], h.Field+"."+h.Key)
	}
	for _, k := range order {
		where := k.path
		if k.project != "" {
			where += " (project " + k.project + ")"
		}
		fmt.Fprintf(w, "  MCP server %q in %s: %s\n", k.server, where, strings.Join(fields[k], ", "))
	}
	if len(order) > 0 {
		fmt.Fprintf(w, "Edit MCP values in the file named (the next `ccpm run %s` applies them), or re-add the server with `ccpm mcp add <server> --scope profile --profile %s` and --env KEY=VALUE / --header KEY=VALUE.\n", profileName, profileName)
	}
	return nil
}

// addFragmentsToTar carries the profile's share-store settings/MCP fragments
// under bundleFragmentsDir. Missing fragments are simply absent.
func addFragmentsToTar(tw *tar.Writer, profileName string, redacted *int) error {
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
		if err := addToTar(tw, path, info, bundleFragmentsDir+"/"+archive, redacted); err != nil {
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

// commandSettings are settings keys whose value Claude Code executes as a
// shell command.
var commandSettings = []string{"statusLine", "apiKeyHelper", "awsAuthRefresh", "awsCredentialExport", "otelHeadersHelper"}

// bundleRunnables lists every item in an extracted bundle that runs a
// command or can redirect Claude Code's traffic on the next `ccpm run`:
// hook commands, command-valued settings (statusLine, apiKeyHelper, ...),
// settings env keys, and MCP servers. Malformed JSON is an error, so an
// unparseable file can't hide what it holds.
func bundleRunnables(profileDir string) ([]string, error) {
	load := func(rel string) (map[string]interface{}, error) {
		return settingsmerge.LoadJSON(filepath.Join(profileDir, filepath.FromSlash(rel)))
	}
	var out []string
	for _, rel := range []string{"settings.json", bundleFragmentsDir + "/settings.json"} {
		doc, err := load(rel)
		if err != nil {
			return nil, err
		}
		hooks, _ := doc["hooks"].(map[string]interface{})
		for _, event := range slices.Sorted(maps.Keys(hooks)) {
			matchers, _ := hooks[event].([]interface{})
			for _, m := range matchers {
				mm, _ := m.(map[string]interface{})
				matcher, _ := mm["matcher"].(string)
				list, _ := mm["hooks"].([]interface{})
				for _, h := range list {
					hm, _ := h.(map[string]interface{})
					body, _ := hm["command"].(string)
					if body == "" {
						body, _ = hm["prompt"].(string)
					}
					out = append(out, fmt.Sprintf("hook        %s %s[%s]: %s", rel, event, matcher, body))
				}
			}
		}
		for _, key := range commandSettings {
			v, ok := doc[key]
			if !ok {
				continue
			}
			body, _ := v.(string)
			if obj, isObj := v.(map[string]interface{}); isObj {
				body, _ = obj["command"].(string)
			}
			out = append(out, fmt.Sprintf("command     %s %s: %s", rel, key, body))
		}
		if env, _ := doc["env"].(map[string]interface{}); len(env) > 0 {
			out = append(out, fmt.Sprintf("env         %s: %s", rel, strings.Join(slices.Sorted(maps.Keys(env)), ", ")))
		}
		servers, _ := doc["mcpServers"].(map[string]interface{})
		out = append(out, mcpRunnables(rel, servers)...)
	}

	frag, err := load(bundleFragmentsDir + "/mcp.json")
	if err != nil {
		return nil, err
	}
	out = append(out, mcpRunnables(bundleFragmentsDir+"/mcp.json", frag)...)

	claudeJSON, err := load(".claude.json")
	if err != nil {
		return nil, err
	}
	servers, _ := claudeJSON["mcpServers"].(map[string]interface{})
	out = append(out, mcpRunnables(".claude.json", servers)...)
	projects, _ := claudeJSON["projects"].(map[string]interface{})
	for _, proj := range slices.Sorted(maps.Keys(projects)) {
		pm, _ := projects[proj].(map[string]interface{})
		servers, _ := pm["mcpServers"].(map[string]interface{})
		out = append(out, mcpRunnables(".claude.json project "+proj, servers)...)
	}
	return out, nil
}

func mcpRunnables(src string, servers map[string]interface{}) []string {
	var out []string
	for _, name := range slices.Sorted(maps.Keys(servers)) {
		def, _ := servers[name].(map[string]interface{})
		var parts []string
		if c, _ := def["command"].(string); c != "" {
			parts = append(parts, "cmd="+c)
		}
		if args, _ := def["args"].([]interface{}); len(args) > 0 {
			parts = append(parts, fmt.Sprintf("args=%v", args))
		}
		if u, _ := def["url"].(string); u != "" {
			parts = append(parts, "url="+u)
		}
		if env, _ := def["env"].(map[string]interface{}); len(env) > 0 {
			parts = append(parts, "env="+strings.Join(slices.Sorted(maps.Keys(env)), ","))
		}
		out = append(out, fmt.Sprintf("mcp server  %s %s: %s", src, name, strings.Join(parts, " ")))
	}
	return out
}

// confirmBundleRunnables shows every runnable item in the extracted bundle
// and requires consent before it is restored, mirroring `ccpm import
// default`'s gate on hooks/MCP: prompt on a TTY, and without one refuse
// unless --trust-bundle, so piping `yes` or CI can't grant shell access.
func confirmBundleRunnables(cmd *cobra.Command, profileDir string) error {
	items, err := bundleRunnables(profileDir)
	if err != nil {
		return fmt.Errorf("inspecting bundle: %w", err)
	}
	if len(items) == 0 {
		return nil
	}
	w := cmd.ErrOrStderr()
	fmt.Fprintln(w, "This bundle contains items that run commands or change where Claude Code connects, every time the restored profile runs:")
	for _, it := range items {
		fmt.Fprintln(w, "  "+it)
	}
	if trust, _ := cmd.Flags().GetBool("trust-bundle"); trust {
		fmt.Fprintln(w, "--trust-bundle: restoring them without prompting.")
		return nil
	}
	if !picker.IsInteractive() {
		return errors.New("refusing to restore a bundle with hooks, MCP servers, commands or env without a TTY; re-run with --trust-bundle only if you trust whoever made it")
	}
	ok, err := picker.Confirm("Restore these? Only say yes if you trust whoever made this bundle.", false)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("import cancelled: bundle not trusted (nothing was restored)")
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
	// Gate before anything lands outside the profile dir or gets registered.
	if err := confirmBundleRunnables(cmd, dstDir); err != nil {
		_ = profile.Remove(name)
		return err
	}
	// Read before restoreBundleFragments drops the staged fragments.
	holes, err := bundlePlaceholders(dstDir)
	if err != nil {
		_ = profile.Remove(name)
		return fmt.Errorf("inspecting bundle: %w", err)
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

	w := cmd.OutOrStdout()
	color.New(color.FgGreen, color.Bold).Fprintf(w, "✓ Restored profile %q from %s\n", name, bundlePath)
	fmt.Fprintf(w, "Authenticate it with:\n  ccpm auth refresh %s\n", name)
	// The profile is restored either way: a failure here only loses the hint.
	if err := printPlaceholderFixes(w, holes, name, dstDir); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not list the values to fill in: %v\n", err)
	}
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
