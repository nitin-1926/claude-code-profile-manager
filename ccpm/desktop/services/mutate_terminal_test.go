//go:build darwin

package services

import (
	"strings"
	"testing"
)

// buildCommand mirrors what terminal() composes, so the quoting can be asserted
// without launching Terminal. It is deliberately a copy of the composition
// rather than a refactor of it: the point is to catch a change to the real one.
func buildCommand(bin, workdir string, args ...string) string {
	quoted := make([]string, 0, len(args)+1)
	for _, a := range append([]string{bin}, args...) {
		quoted = append(quoted, shellQuote(a))
	}
	full := strings.Join(quoted, " ")
	if workdir != "" {
		full = "cd " + shellQuote(workdir) + " && " + full
	}
	return full
}

func TestShellQuoteNeutralisesShellMetacharacters(t *testing.T) {
	// Everything here would be a second command without the quoting.
	cases := map[string]string{
		"/tmp/a$(id)b":     `'/tmp/a$(id)b'`,
		"/tmp/a`id`b":      "'/tmp/a`id`b'",
		"/tmp/a;rm -rf /":  `'/tmp/a;rm -rf /'`,
		"/tmp/a|cat":       `'/tmp/a|cat'`,
		"/tmp/a b":         `'/tmp/a b'`,
		"/tmp/it's":        `'/tmp/it'\''s'`,
		"/tmp/a&&echo pwn": `'/tmp/a&&echo pwn'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestTerminalCommandShape(t *testing.T) {
	got := buildCommand("/usr/local/bin/ccpm", "/repo/project", "run", "work", "--", "--resume", "abc-123")
	want := `cd '/repo/project' && '/usr/local/bin/ccpm' 'run' 'work' '--' '--resume' 'abc-123'`
	if got != want {
		t.Errorf("command = %s\nwant     = %s", got, want)
	}
}

func TestTerminalWorkdirQuotingContainsHostileNames(t *testing.T) {
	// The && joining cd to the command is emitted outside the quoting; the path
	// itself goes through it, so a directory name cannot start a second command.
	got := buildCommand("/bin/ccpm", "/tmp/x'; echo pwn; '", "run", "work")
	if !strings.HasPrefix(got, `cd '/tmp/x'\''; echo pwn; '\''' && `) {
		t.Errorf("hostile workdir was not contained: %s", got)
	}
	if strings.Count(got, "&&") != 1 {
		t.Errorf("a second command separator leaked in: %s", got)
	}
}

func TestTerminalEmptyWorkdirEmitsNoCd(t *testing.T) {
	got := buildCommand("/bin/ccpm", "", "add")
	if strings.Contains(got, "cd ") {
		t.Errorf("empty workdir still emitted a cd: %s", got)
	}
	if got != `'/bin/ccpm' 'add'` {
		t.Errorf("command = %s", got)
	}
}

// TestTerminalRejectsControlCharacters covers the composition hazard: a newline
// stays inside the single quotes for the shell, but Go's %q renders it as \n
// and AppleScript's parser materialises it back into a real newline, so
// `do script` types a broken command. The funnel refuses rather than emitting it.
func TestTerminalRejectsControlCharacters(t *testing.T) {
	m := NewMutate()
	for _, bad := range []string{"/tmp/a\nb", "/tmp/a\rb", "/tmp/a\x00b"} {
		if r := m.terminal(bad, "run", "work"); r.OK || r.Error == "" {
			t.Errorf("terminal accepted a workdir containing a control character: %q -> %+v", bad, r)
		}
	}
	if r := m.terminal("", "run", "wo\nrk"); r.OK || r.Error == "" {
		t.Error("terminal accepted an argument containing a newline")
	}
}

func TestResumeRejectsImplausibleSessionIDs(t *testing.T) {
	h := NewHistory()
	for _, bad := range []string{
		"", "../../../etc/passwd", "a b", "a;id", "a'b", "a\nb", "a$(id)", strings.Repeat("x", 200),
	} {
		r := h.Resume("work", bad)
		if r.OK {
			t.Errorf("Resume accepted session id %q", bad)
		}
	}
}

func TestResumeUnknownProfileAndSession(t *testing.T) {
	h := NewHistory()
	if r := h.Resume("definitely-not-a-real-profile-xyz", "4245147b-6298-4288-9207-146fb29288b4"); r.OK {
		t.Error("Resume succeeded for an unknown profile")
	}
	name := firstProfile(t)
	if r := h.Resume(name, "00000000-0000-0000-0000-000000000000"); r.OK {
		t.Error("Resume succeeded for a session with no transcript")
	}
}
