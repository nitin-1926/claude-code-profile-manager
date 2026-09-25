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
		if outdatedCLI(r.Output) {
			r.Error = "the ccpm CLI at " + bin + " is too old for this — update it and try again (" +
				strings.TrimSpace(r.Output) + ")"
		}
	}
	return r
}

// outdatedCLI reports whether ccpm's output is cobra refusing a flag or
// subcommand it does not know.
//
// The desktop app ships separately from the CLI and drives it through its
// flags. A desktop build that is newer than the CLI on PATH therefore fails
// with cobra's own message — "unknown flag: --profile" when saving a status
// line layout against a CLI that predates `statusline configure` — which tells
// the user nothing about what to do. Measured on a real machine: the CLI on
// PATH was 0.5.4 and every status line save failed exactly this way.
func outdatedCLI(output string) bool {
	return strings.Contains(output, "unknown flag:") ||
		strings.Contains(output, "unknown shorthand flag:") ||
		strings.Contains(output, "unknown command \"")
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
	return s.terminal("", "run", name)
}

// CreateInTerminal opens a Terminal running `ccpm add <name>` (the interactive
// auth wizard) — the bridge for in-GUI OAuth being deferred.
func (s *MutateService) CreateInTerminal(name string) CmdResult {
	if err := profile.ValidateName(name); err != nil {
		return CmdResult{Error: err.Error()}
	}
	return s.terminal("", "add", name)
}

// ImportInTerminal opens a Terminal running the import-from-host wizard.
func (s *MutateService) ImportInTerminal() CmdResult {
	return s.terminal("", "add")
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

// terminal launches a new Terminal window running `<ccpm> <args...>`, optionally
// after cd-ing into workdir.
//
// AppleScript's `do script` hands its argument to a shell, and %q only escapes
// the AppleScript string literal — `;`, `|`, `$(…)` and backticks survive it
// intact. Every argument is therefore single-quoted for the shell here, in the
// one function all Terminal launches route through, so a profile name can
// never break out into a second command.
//
// The `cd` for workdir is composed here rather than by callers: the path goes
// through shellQuote like any other argument and only the `&&` is emitted
// outside the quoting, so a directory name cannot introduce a second command.
// errControlChar is the exact refusal a caller gets for a control character.
// Exported as a constant so the test can assert THIS error rather than merely
// "an error" — asserting presence is what let the previous version pass on a
// machine where the CLI could not be found at all.
const errControlChar = "refusing to run a command containing a control character"

// terminalArgsOK reports whether every value is safe to compose into the
// AppleScript that `terminal` hands to `do script`.
//
// Reject control characters for every caller, in the shared funnel rather than
// at each call site. A newline does not escape the single quotes — it stays
// inside them — but Go's %q renders it as \n and AppleScript's parser turns
// that back into a real newline, so `do script` would type a broken command
// into Terminal. Refusing is clearer than emitting something confusing.
//
// Scoped to the three characters that actually cause the problem, and named
// for them rather than for "control characters" generally. Go's %q renders
// \x1b, \a, \b, \f, \v and \x7f in forms AppleScript refuses to compile, which
// CombinedOutput surfaces as an error — so those fail closed already. A tab is
// legal in a macOS directory name and passes harmlessly inside the single
// quotes. Widening the check would reject working paths to restate a guarantee
// the shell quoting already provides.
//
// Split out from terminal so the guard is testable on its own. terminal itself
// opens a real Terminal window on the developer's machine for any input that
// PASSES, so a test that fed it a clean value to prove the guard is not
// over-eager would spawn a window on every run — which is exactly what happened
// before this was extracted.
func terminalArgsOK(workdir string, args []string) bool {
	for _, a := range append([]string{workdir}, args...) {
		if strings.ContainsAny(a, "\n\r\x00") {
			return false
		}
	}
	return true
}

func (s *MutateService) terminal(workdir string, args ...string) CmdResult {
	// Validate BEFORE resolving the binary, deliberately.
	//
	// With the order reversed, a machine without ccpm on PATH returns the
	// unrelated "not found" error for hostile input too — which made the test
	// for this guard unfalsifiable: it asserted only that *some* error came
	// back, so it stayed green on CI containers and would have stayed green
	// with the guard deleted outright. Input validation does not depend on
	// binary discovery, so there is no reason for it to run second.
	if !terminalArgsOK(workdir, args) {
		return CmdResult{Error: errControlChar}
	}
	bin := findCCPM()
	if bin == "" {
		return CmdResult{Error: "ccpm CLI not found on PATH"}
	}
	full := composeCommand(bin, workdir, args...)
	if runtime.GOOS != "darwin" {
		return CmdResult{OK: false, Output: full, Error: "open a terminal and run: " + full}
	}
	script := fmt.Sprintf(`tell application "Terminal"
	activate
	do script %q
end tell`, full)
	// Run, not Start: Start only reports a failure to launch osascript itself,
	// so a script osascript refuses to compile (a non-UTF-8 byte in a path
	// renders as \xNN via %q, which AppleScript rejects) would return OK.
	//
	// Bounded, because waiting is now possible: osascript does not return while
	// macOS is showing the Automation consent sheet ("CCPM wants to control
	// Terminal"), which is guaranteed on first use and waits on a human. Without
	// a deadline that blocks the Wails goroutine forever and the button silently
	// does nothing. runCCPM above bounds its shell-out for the same reason.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "osascript", "-e", script).CombinedOutput(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return CmdResult{
				Error:  "Terminal did not respond within 30s — if macOS asked for permission to control Terminal, grant it and try again",
				Output: full,
			}
		}
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return CmdResult{Error: msg, Output: full}
	}
	return CmdResult{OK: true, Output: full}
}

// composeCommand builds the shell command terminal() hands to AppleScript.
//
// Split out of terminal() so the quoting tests can exercise the real thing. They
// used to call a copy of these lines living in the test file, which cannot catch
// a change to the original: a reviewer deleted the shellQuote around workdir —
// a live command injection, since Resume passes a directory read out of on-disk
// JSON — and all five tests stayed green.
func composeCommand(bin, workdir string, args ...string) string {
	quoted := make([]string, 0, len(args)+1)
	for _, a := range append([]string{bin}, args...) {
		quoted = append(quoted, shellQuote(a))
	}
	full := strings.Join(quoted, " ")
	if workdir != "" {
		// Only the && sits outside the quoting; the path itself goes through it.
		full = "cd " + shellQuote(workdir) + " && " + full
	}
	return full
}

// shellQuote wraps s in single quotes for /bin/sh. Inside single quotes the
// shell expands nothing, so the only character needing care is the quote
// itself: close, emit an escaped quote, reopen.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
