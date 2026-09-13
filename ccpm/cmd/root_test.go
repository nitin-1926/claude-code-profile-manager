package cmd

import (
	"bytes"
	"strings"
	"testing"
)

// TestCobraDoesNotPrintErrorsItself is the regression for a doubled error
// message. Every ccpm failure used to print twice:
//
//	Error: unknown config key "nosuchkey"
//	unknown config key "nosuchkey"
//
// once from cobra and once from Execute. Execute has to do the printing — it is
// the only place that also maps codedError to an exit code — so cobra must stay
// quiet. Execute itself calls os.Exit and cannot be called from a test, so this
// asserts the half that is testable: running a failing command through the real
// root writes nothing at all, leaving Execute's single line as the only output.
func TestCobraDoesNotPrintErrorsItself(t *testing.T) {
	// A pure arg-count failure: it never reaches a RunE, so nothing touches the
	// filesystem or the real ~/.ccpm.
	var out, errOut bytes.Buffer
	restore := swapRootIO(t, &out, &errOut)
	defer restore()

	rootCmd.SetArgs([]string{"config", "get"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected `config get` with no argument to fail")
	}
	if got := errOut.String(); got != "" {
		t.Errorf("cobra wrote to stderr, so the message would print twice:\n%q", got)
	}
	if got := out.String(); got != "" {
		t.Errorf("cobra wrote to stdout on a failure:\n%q", got)
	}
	// The usage block must stay suppressed too — the other half of why these
	// two flags are set together.
	if strings.Contains(errOut.String()+out.String(), "Usage:") {
		t.Error("the usage block was printed on an error")
	}
}

// TestSilenceErrorsCoversSubcommands guards the reason this lives on the root
// rather than on each command: cobra consults the executed command's flag OR
// the root's, so setting it once covers the whole tree. A subcommand added
// later inherits the behaviour without having to know about it.
func TestSilenceErrorsCoversSubcommands(t *testing.T) {
	if !rootCmd.SilenceErrors {
		t.Fatal("rootCmd.SilenceErrors is off — every command's errors print twice")
	}
	if !rootCmd.SilenceUsage {
		t.Fatal("rootCmd.SilenceUsage is off — failures bury the message in help text")
	}

	var out, errOut bytes.Buffer
	restore := swapRootIO(t, &out, &errOut)
	defer restore()

	// A nested subcommand two levels down, again failing on arg count alone.
	rootCmd.SetArgs([]string{"statusline", "configure", "unexpected-arg"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected an unexpected positional arg to fail")
	}
	if got := errOut.String() + out.String(); got != "" {
		t.Errorf("a nested subcommand printed its own error:\n%q", got)
	}
}

// swapRootIO redirects the shared rootCmd's streams and args for one test and
// returns a restore func. rootCmd is a package-level global, so leaving it
// pointed at a test's buffer would corrupt anything that ran afterwards.
func swapRootIO(t *testing.T, out, errOut *bytes.Buffer) func() {
	t.Helper()
	rootCmd.SetOut(out)
	rootCmd.SetErr(errOut)
	return func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	}
}
