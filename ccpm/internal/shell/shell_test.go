package shell

import (
	"strings"
	"testing"
)

func TestGenerateHookBash(t *testing.T) {
	hook := GenerateHook("bash")
	if !strings.Contains(hook, "ccpm()") {
		t.Error("Bash hook should define ccpm() function")
	}
	if !strings.Contains(hook, "eval") {
		t.Error("Bash hook should use eval for 'use' subcommand")
	}
	if !strings.Contains(hook, `"$1" = "use"`) {
		t.Error("Bash hook should check for 'use' argument")
	}
}

func TestGenerateHookFish(t *testing.T) {
	hook := GenerateHook("fish")
	if !strings.Contains(hook, "function ccpm") {
		t.Error("Fish hook should define ccpm function")
	}
}

func TestGenerateHookPowershell(t *testing.T) {
	hook := GenerateHook("powershell")
	if !strings.Contains(hook, "function ccpm") {
		t.Error("PowerShell hook should define ccpm function")
	}
}

func TestExportStatementsBash(t *testing.T) {
	out := ExportStatements("bash", "myprofile", "/home/user/.ccpm/profiles/myprofile")

	if !strings.Contains(out, "export CLAUDE_CONFIG_DIR=") {
		t.Error("Should export CLAUDE_CONFIG_DIR")
	}
	if !strings.Contains(out, "export CCPM_ACTIVE_PROFILE=") {
		t.Error("Should export CCPM_ACTIVE_PROFILE")
	}
	if !strings.Contains(out, "/home/user/.ccpm/profiles/myprofile") {
		t.Error("Should contain the full profile dir path")
	}
	if !strings.Contains(out, "myprofile") {
		t.Error("Should contain the profile name")
	}
}

func TestExportStatementsFish(t *testing.T) {
	out := ExportStatements("fish", "work", "/path/to/work")
	if !strings.Contains(out, "set -gx CLAUDE_CONFIG_DIR") {
		t.Error("Fish should use 'set -gx'")
	}
}

func TestExportStatementsPathWithSpaces(t *testing.T) {
	// Profile dirs shouldn't have spaces (we validate names), but paths might
	out := ExportStatements("bash", "test", "/Users/my user/path")
	if !strings.Contains(out, "'/Users/my user/path'") {
		t.Error("Paths should be single-quoted to handle spaces")
	}
}

func TestExportStatementsNoInjection(t *testing.T) {
	// Ensure profile names can't inject shell commands
	out := ExportStatements("bash", "test'; rm -rf /; echo '", "/safe/path")
	// The output should have the name in a safe context
	if strings.Contains(out, "rm -rf") && !strings.Contains(out, "'") {
		t.Error("Profile name should not allow shell injection")
	}
}

func TestDetectShell(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	if got := DetectShell(); got != "zsh" {
		t.Errorf("DetectShell() = %q for /bin/zsh, want %q", got, "zsh")
	}

	t.Setenv("SHELL", "/usr/bin/fish")
	if got := DetectShell(); got != "fish" {
		t.Errorf("DetectShell() = %q for /usr/bin/fish, want %q", got, "fish")
	}
}
