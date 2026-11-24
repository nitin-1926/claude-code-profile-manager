package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/transcript"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

// sessionsFixture registers a profile "work" under a temp HOME and returns its
// directory. The session flags are package globals, so they are reset after.
func sessionsFixture(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	profileDir := filepath.Join(home, "profiles", "work")
	writeTestConfig(t, home, "work", profileDir)
	t.Cleanup(func() { sessionsAll, sessionsJSON, sessionsLimit = false, false, 20 })
	sessionsAll, sessionsJSON, sessionsLimit = false, false, 20
	return profileDir
}

// writeSession writes a transcript shaped like Claude Code's: one user line and
// one assistant line, each with its own uuid, the assistant carrying a
// message.id + requestId usage key.
func writeSession(t *testing.T, path, sess, cwd, prompt string) {
	t.Helper()
	lines := []map[string]any{
		{"type": "user", "uuid": sess + "-u1", "parentUuid": nil, "isSidechain": false,
			"sessionId": sess, "cwd": cwd, "timestamp": "2026-06-27T10:00:00.000Z",
			"message": map[string]any{"role": "user", "content": prompt}},
		{"type": "assistant", "uuid": sess + "-a1", "parentUuid": sess + "-u1", "isSidechain": false,
			"sessionId": sess, "cwd": cwd, "timestamp": "2026-06-27T10:00:05.000Z", "requestId": "req_" + sess,
			"message": map[string]any{"id": "msg_" + sess, "role": "assistant", "model": "claude-opus-4-8",
				"content": []any{map[string]any{"type": "text", "text": "ok"}},
				"usage":   map[string]any{"input_tokens": 10, "output_tokens": 2}}},
	}
	var sb strings.Builder
	for _, l := range lines {
		b, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

type listedSession struct {
	SessionID   string `json:"session_id"`
	Started     string `json:"started"`
	LastActive  string `json:"last_active"`
	Cwd         string `json:"cwd"`
	FirstPrompt string `json:"first_prompt"`
}

func listSessionsJSON(t *testing.T) []listedSession {
	t.Helper()
	sessionsJSON = true
	defer func() { sessionsJSON = false }()
	out := captureStdout(t, func() error { return runSessionsList(nil, []string{"work"}) })
	var got []listedSession
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", err, out)
	}
	return got
}

// Subagent transcripts live beside their session under <sid>/subagents/. They
// are not sessions — `claude --resume agent-…` resumes nothing — and on real
// profiles they outnumber real sessions ~9:1, pushing them off --limit. A
// session whose transcript Claude Code has pruned is equally unresumable.
func TestSessionsListShowsOnlyResumableSessions(t *testing.T) {
	profileDir := sessionsFixture(t)
	proj := filepath.Join(profileDir, "projects", usage.EncodeCwd("/repo"))
	writeSession(t, filepath.Join(proj, "s1.jsonl"), "s1", "/repo", "real prompt")
	writeSession(t, filepath.Join(proj, "s1", "subagents", "agent-a1.jsonl"), "s1", "/repo", "subagent task")
	writeSession(t, filepath.Join(proj, "gone.jsonl"), "gone", "/repo", "pruned later")
	if _, err := transcript.BuildIndex(profileDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(proj, "gone.jsonl")); err != nil {
		t.Fatal(err)
	}

	sessionsAll = true
	got := listSessionsJSON(t)
	if len(got) != 1 || got[0].SessionID != "s1" {
		t.Fatalf("listed %+v, want only s1", got)
	}
	if got[0].Cwd != "/repo" || got[0].FirstPrompt != "real prompt" {
		t.Errorf("row = %+v, want cwd /repo and first_prompt %q", got[0], "real prompt")
	}
	if got[0].Started != "2026-06-27T10:00:00Z" {
		t.Errorf("started = %q, want the session's first timestamp", got[0].Started)
	}
}

// Transcript text is attacker-influenced (a pasted log, a fetched page). The
// table prints it to a terminal, so control sequences must not survive: OSC 52
// writes the clipboard, CSI repaints the screen. A multi-line prompt must not
// break the row, and truncation must not split a rune.
func TestSessionsListTableIsTerminalSafe(t *testing.T) {
	profileDir := sessionsFixture(t)
	proj := filepath.Join(profileDir, "projects", usage.EncodeCwd("/repo"))
	evil := "hi \x1b]52;c;cHduZWQ=\x07 there \u009b2J done"
	writeSession(t, filepath.Join(proj, "s1.jsonl"), "s1", "/repo", evil)
	writeSession(t, filepath.Join(proj, "s2.jsonl"), "s2", "/repo", strings.Repeat("é", 100))

	sessionsAll = true
	out := captureStdout(t, func() error { return runSessionsList(nil, []string{"work"}) })
	if strings.ContainsAny(out, "\x1b\x07\u009b") {
		t.Errorf("control characters reached the terminal: %q", out)
	}
	if !utf8.ValidString(out) {
		t.Errorf("output is not valid UTF-8 (a rune was split): %q", out)
	}
	if !strings.Contains(out, "éé…") {
		t.Errorf("long title was not clipped with an ellipsis: %q", out)
	}
}

// Claude Code records the PHYSICAL cwd (/private/tmp on macOS), while
// os.Getwd returns the logical $PWD (/tmp). Encoding only the logical path
// found nothing in any symlinked directory.
func TestSessionsListMatchesPhysicalCwd(t *testing.T) {
	profileDir := sessionsFixture(t)
	real, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	proj := filepath.Join(profileDir, "projects", usage.EncodeCwd(real))
	writeSession(t, filepath.Join(proj, "s1.jsonl"), "s1", real, "here")
	writeSession(t, filepath.Join(profileDir, "projects", "-elsewhere", "s2.jsonl"), "s2", "/elsewhere", "not here")

	t.Chdir(link)
	t.Setenv("PWD", link) // os.Getwd trusts $PWD when it names the same dir
	got := listSessionsJSON(t)
	if len(got) != 1 || got[0].SessionID != "s1" {
		t.Fatalf("listed %+v from a symlinked cwd, want only s1", got)
	}
}

// These expectations previously encoded a bug: runs of non-alphanumerics were
// collapsed and the ends trimmed, so "/Users/alex/code/repo" came out as
// "Users-alex-code-repo" — a directory name that exists nowhere. Claude Code
// writes one dash per non-alphanumeric character and trims nothing, so the real
// directory is "-Users-alex-code-repo". Verified against 18 real project
// directories by usage.TestEncodeCwdMatchesRealLayout.
func TestEncodeCwdForClaude(t *testing.T) {
	cases := map[string]string{
		"/Users/alex/code/repo": "-Users-alex-code-repo",
		"/tmp/with.dots":        "-tmp-with-dots",
		"/a//b///c":             "-a--b---c",
		"already-clean":         "already-clean",
		"/":                     "-",
		"trailing/slash/":       "trailing-slash-",
		"//leading-empty":       "--leading-empty",
		"C:\\Users\\alex\\repo": "C--Users-alex-repo",
		"/Users/a/repo (local)": "-Users-a-repo--local-",
	}
	for in, want := range cases {
		if got := encodeCwdForClaude(in); got != want {
			t.Errorf("encodeCwdForClaude(%q) = %q, want %q", in, got, want)
		}
	}
}

// Rows are ordered by last activity, so the table must show last activity: a
// session started long ago but resumed today belongs at the top, and printing
// its start date there made the list look unsorted.
func TestSessionsListShowsTheActivityItIsSortedBy(t *testing.T) {
	profileDir := sessionsFixture(t)
	proj := filepath.Join(profileDir, "projects", usage.EncodeCwd("/repo"))
	oldResumed := filepath.Join(proj, "old.jsonl")
	fresh := filepath.Join(proj, "fresh.jsonl")
	writeSession(t, oldResumed, "old", "/repo", "started long ago")
	writeSession(t, fresh, "fresh", "/repo", "started recently")
	day := func(d int) time.Time { return time.Date(2026, 9, d, 12, 0, 0, 0, time.UTC) }
	if err := os.Chtimes(fresh, day(10), day(10)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(oldResumed, day(20), day(20)); err != nil { // resumed later
		t.Fatal(err)
	}
	sessionsAll = true

	got := listSessionsJSON(t)
	if len(got) != 2 || got[0].SessionID != "old" {
		t.Fatalf("order = %+v, want the most recently active session first", got)
	}
	if got[0].LastActive != day(20).Format(time.RFC3339) {
		t.Errorf("last_active = %q, want %q", got[0].LastActive, day(20).Format(time.RFC3339))
	}

	out := captureStdout(t, func() error { return runSessionsList(nil, []string{"work"}) })
	if !strings.Contains(out, "LAST ACTIVE") || strings.Contains(out, "STARTED") {
		t.Errorf("table header should name the sort key:\n%s", out)
	}
	if !strings.Contains(out, day(20).Local().Format("2006-01-02")) {
		t.Errorf("table should show the last-active date of the top row:\n%s", out)
	}
}

const portTestID = "7392c3d1-59b8-4832-9e0e-fc5d6e62b90a"

// sessionsPortFixture registers profiles "work" and "labs" under a temp HOME, with
// one session in "work" whose directory exists and has a space in its name.
func sessionsPortFixture(t *testing.T) (cwd, labsDir string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	profile := func(name string) map[string]any {
		return map[string]any{"name": name, "dir": filepath.Join(home, "profiles", name), "auth_method": "oauth"}
	}
	data, err := json.Marshal(map[string]any{
		"version": "1", "default_profile": "work",
		"profiles": map[string]any{"work": profile("work"), "labs": profile("labs")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".ccpm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ccpm", "config.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	cwd = filepath.Join(home, "my repo")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSession(t, filepath.Join(home, "profiles", "work", "projects", usage.EncodeCwd(cwd), portTestID+".jsonl"), portTestID, cwd, "hello")
	t.Cleanup(func() { sessionsPortFrom, sessionsPortTo, sessionsJSON = "", "", false })
	sessionsPortFrom, sessionsPortTo, sessionsJSON = "work", "labs", false
	return cwd, filepath.Join(home, "profiles", "labs")
}

func TestSessionsPortPrintsTheExactResumeCommand(t *testing.T) {
	cwd, labsDir := sessionsPortFixture(t)
	out, err := captureStdoutErr(t, func() error { return runSessionsPort(nil, []string{portTestID}) })
	if err != nil {
		t.Fatalf("port: %v", err)
	}
	want := "cd '" + cwd + "' && ccpm run labs -- --resume " + portTestID
	if !strings.Contains(out, want) {
		t.Errorf("output lacks %q:\n%s", want, out)
	}
	if _, err := os.Stat(filepath.Join(labsDir, "projects", usage.EncodeCwd(cwd), portTestID+".jsonl")); err != nil {
		t.Errorf("transcript was not copied: %v", err)
	}

	out, err = captureStdoutErr(t, func() error { return runSessionsPort(nil, []string{portTestID}) })
	if err != nil || !strings.Contains(out, "already has this session") {
		t.Errorf("second port: err=%v output:\n%s", err, out)
	}
}

func TestSessionsPortJSON(t *testing.T) {
	cwd, _ := sessionsPortFixture(t)
	sessionsJSON = true
	out, err := captureStdoutErr(t, func() error { return runSessionsPort(nil, []string{portTestID}) })
	if err != nil {
		t.Fatalf("port: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if got["cwd"] != cwd || got["to"] != "labs" || got["already_ported"] != false || !strings.Contains(got["next_command"].(string), "--resume "+portTestID) {
		t.Errorf("json = %v", got)
	}
}

func TestSessionsPortUnknownProfile(t *testing.T) {
	sessionsPortFixture(t)
	sessionsPortTo = "ghost"
	_, err := captureStdoutErr(t, func() error { return runSessionsPort(nil, []string{portTestID}) })
	if err == nil || !strings.Contains(err.Error(), `profile "ghost" not found`) {
		t.Errorf("err = %v", err)
	}
}

// A cd into a directory that is gone would stop the printed command before it
// ran, and Claude Code does not need the recorded directory to resume.
func TestSessionsPortMissingCwdOmitsCd(t *testing.T) {
	cwd, _ := sessionsPortFixture(t)
	if err := os.RemoveAll(cwd); err != nil {
		t.Fatal(err)
	}
	out, err := captureStdoutErr(t, func() error { return runSessionsPort(nil, []string{portTestID}) })
	if err != nil {
		t.Fatalf("port: %v", err)
	}
	if strings.Contains(out, "cd ") || !strings.Contains(out, "no longer exists") || !strings.Contains(out, "ccpm run labs -- --resume "+portTestID) {
		t.Errorf("output:\n%s", out)
	}
}

func TestShellArg(t *testing.T) {
	for in, want := range map[string]string{
		"/Users/a/repo-1.2_x": "/Users/a/repo-1.2_x",
		"/a b":                "'/a b'",
		"/a$(id)":             "'/a$(id)'",
		"/it's":               `'/it'\''s'`,
		"":                    "''",
	} {
		if got := shellArg(in); got != want {
			t.Errorf("shellArg(%q) = %s, want %s", in, got, want)
		}
	}
}
