//go:build darwin

package services

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/profile"
)

// CmdResult is the outcome of a shelled-out ccpm command.
type CmdResult struct {
	OK       bool   `json:"ok"`
	Output   string `json:"output"`
	Error    string `json:"error"`
	CCPMPath string `json:"ccpmPath"`
}

// MutateService performs WRITES by shelling out to the ccpm CLI, reusing its
// lock/keychain/atomic/validation logic rather than duplicating it. Interactive
// flows that need OAuth (create/auth) are bridged to a Terminal (in-GUI OAuth is
// deferred to v2).
type MutateService struct{}

func NewMutate() *MutateService { return &MutateService{} }

func runCCPM(args ...string) CmdResult {
	bin := findCCPM()
	if bin == "" {
		return CmdResult{Error: "ccpm CLI not found on PATH"}
	}
	// Bound every mutating shell-out so a stuck ccpm can't wedge the UI.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(envWithoutColor(), "NO_COLOR=1")
	out, err := cmd.CombinedOutput()
	r := CmdResult{OK: err == nil, Output: ansiRE.ReplaceAllString(string(out), ""), CCPMPath: bin}
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			r.Error = "ccpm timed out after 60s"
		} else {
			r.Error = strings.TrimSpace(err.Error())
		}
		if r.Output == "" {
			r.Output = r.Error
		}
	}
	return r
}

// Clone duplicates src into a new profile dst (assets + settings + auth).
func (s *MutateService) Clone(src, dst string) CmdResult { return runCCPM("clone", src, dst) }

// Rename renames a profile (migrates keychain + plugin paths).
func (s *MutateService) Rename(oldName, newName string) CmdResult {
	return runCCPM("rename", oldName, newName)
}

// Remove deletes a profile. Destructive — the UI confirms first.
func (s *MutateService) Remove(name string) CmdResult { return runCCPM("remove", name, "--force") }

// OpenFolder reveals the profile directory in the system file manager.
func (s *MutateService) OpenFolder(name string) CmdResult {
	cfg, err := config.Load()
	if err != nil {
		return CmdResult{Error: err.Error()}
	}
	pc, ok := cfg.Profiles[name]
	if !ok {
		return CmdResult{Error: "unknown profile: " + name}
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", pc.Dir)
	case "windows":
		cmd = exec.Command("explorer", pc.Dir)
	default:
		cmd = exec.Command("xdg-open", pc.Dir)
	}
	if err := cmd.Start(); err != nil {
		return CmdResult{Error: err.Error()}
	}
	return CmdResult{OK: true, Output: pc.Dir}
}

// Launch opens a new Terminal running `ccpm run <name>` (spawns Claude Code with
// the profile). macOS only for v1; other platforms get a copyable command.
func (s *MutateService) Launch(name string) CmdResult {
	if err := profile.ValidateName(name); err != nil {
		return CmdResult{Error: err.Error()}
	}
	return s.terminal("run", name)
}

// CreateInTerminal opens a Terminal running `ccpm add <name>` (the interactive
// auth wizard) — the bridge for in-GUI OAuth being deferred.
func (s *MutateService) CreateInTerminal(name string) CmdResult {
	if err := profile.ValidateName(name); err != nil {
		return CmdResult{Error: err.Error()}
	}
	return s.terminal("add", name)
}

// ImportInTerminal opens a Terminal running the import-from-host wizard.
func (s *MutateService) ImportInTerminal() CmdResult {
	return s.terminal("add")
}

// --- asset-level writes (profile-scoped) ---

// AddAsset installs an asset of kind (skill/agent/command/rule/hook) from a
// filesystem path into a profile.
func (s *MutateService) AddAsset(kind, path, profile string) CmdResult {
	return runCCPM(kind, "add", path, "--profile", profile)
}

// RemoveAsset removes a named asset of kind from a profile.
func (s *MutateService) RemoveAsset(kind, name, profile string) CmdResult {
	return runCCPM(kind, "remove", name, "--profile", profile)
}

// --- MCP + plugins ---

// AddStdioMCP adds a stdio MCP server to a profile.
func (s *MutateService) AddStdioMCP(name, command, profile string) CmdResult {
	return runCCPM("mcp", "add", name, "--scope", "profile", "--profile", profile, "--command", command)
}

// AddHTTPMCP adds an http/sse MCP server to a profile.
func (s *MutateService) AddHTTPMCP(name, url, profile string) CmdResult {
	return runCCPM("mcp", "add", name, "--scope", "profile", "--profile", profile, "--transport", "http", "--url", url)
}

// RemoveMCP removes a profile-scoped MCP server.
func (s *MutateService) RemoveMCP(name, profile string) CmdResult {
	return runCCPM("mcp", "remove", name, "--scope", "profile", "--profile", profile)
}

// TogglePlugin enables or disables a plugin (<name>@<marketplace>) for a profile.
func (s *MutateService) TogglePlugin(plugin string, enable bool, profile string) CmdResult {
	verb := "disable"
	if enable {
		verb = "enable"
	}
	return runCCPM("plugin", verb, plugin, "--profile", profile)
}

// InstallPlugin installs a plugin (<name>@<marketplace>) into a profile.
func (s *MutateService) InstallPlugin(plugin, profile string) CmdResult {
	return runCCPM("plugin", "install", plugin, "--profile", profile)
}

// RemovePlugin uninstalls a plugin (<name>@<marketplace>) from a profile.
func (s *MutateService) RemovePlugin(plugin, profile string) CmdResult {
	return runCCPM("plugin", "remove", plugin, "--profile", profile)
}

// SetSetting sets a settings key (dot notation) to a JSON value for a profile.
func (s *MutateService) SetSetting(key, value, profile string) CmdResult {
	return runCCPM("settings", "set", key, value, "--profile", profile)
}

// --- permissions + env ---

// AddPermission adds a rule to a bucket (allow/ask/deny) for a profile.
func (s *MutateService) AddPermission(bucket, rule, profile string) CmdResult {
	return runCCPM("permissions", bucket, rule, "--profile", profile)
}

// RemovePermission strips a rule from all permission buckets for a profile.
func (s *MutateService) RemovePermission(rule, profile string) CmdResult {
	return runCCPM("permissions", "remove", rule, "--profile", profile)
}

// SetPermissionMode sets the default permission mode for a profile.
func (s *MutateService) SetPermissionMode(mode, profile string) CmdResult {
	return runCCPM("permissions", "mode", mode, "--profile", profile)
}

// SetEnv sets a KEY=VALUE env var on a profile.
func (s *MutateService) SetEnv(kv, profile string) CmdResult {
	return runCCPM("env", "set", kv, "--profile", profile)
}

// UnsetEnv removes an env var from a profile.
func (s *MutateService) UnsetEnv(key, profile string) CmdResult {
	return runCCPM("env", "unset", key, "--profile", profile)
}

// terminal launches a new Terminal window running `<ccpm> <args...>`.
//
// AppleScript's `do script` hands its argument to a shell, and %q only escapes
// the AppleScript string literal — `;`, `|`, `$(…)` and backticks survive it
// intact. Every argument is therefore single-quoted for the shell here, in the
// one function all Terminal launches route through, so a profile name can
// never break out into a second command.
func (s *MutateService) terminal(args ...string) CmdResult {
	bin := findCCPM()
	if bin == "" {
		return CmdResult{Error: "ccpm CLI not found on PATH"}
	}
	quoted := make([]string, 0, len(args)+1)
	for _, a := range append([]string{bin}, args...) {
		quoted = append(quoted, shellQuote(a))
	}
	full := strings.Join(quoted, " ")
	if runtime.GOOS != "darwin" {
		return CmdResult{OK: false, Output: full, Error: "open a terminal and run: " + full}
	}
	script := fmt.Sprintf(`tell application "Terminal"
	activate
	do script %q
end tell`, full)
	if err := exec.Command("osascript", "-e", script).Start(); err != nil {
		return CmdResult{Error: err.Error(), Output: full}
	}
	return CmdResult{OK: true, Output: full}
}

// shellQuote wraps s in single quotes for /bin/sh. Inside single quotes the
// shell expands nothing, so the only character needing care is the quote
// itself: close, emit an escaped quote, reopen.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
