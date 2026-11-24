//go:build darwin

package services

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

// These tests run /bin/sh, cat and sleep in the PTY. They never run ccpm or
// claude, and never open Terminal.app: the command builder is replaced in
// every one of them.

type termEvents struct {
	mu     sync.Mutex
	names  []string
	chunks map[string][]terminalChunk
}

func (e *termEvents) emit(event string, payload any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.names = append(e.names, event)
	if c, ok := payload.(terminalChunk); ok {
		id := strings.TrimPrefix(event, "terminal:data:")
		e.chunks[id] = append(e.chunks[id], c)
	}
}

func (e *termEvents) has(name string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Contains(e.names, name)
}

// testTerminal returns a service whose sessions run argv instead of ccpm.
func testTerminal(t *testing.T, argv ...string) (*TerminalService, *termEvents) {
	t.Helper()
	ev := &termEvents{chunks: map[string][]terminalChunk{}}
	ts := NewTerminal()
	ts.emit = ev.emit
	ts.command = func(string, []string) (*exec.Cmd, error) {
		return exec.Command(argv[0], argv[1:]...), nil
	}
	t.Cleanup(ts.CloseAll)
	return ts, ev
}

func start(t *testing.T, ts *TerminalService, profile string) TerminalSession {
	t.Helper()
	s, err := ts.Start(profile, "", nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return s
}

func scrollback(t *testing.T, ts *TerminalService, id string) (string, TerminalAttach) {
	t.Helper()
	a, err := ts.Attach(id)
	if err != nil {
		t.Fatalf("Attach(%s): %v", id, err)
	}
	b, err := base64.StdEncoding.DecodeString(a.Data)
	if err != nil {
		t.Fatalf("scrollback is not base64: %v", err)
	}
	return string(b), a
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func exited(t *testing.T, ts *TerminalService, id string) TerminalSession {
	t.Helper()
	var a TerminalAttach
	eventually(t, "the session to exit", func() bool {
		_, a = scrollback(t, ts, id)
		return !a.Session.Running
	})
	return a.Session
}

// gone reports whether pid no longer exists. A zombie still answers kill(0),
// so ESRCH also proves the process was reaped.
func gone(pid int) bool {
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

func TestTerminalStartCapturesOutputExitAndEnvironment(t *testing.T) {
	name := syntheticProfile(t)
	home, _ := os.UserHomeDir()
	ts, ev := testTerminal(t, "/bin/sh", "-c", `printf 'pwd=%s term=%s\n' "$PWD" "$TERM"; exit 3`)
	s := start(t, ts, name)
	if s.Profile != name || s.Cwd != home || !s.Running || s.Title != "claude" {
		t.Errorf("Start returned %+v", s)
	}
	info := exited(t, ts, s.ID)
	if info.ExitCode != 3 {
		t.Errorf("exit code = %d, want 3", info.ExitCode)
	}
	out, _ := scrollback(t, ts, s.ID)
	if want := "pwd=" + home + " term=xterm-256color"; !strings.Contains(out, want) {
		t.Errorf("output %q does not contain %q (empty cwd must mean home)", out, want)
	}
	for _, e := range []string{"terminal:data:" + s.ID, "terminal:exit:" + s.ID, "terminal:changed"} {
		if !ev.has(e) {
			t.Errorf("event %s was never emitted", e)
		}
	}
	// An exited session stays listed, with its exit code, until closed.
	if l := ts.List(); len(l) != 1 || l[0].Running || l[0].ExitCode != 3 {
		t.Errorf("List after exit = %+v", l)
	}
}

func TestTerminalWriteReachesTheChild(t *testing.T) {
	ts, _ := testTerminal(t, "cat")
	s := start(t, ts, syntheticProfile(t))
	if err := ts.Write(s.ID, "ping\n"); err != nil {
		t.Fatal(err)
	}
	// Twice: once as the tty's echo, once as cat's output. Echo alone would
	// mean the bytes never reached the process.
	eventually(t, "cat to print the input back", func() bool {
		out, _ := scrollback(t, ts, s.ID)
		return strings.Count(out, "ping") >= 2
	})
}

func TestTerminalResizeChangesTheWinsize(t *testing.T) {
	ts, _ := testTerminal(t, "/bin/sh", "-c", `read x; stty size; read y`)
	s := start(t, ts, syntheticProfile(t))
	if err := ts.Resize(s.ID, 100, 40); err != nil {
		t.Fatal(err)
	}
	if err := ts.Write(s.ID, "\n"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "stty to report the new size", func() bool {
		out, _ := scrollback(t, ts, s.ID)
		return strings.Contains(out, "40 100")
	})
	for _, bad := range [][2]int{{0, 10}, {10, 0}, {-1, 5}, {5000, 10}} {
		if err := ts.Resize(s.ID, bad[0], bad[1]); err == nil {
			t.Errorf("Resize(%d, %d) accepted", bad[0], bad[1])
		}
	}
}

func TestTerminalCloseKillsTheWholeProcessGroup(t *testing.T) {
	// The shell and the sleep it spawns both ignore SIGHUP, so this passes only
	// if Close escalates to SIGKILL and signals the group, not just the leader.
	ts, _ := testTerminal(t, "/bin/sh", "-c", `trap "" HUP; sleep 300 & echo "grandchild=$!"; wait`)
	s := start(t, ts, syntheticProfile(t))
	re := regexp.MustCompile(`grandchild=(\d+)`)
	var grandchild int
	eventually(t, "the grandchild pid", func() bool {
		out, _ := scrollback(t, ts, s.ID)
		m := re.FindStringSubmatch(out)
		if m != nil {
			grandchild, _ = strconv.Atoi(m[1])
		}
		return m != nil
	})
	leader := ts.get(s.ID).cmd.Process.Pid
	// If Close is broken, CloseAll in cleanup cannot reach the session (Close
	// already forgot it), and the HUP-proof sleep would outlive the test run.
	t.Cleanup(func() { _ = syscall.Kill(-leader, syscall.SIGKILL); _ = syscall.Kill(grandchild, syscall.SIGKILL) })
	if gone(leader) || gone(grandchild) {
		t.Fatal("fixture processes died before Close")
	}

	ts.Close(s.ID)

	// Close returns once the leader is reaped, so no polling for it.
	if !gone(leader) {
		t.Errorf("leader %d still exists after Close (killed but not reaped?)", leader)
	}
	eventually(t, "the grandchild to die", func() bool { return gone(grandchild) })
	if ts.get(s.ID) != nil || len(ts.List()) != 0 {
		t.Error("a closed session is still listed")
	}
}

func TestTerminalAttachReplaysWhatTheEventsCarried(t *testing.T) {
	ts, ev := testTerminal(t, "/bin/sh", "-c", `printf 'line1\nline2\n'`)
	s := start(t, ts, syntheticProfile(t))
	exited(t, ts, s.ID)
	out, a := scrollback(t, ts, s.ID)
	if !strings.Contains(out, "line1\r\nline2\r\n") {
		t.Errorf("scrollback = %q", out)
	}
	// The view drops a live chunk whose seq <= Attach's seq. That is only
	// right if seq counts exactly the bytes the chunks carried.
	ev.mu.Lock()
	chunks := ev.chunks[s.ID]
	ev.mu.Unlock()
	var all strings.Builder
	for _, c := range chunks {
		b, _ := base64.StdEncoding.DecodeString(c.Data)
		all.Write(b)
	}
	if all.String() != out {
		t.Errorf("events carried %q, scrollback holds %q", all.String(), out)
	}
	if len(chunks) == 0 || chunks[len(chunks)-1].Seq != a.Seq || a.Seq != int64(len(out)) {
		t.Errorf("seq mismatch: last chunk %+v, attach seq %d, %d bytes", chunks, a.Seq, len(out))
	}
}

func TestTerminalScrollbackIsBounded(t *testing.T) {
	ts, _ := testTerminal(t, "/bin/sh", "-c", `head -c 10000 /dev/zero | tr '\0' x; printf END`)
	ts.scrollback = 1024
	s := start(t, ts, syntheticProfile(t))
	exited(t, ts, s.ID)
	out, a := scrollback(t, ts, s.ID)
	if len(out) != 1024 {
		t.Errorf("scrollback holds %d bytes, want the 1024 bound", len(out))
	}
	if !strings.HasSuffix(out, "END") {
		t.Errorf("the bound dropped the newest output, not the oldest: ...%q", out[max(0, len(out)-20):])
	}
	if a.Seq != 10003 {
		t.Errorf("seq = %d, want 10003: it counts everything written, not what is kept", a.Seq)
	}
}

func TestTerminalUnknownIDIsSafe(t *testing.T) {
	ts, _ := testTerminal(t, "cat")
	ts.Close("nope") // must not panic
	if _, err := ts.Attach("nope"); err == nil || err.Error() != errNoTerminal {
		t.Errorf("Attach: %v", err)
	}
	if err := ts.Write("nope", "x"); err == nil || err.Error() != errNoTerminal {
		t.Errorf("Write: %v", err)
	}
	if err := ts.Resize("nope", 80, 24); err == nil || err.Error() != errNoTerminal {
		t.Errorf("Resize: %v", err)
	}
	s := start(t, ts, syntheticProfile(t))
	ts.Close(s.ID)
	ts.Close(s.ID) // twice is fine
}

func TestTerminalExitedSessionIgnoresInput(t *testing.T) {
	ts, _ := testTerminal(t, "/bin/sh", "-c", "exit 0")
	s := start(t, ts, syntheticProfile(t))
	exited(t, ts, s.ID)
	if err := ts.Write(s.ID, "x"); err != nil {
		t.Errorf("Write to an exited session: %v", err)
	}
	if err := ts.Resize(s.ID, 80, 24); err != nil {
		t.Errorf("Resize of an exited session: %v", err)
	}
}

func TestTerminalCloseAllEndsEverySession(t *testing.T) {
	ts, _ := testTerminal(t, "sleep", "300")
	name := syntheticProfile(t)
	var pids []int
	for range 3 {
		s := start(t, ts, name)
		pids = append(pids, ts.get(s.ID).cmd.Process.Pid)
	}
	ts.CloseAll()
	if l := ts.List(); len(l) != 0 {
		t.Errorf("sessions survived CloseAll: %+v", l)
	}
	for _, pid := range pids {
		if !gone(pid) {
			t.Errorf("pid %d survived CloseAll", pid)
		}
	}
}

func TestTerminalStartRefusesBadInputBeforeSpawning(t *testing.T) {
	name := syntheticProfile(t)
	ts := NewTerminal()
	ts.emit = func(string, any) {}
	ts.command = func(string, []string) (*exec.Cmd, error) {
		t.Fatal("the command builder ran for input Start should have refused")
		return nil, nil
	}
	missing := filepath.Join(t.TempDir(), "gone")
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		profile, cwd string
		args         []string
		want         string
	}{
		{name, "/tmp/a\nb", nil, errControlChar},
		{name, "", []string{"--", "--resume", "a\rb"}, errControlChar},
		{"wo\x00rk", "", nil, errControlChar},
		{"-rf", "", nil, "profile name must start"},
		{"no-such-profile", "", nil, "unknown profile"},
		{name, missing, nil, "directory does not exist"},
		{name, file, nil, "directory does not exist"},
	}
	for _, c := range cases {
		_, err := ts.Start(c.profile, c.cwd, c.args)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("Start(%q, %q, %q) = %v, want %q", c.profile, c.cwd, c.args, err, c.want)
		}
	}
}

func TestTerminalEnv(t *testing.T) {
	env := terminalEnv([]string{"PATH=/bin", "TERM=dumb"})
	if envValue(env, "TERM") != "xterm-256color" || envValue(env, "LANG") != "en_US.UTF-8" {
		t.Errorf("env = %q", env)
	}
	// A locale the user has must be kept, not overridden.
	env = terminalEnv([]string{"LC_ALL=fr_FR.UTF-8"})
	if envValue(env, "LANG") != "" {
		t.Errorf("LANG forced over an existing LC_ALL: %q", env)
	}
}

// An app launched from a Claude Code terminal inherits that session's markers;
// the embedded claude must not, or it runs as the parent's child (transcripts
// off, the parent's messaging socket and token in hand).
func TestTerminalChildDoesNotInheritClaudeSessionMarkers(t *testing.T) {
	name := syntheticProfile(t)
	for _, k := range claudeSessionMarkers {
		t.Setenv(k, "leaked")
	}
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "1") // user config, must pass through
	ts, _ := testTerminal(t, "/usr/bin/env")
	s := start(t, ts, name)
	exited(t, ts, s.ID)
	out, _ := scrollback(t, ts, s.ID)
	seen := map[string]string{}
	for _, line := range strings.Split(out, "\r\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			seen[k] = v
		}
	}
	for _, k := range claudeSessionMarkers {
		if v, ok := seen[k]; ok {
			t.Errorf("child inherited %s=%s", k, v)
		}
	}
	if seen["CLAUDE_CODE_USE_BEDROCK"] != "1" {
		t.Errorf("CLAUDE_CODE_USE_BEDROCK did not reach the child (env: %q)", out)
	}
}

func TestMergePath(t *testing.T) {
	got := mergePath("/usr/bin:/opt/homebrew/bin", "/opt/homebrew/bin", "", ".", "/Users/a/.local/bin", "/usr/bin")
	if want := "/usr/bin:/opt/homebrew/bin:/Users/a/.local/bin"; got != want {
		t.Errorf("mergePath = %q, want %q", got, want)
	}
	if got := mergePath("", "/a"); got != "/a" {
		t.Errorf("mergePath from an empty PATH = %q", got)
	}
}

// resumeFixture registers a synthetic profile holding one indexed session,
// "sess1", whose transcript records cwd.
func resumeFixture(t *testing.T, cwd string) string {
	t.Helper()
	name := syntheticProfile(t)
	proj := filepath.Join(profileDir(name), "projects", usage.EncodeCwd(cwd))
	if err := os.MkdirAll(proj, 0o700); err != nil {
		t.Fatal(err)
	}
	q, _ := json.Marshal(cwd)
	withCwd := func(line string) string { return strings.Replace(line, `"cwd":"/repo"`, `"cwd":`+string(q), 1) }
	body := withCwd(userLine(t, "u1", "sess1", "hello", "2026-01-02T00:00:00Z")) +
		withCwd(asstLine(t, "a1", "u1", "sess1", "msg_1", "hi", "2026-01-02T00:00:01Z", false))
	if err := os.WriteFile(filepath.Join(proj, "sess1.jsonl"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewHistory().Sessions(name); err != nil {
		t.Fatalf("indexing: %v", err)
	}
	return name
}

func TestResumeOpensAnEmbeddedTerminalInTheSessionsCwd(t *testing.T) {
	cwd := t.TempDir()
	name := resumeFixture(t, cwd)
	ts, _ := testTerminal(t, "/bin/sh", "-c", `printf 'pwd=%s\n' "$PWD"`)
	var gotProfile string
	var gotArgs []string
	real := ts.command
	ts.command = func(p string, a []string) (*exec.Cmd, error) {
		gotProfile, gotArgs = p, a
		return real(p, a)
	}
	h := NewHistory()
	h.Terminal = ts

	s, err := h.Resume(name, "sess1")
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if gotProfile != name || !slices.Equal(gotArgs, []string{"--", "--resume", "sess1"}) {
		t.Errorf("ran ccpm run %s %q, want run %s -- --resume sess1", gotProfile, gotArgs, name)
	}
	if s.Cwd != cwd || s.Profile != name || s.Title != "resume sess1" {
		t.Errorf("Resume returned %+v", s)
	}
	exited(t, ts, s.ID)
	if out, _ := scrollback(t, ts, s.ID); !strings.Contains(out, "pwd="+cwd) {
		t.Errorf("the session did not run in its cwd: %q", out)
	}
}

func TestResumeRefusesAMissingOrHostileCwd(t *testing.T) {
	nl := filepath.Join(t.TempDir(), "a\nb")
	if err := os.MkdirAll(nl, 0o700); err != nil {
		t.Fatal(err)
	}
	for cwd, want := range map[string]string{
		filepath.Join(t.TempDir(), "gone"): "that session's directory no longer exists",
		// Exists, so the stat passes: the refusal has to come from the
		// control-character funnel in Start.
		nl: errControlChar,
	} {
		name := resumeFixture(t, cwd)
		ts := NewTerminal()
		ts.command = func(string, []string) (*exec.Cmd, error) {
			t.Fatalf("Resume spawned a process for cwd %q", cwd)
			return nil, nil
		}
		h := NewHistory()
		h.Terminal = ts
		if _, err := h.Resume(name, "sess1"); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Resume with cwd %q: err = %v, want %q", cwd, err, want)
		}
	}
}
