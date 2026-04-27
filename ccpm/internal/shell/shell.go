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

const bashZshHook = `ccpm() {
  if [ "$1" = "use" ]; then
    eval "$(command ccpm use "${@:2}")"
  else
    command ccpm "$@"
  fi
}`

const fishHook = `function ccpm
  if test "$argv[1]" = "use"
    eval (command ccpm use $argv[2..])
  else
    command ccpm $argv
  end
end`

const powershellHook = `function ccpm {
  if ($args[0] -eq "use") {
    $output = & ccpm.exe use $args[1..($args.Length-1)]
    Invoke-Expression $output
  } else {
    & ccpm.exe @args
  }
}`
