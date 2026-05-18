package shell

import (
	"os"
	"path/filepath"
	"strings"
)

func DetectShell() string {
	shell := os.Getenv("SHELL")
	base := filepath.Base(shell)
	switch base {
	case "zsh":
		return "zsh"
	case "bash":
		return "bash"
	case "fish":
		return "fish"
	default:
		if os.Getenv("PSModulePath") != "" {
			return "powershell"
		}
		return "bash"
	}
}

func GenerateHook(shellName string) string {
	switch shellName {
	case "fish":
		return fishHook
	case "powershell":
		return powershellHook
	default:
		return bashZshHook
	}
}

func ExportStatements(shellName, profileName, profileDir string) string {
	profileDir = strings.ReplaceAll(profileDir, "'", "'\\''")
	switch shellName {
	case "fish":
		return "set -gx CLAUDE_CONFIG_DIR '" + profileDir + "'\n" +
			"set -gx CCPM_ACTIVE_PROFILE '" + profileName + "'\n" +
			"echo 'Switched to profile: " + profileName + ". Run claude to start.'"
	case "powershell":
		return "$env:CLAUDE_CONFIG_DIR = '" + profileDir + "'\n" +
			"$env:CCPM_ACTIVE_PROFILE = '" + profileName + "'\n" +
			"Write-Host 'Switched to profile: " + profileName + ". Run claude to start.'"
	default:
		return "export CLAUDE_CONFIG_DIR='" + profileDir + "'\n" +
			"export CCPM_ACTIVE_PROFILE='" + profileName + "'\n" +
			"echo 'Switched to profile: " + profileName + ". Run claude to start.'"
	}
}

// bashZshHook ships two shell functions:
//   - ccpm(): unchanged — needed so 'ccpm use' can mutate the parent shell env.
//   - claude(): routes plain 'claude' invocations through the ccpm default
//     profile when no CLAUDE_CONFIG_DIR is already set in the environment.
//
// The claude() wrapper exists because Claude Code v2.1.138 has a startup-time
// flow (refresh-on-startup against /v1/oauth/token) that misbehaves when
// CLAUDE_CONFIG_DIR resolves to the bare ~/.claude directory, even when the
// keychain default slot and ~/.claude.json identity are correctly synced. The
// same claude binary, given a profile-namespaced CLAUDE_CONFIG_DIR, works
// fine. Routing through the profile slot transparently dodges the bug for
// terminal-launched claude until upstream ships a fix. IDE extensions launch
// the claude binary directly (no shell), so they are not covered by this
// wrapper — for them the default keychain slot remains the source of truth.
//
// The wrapper is intentionally conservative:
//   - Only fires when CLAUDE_CONFIG_DIR is unset (so 'ccpm run' / 'ccpm use'
//     callers, which already set it, pass straight through).
//   - Calls 'command ccpm config get default_dir' which prints empty on no
//     default — in that case we fall through to plain 'command claude'.
//   - Uses 'command claude' rather than $0 so we cannot loop on ourselves.
const bashZshHook = `ccpm() {
  if [ "$1" = "use" ]; then
    eval "$(command ccpm use "${@:2}")"
  else
    command ccpm "$@"
  fi
}

claude() {
  if [ -z "$CLAUDE_CONFIG_DIR" ] && command -v ccpm >/dev/null 2>&1; then
    local _ccpm_default_dir
    _ccpm_default_dir="$(command ccpm config get default_dir 2>/dev/null)"
    if [ -n "$_ccpm_default_dir" ] && [ -d "$_ccpm_default_dir" ]; then
      CLAUDE_CONFIG_DIR="$_ccpm_default_dir" command claude "$@"
      return
    fi
  fi
  command claude "$@"
}`

const fishHook = `function ccpm
  if test "$argv[1]" = "use"
    eval (command ccpm use $argv[2..])
  else
    command ccpm $argv
  end
end

function claude
  if test -z "$CLAUDE_CONFIG_DIR"; and command -v ccpm >/dev/null 2>&1
    set -l _ccpm_default_dir (command ccpm config get default_dir 2>/dev/null)
    if test -n "$_ccpm_default_dir"; and test -d "$_ccpm_default_dir"
      CLAUDE_CONFIG_DIR=$_ccpm_default_dir command claude $argv
      return
    end
  end
  command claude $argv
end`

const powershellHook = `function ccpm {
  if ($args[0] -eq "use") {
    $output = & ccpm.exe use $args[1..($args.Length-1)]
    Invoke-Expression $output
  } else {
    & ccpm.exe @args
  }
}

function claude {
  if (-not $env:CLAUDE_CONFIG_DIR -and (Get-Command ccpm.exe -ErrorAction SilentlyContinue)) {
    $defaultDir = (& ccpm.exe config get default_dir 2>$null)
    if ($defaultDir -and (Test-Path $defaultDir)) {
      $prev = $env:CLAUDE_CONFIG_DIR
      $env:CLAUDE_CONFIG_DIR = $defaultDir
      try { & claude.exe @args } finally { $env:CLAUDE_CONFIG_DIR = $prev }
      return
    }
  }
  & claude.exe @args
}`
