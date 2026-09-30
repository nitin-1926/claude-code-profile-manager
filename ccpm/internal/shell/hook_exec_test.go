package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// runPOSIXHook sources the generated hook in sh (bash or zsh) with a stub
// `ccpm` first on PATH, runs script, and returns its stdout. The stub answers
// `use good` with real ExportStatements output and fails `use` of anything
// else the way the real binary does for an unknown profile.
func runPOSIXHook(t *testing.T, sh, profileDir, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	if _, err := exec.LookPath(sh); err != nil {
		t.Skip(sh + " not installed")
	}
	bin := t.TempDir()
	exports := filepath.Join(bin, "exports.sh")
	if err := os.WriteFile(exports, []byte(ExportStatements("bash", "good", profileDir)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stub := "#!/bin/sh\n" +
		"if [ \"$1 $2\" = \"use good\" ]; then cat '" + exports + "'; exit 0; fi\n" +
		"echo \"Error: profile \\\"$2\\\" not found\" >&2; exit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "ccpm"), []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	flags := []string{"--noprofile", "--norc"}
	if sh == "zsh" {
		flags = []string{"-f"}
	}
	c := exec.Command(sh, append(flags, "-c", GenerateHook(sh)+"\n"+script)...)
	c.Env = []string{"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + t.TempDir()}
	out, err := c.Output()
	if err != nil {
		t.Fatalf("%s: %v\n%s", sh, err, out)
	}
	return string(out)
}

// TestBashHookPropagatesUseFailure: `eval "$(command ccpm use …)"` returned
// eval's status (0), so `ccpm use typo && claude` went ahead on the old
// profile.
func TestBashHookPropagatesUseFailure(t *testing.T) {
	for _, sh := range []string{"bash", "zsh"} {
		t.Run(sh, func(t *testing.T) {
			out := runPOSIXHook(t, sh, "/p/good", `ccpm use nosuch 2>/dev/null; echo "status=$?"`)
			if !strings.Contains(out, "status=1") {
				t.Fatalf("ccpm use of an unknown profile reported %q, want status=1", strings.TrimSpace(out))
			}
		})
	}
}

// TestBashHookActivatesProfileWithQuoteInPath runs the full use flow and
// checks the exported path survives a single quote intact.
func TestBashHookActivatesProfileWithQuoteInPath(t *testing.T) {
	dir := "/Users/o'brien/.ccpm/profiles/good"
	for _, sh := range []string{"bash", "zsh"} {
		t.Run(sh, func(t *testing.T) {
			out := runPOSIXHook(t, sh, dir, `ccpm use good >/dev/null; echo "dir=$CLAUDE_CONFIG_DIR profile=$CCPM_ACTIVE_PROFILE"`)
			if want := "dir=" + dir + " profile=good"; strings.TrimSpace(out) != want {
				t.Fatalf("got %q, want %q", strings.TrimSpace(out), want)
			}
		})
	}
}

// TestFishHookSourcesWholeOutput: `eval (cmd)` split the multi-line output
// into arguments joined by spaces, running all three statements as one
// mangled command line.
func TestFishHookSourcesWholeOutput(t *testing.T) {
	hook := GenerateHook("fish")
	if strings.Contains(hook, "eval (command ccpm use") {
		t.Fatal("fish hook still evals a line-split command substitution")
	}
	if !strings.Contains(hook, "| source") {
		t.Fatal("fish hook must pipe the joined output of `ccpm use` to source")
	}
	if !strings.Contains(hook, "or return") {
		t.Fatal("fish hook must return the failure status of `ccpm use`")
	}
}

// TestPowerShellHookJoinsOutputAndGuardsSlice: Invoke-Expression received a
// string array (one element per line), and with no profile name
// $args[1..($args.Length-1)] was $args[1..0] — i.e. `use` again — so
// `ccpm use` ran `ccpm use use`.
func TestPowerShellHookJoinsOutputAndGuardsSlice(t *testing.T) {
	hook := GenerateHook("powershell")
	if strings.Contains(hook, "$args[1..($args.Length-1)]") {
		t.Fatal("powershell hook still slices $args[1..($args.Length-1)], which is $args[1..0] with no name")
	}
	if !strings.Contains(hook, "-join \"`n\"") {
		t.Fatal("powershell hook must join the output lines before Invoke-Expression")
	}
	if !strings.Contains(hook, "$LASTEXITCODE") {
		t.Fatal("powershell hook must not evaluate output of a failed `ccpm use`")
	}
}

// TestExportStatementsQuotePerShell: every shell got POSIX '\” escaping,
// which is not an escape in PowerShell (” is) and breaks fish, whose single
// quotes treat \' and \\ as escapes.
func TestExportStatementsQuotePerShell(t *testing.T) {
	dir := `C:\Users\o'brien`
	cases := map[string]string{
		"bash":       `export CLAUDE_CONFIG_DIR='C:\Users\o'\''brien'`,
		"fish":       `set -gx CLAUDE_CONFIG_DIR 'C:\\Users\\o\'brien'`,
		"powershell": `$env:CLAUDE_CONFIG_DIR = 'C:\Users\o''brien'`,
	}
	for sh, want := range cases {
		if out := ExportStatements(sh, "p", dir); !strings.Contains(out, want) {
			t.Errorf("%s: output lacks %q:\n%s", sh, want, out)
		}
	}
}
