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
	// Hostile input ONLY goes through m.terminal, because a refusal returns
	// before osascript is ever reached. Never feed this a clean value: terminal
	// really does open a Terminal window on the developer's machine, and a
	// "clean input is not refused" case here spawned one on every test run.
	// That direction is covered by TestTerminalArgsOK against the pure guard.
	//
	// Asserting the SPECIFIC refusal matters too. terminal used to resolve the
	// ccpm binary before validating, so on a machine without ccpm on PATH every
	// case returned "ccpm CLI not found on PATH" and the test passed — it would
	// have passed with the guard deleted outright.
	m := NewMutate()
	for _, bad := range []string{"/tmp/a\nb", "/tmp/a\rb", "/tmp/a\x00b"} {
		r := m.terminal(bad, "run", "work")
		if r.OK || r.Error != errControlChar {
			t.Errorf("workdir %q: got %+v, want the control-character refusal", bad, r)
		}
	}
	for _, bad := range []string{"wo\nrk", "wo\rrk", "wo\x00rk"} {
		r := m.terminal("", "run", bad)
		if r.OK || r.Error != errControlChar {
			t.Errorf("argument %q: got %+v, want the control-character refusal", bad, r)
		}
	}
}

// TestTerminalArgsOK exercises the guard directly, including the clean cases —
// which is only safe because this function composes nothing and launches
// nothing. Without the clean cases the test above would still pass against a
// validator that rejected every input.
func TestTerminalArgsOK(t *testing.T) {
	clean := []struct {
		workdir string
		args    []string
	}{
		{"", []string{"run", "work"}},
		{"/tmp", []string{"run", "work"}},
		{"/Users/a/My Projects/repo", []string{"run", "work-2"}},
		{"/tmp/caf\u00e9", []string{"run", "profil\u00e9"}},
		{"/tmp/it's", []string{"run", "a;b|c$(d)"}}, // shell metacharacters are shellQuote's job, not this guard's
		{"", nil},
	}
	for _, c := range clean {
		if !terminalArgsOK(c.workdir, c.args) {
			t.Errorf("terminalArgsOK(%q, %q) = false, want true", c.workdir, c.args)
		}
	}

	hostile := []struct {
		workdir string
		args    []string
	}{
		{"/tmp/a\nb", []string{"run", "work"}},
		{"/tmp/a\rb", []string{"run", "work"}},
		{"/tmp/a\x00b", []string{"run", "work"}},
		{"", []string{"run", "wo\nrk"}},
		{"", []string{"run", "work", "extra\r"}},
		{"\n", nil},
	}
	for _, c := range hostile {
		if terminalArgsOK(c.workdir, c.args) {
			t.Errorf("terminalArgsOK(%q, %q) = true, want false", c.workdir, c.args)
		}
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
