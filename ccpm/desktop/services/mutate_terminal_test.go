//go:build darwin

package services

import (
	"strings"
	"testing"
)

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
	got := composeCommand("/usr/local/bin/ccpm", "/repo/project", "run", "work", "--", "--resume", "abc-123")
	want := `cd '/repo/project' && '/usr/local/bin/ccpm' 'run' 'work' '--' '--resume' 'abc-123'`
	if got != want {
		t.Errorf("command = %s\nwant     = %s", got, want)
	}
}

func TestTerminalWorkdirQuotingContainsHostileNames(t *testing.T) {
	// The && joining cd to the command is emitted outside the quoting; the path
	// itself goes through it, so a directory name cannot start a second command.
	got := composeCommand("/bin/ccpm", "/tmp/x'; echo pwn; '", "run", "work")
	if !strings.HasPrefix(got, `cd '/tmp/x'\''; echo pwn; '\''' && `) {
		t.Errorf("hostile workdir was not contained: %s", got)
	}
	if strings.Count(got, "&&") != 1 {
		t.Errorf("a second command separator leaked in: %s", got)
	}
}

func TestTerminalEmptyWorkdirEmitsNoCd(t *testing.T) {
	got := composeCommand("/bin/ccpm", "", "add")
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
	// Assert the SPECIFIC rejection, against a profile that cannot exist. The
	// earlier version used a real profile name and asserted only !r.OK, so every
	// id was rejected later by the unknown-session lookup instead — deleting the
	// safeSessionID gate entirely left this test green.
	h := NewHistory()
	for _, bad := range []string{
		"", "../../../etc/passwd", "a b", "a;id", "a'b", "a\nb", "a$(id)", strings.Repeat("x", 200),
	} {
		r := h.Resume("definitely-not-a-real-profile-xyz", bad)
		if r.OK {
			t.Errorf("Resume accepted session id %q", bad)
		}
		if !strings.Contains(r.Error, "implausible") {
			t.Errorf("Resume(%q) error = %q, want the session-id gate to reject it "+
				"(a later check rejecting it means the gate is untested)", bad, r.Error)
		}
	}
	// A well-formed id must get PAST the gate and fail on the profile instead.
	if r := h.Resume("definitely-not-a-real-profile-xyz", "4245147b-6298-4288-9207-146fb29288b4"); strings.Contains(r.Error, "implausible") {
		t.Error("a valid UUID was rejected by the session-id gate")
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
