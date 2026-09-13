package cmd

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestMain doubles as a harness for exercising Execute, which calls os.Exit and
// therefore cannot run in-process. When CCPM_TEST_EXECUTE is set the test binary
// behaves as ccpm itself, so a subtest can re-exec it and observe exactly what a
// user's terminal would see — including the exit code.
func TestMain(m *testing.M) {
	if args := os.Getenv("CCPM_TEST_EXECUTE"); args != "" {
		os.Args = append([]string{"ccpm"}, strings.Fields(args)...)
		Execute()
		return
	}
	os.Exit(m.Run())
}

// runCCPMForTest re-execs the test binary as ccpm and returns its combined
// output and exit code. HOME points at a scratch directory so nothing touches
// the real ~/.ccpm.
func runCCPMForTest(t *testing.T, args string) (string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(),
		"CCPM_TEST_EXECUTE="+args,
		"HOME="+t.TempDir(),
		"NO_COLOR=1",
		"CCPM_NO_TTY=1",
	)
	out, err := cmd.CombinedOutput()
	code := 0
	var ee *exec.ExitError
	if err != nil {
		if !asExitError(err, &ee) {
			t.Fatalf("running %q: %v\n%s", args, err, out)
		}
		code = ee.ExitCode()
	}
	return string(out), code
}

func asExitError(err error, target **exec.ExitError) bool {
	ee, ok := err.(*exec.ExitError)
	if ok {
		*target = ee
	}
	return ok
}

// TestErrorsArePrintedExactlyOnce is the regression for a doubled message.
// Every ccpm failure used to print twice — once from cobra, once from Execute:
//
//	Error: unknown config key "nosuchkey"
//	unknown config key "nosuchkey"
//
// Cobra is the one kept, because it prints more than the message. Execute now
// only maps the error to an exit code.
func TestErrorsArePrintedExactlyOnce(t *testing.T) {
	cases := []struct {
		name     string
		args     string
		fragment string // the message body, without cobra's "Error: " prefix
	}{
		{"RunE error", "config set nosuchkey true", `unknown config key "nosuchkey"`},
		{"unknown flag", "list --definitely-not-a-flag", "unknown flag: --definitely-not-a-flag"},
		{"wrong arg count", "config get", "accepts 1 arg(s), received 0"},
		{"unknown command", "nosuchcommand", `unknown command "nosuchcommand" for "ccpm"`},
		{"unknown subcommand flag", "statusline configure --nope", "unknown flag: --nope"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, code := runCCPMForTest(t, tc.args)
			if n := strings.Count(out, tc.fragment); n != 1 {
				t.Errorf("message appears %d times, want exactly 1:\n%s", n, out)
			}
			if !strings.Contains(out, "Error: "+tc.fragment) {
				t.Errorf("missing cobra's %q prefix:\n%s", "Error: ", out)
			}
			if code != 1 {
				t.Errorf("exit code = %d, want 1", code)
			}
			// SilenceUsage must still hold: a failure prints the message, not
			// forty lines of help.
			if strings.Contains(out, "Usage:") {
				t.Errorf("the usage block was printed on an error:\n%s", out)
			}
		})
	}
}

// TestUnknownCommandKeepsTheHelpHint guards the reason cobra does the printing
// rather than Execute. Silencing cobra and printing the error ourselves looked
// equivalent, but cobra also emits a discoverability line for an unknown
// command, and doing it by hand silently dropped it.
func TestUnknownCommandKeepsTheHelpHint(t *testing.T) {
	out, _ := runCCPMForTest(t, "nosuchcommand")
	if !strings.Contains(out, "Run 'ccpm --help' for usage.") {
		t.Errorf("lost cobra's help hint for an unknown command:\n%s", out)
	}
}

// TestSuccessIsQuietAndExitsZero is the other half: the error path must not
// have made the success path noisy.
func TestSuccessIsQuietAndExitsZero(t *testing.T) {
	out, code := runCCPMForTest(t, "config get statusline")
	if code != 0 {
		t.Errorf("exit code = %d, want 0\n%s", code, out)
	}
	if strings.Contains(out, "Error:") {
		t.Errorf("a successful command printed an error:\n%s", out)
	}
	if strings.TrimSpace(out) != "true" {
		t.Errorf("got %q, want the setting's value", strings.TrimSpace(out))
	}
}
