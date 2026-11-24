package transcript

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

const portID = "7392c3d1-59b8-4832-9e0e-fc5d6e62b90a"

// portTurn is one user line plus one usage-bearing assistant line, in the shape
// Claude Code 2.1.318 wrote during the port spike. input is the assistant line's
// token count, so tests can tell exactly which turns were counted.
func portTurn(t *testing.T, sess, cwd, ts, msgID string, input int64) string {
	t.Helper()
	user := jl(t, map[string]any{
		"type": "user", "uuid": msgID + "-u", "parentUuid": nil, "isSidechain": false,
		"sessionId": sess, "cwd": cwd, "timestamp": ts,
		"message": map[string]any{"role": "user", "content": "prompt " + msgID},
	})
	asst := jl(t, map[string]any{
		"type": "assistant", "uuid": msgID + "-a", "parentUuid": msgID + "-u", "isSidechain": false,
		"sessionId": sess, "cwd": cwd, "timestamp": ts, "requestId": "req_" + msgID,
		"message": map[string]any{"id": msgID, "role": "assistant", "model": "claude-opus-5-5",
			"content": []any{map[string]any{"type": "text", "text": "pong"}},
			"usage":   map[string]any{"input_tokens": input, "output_tokens": 0}},
	})
	return user + "\n" + asst + "\n"
}

func portWrite(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func portRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

type portEnv struct {
	from, to Scope
	cwd      string // the session's first recorded directory
	rel      string // projects-relative, slash-separated
	src, dst string // the session's transcript in each profile
}

// portFixture is a session in profile "work" whose cwd drifts into a subdirectory
// partway through, and an empty profile "labs" with no projects/ directory yet.
func portFixture(t *testing.T) portEnv {
	t.Helper()
	root := t.TempDir()
	cwd := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(cwd, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	e := portEnv{
		from: Scope{Profile: "work", Dir: filepath.Join(root, "work")},
		to:   Scope{Profile: "labs", Dir: filepath.Join(root, "labs")},
		cwd:  cwd,
		rel:  usage.EncodeCwd(cwd) + "/" + portID + ".jsonl",
	}
	e.src = filepath.Join(e.from.Dir, "projects", filepath.FromSlash(e.rel))
	e.dst = filepath.Join(e.to.Dir, "projects", filepath.FromSlash(e.rel))
	ts := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	portWrite(t, e.src,
		`{"type":"queue-operation","sessionId":"`+portID+`"}`+"\n"+
			portTurn(t, portID, cwd, ts, "msg_first", 1000)+
			portTurn(t, portID, filepath.Join(cwd, "sub"), ts, "msg_second", 2000))
	return e
}

func TestPortCopiesSessionAndMakesItDiscoverable(t *testing.T) {
	e := portFixture(t)
	dir := strings.TrimSuffix(e.src, ".jsonl")
	ts := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	sub := portTurn(t, portID, e.cwd, ts, "msg_sub", 300)
	portWrite(t, filepath.Join(dir, "subagents", "agent-a.jsonl"), sub)
	portWrite(t, filepath.Join(dir, "subagents", "agent-a.meta.json"), `{"agentType":"general-purpose"}`)
	portWrite(t, filepath.Join(dir, "subagents", "workflows", "wf_1", "agent-b.jsonl"), sub)
	portWrite(t, filepath.Join(dir, "tool-results", "big.txt"), "output")
	before := portRead(t, e.src)

	res, err := Port(e.from, e.to, portID)
	if err != nil {
		t.Fatalf("Port: %v", err)
	}
	if res.AlreadyPorted || res.CwdMissing {
		t.Errorf("result = %+v, want a fresh port with a live cwd", res)
	}
	// The first cwd, not the subdirectory the session drifted into.
	if res.Cwd != e.cwd {
		t.Errorf("Cwd = %q, want the first recorded %q", res.Cwd, e.cwd)
	}
	if got := portRead(t, e.dst); got != before {
		t.Error("the copy's bytes differ from the source")
	}
	if got := portRead(t, e.src); got != before {
		t.Error("the source was modified")
	}
	if fi, err := os.Lstat(e.dst); err != nil || !fi.Mode().IsRegular() {
		t.Errorf("target must be a regular file, never a link: %v %v", fi, err)
	}
	dstDir := strings.TrimSuffix(e.dst, ".jsonl")
	for _, name := range []string{"agent-a.jsonl", "agent-a.meta.json", "workflows/wf_1/agent-b.jsonl"} {
		if _, err := os.Stat(filepath.Join(dstDir, "subagents", filepath.FromSlash(name))); err != nil {
			t.Errorf("subagent file %s did not travel: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dstDir, "tool-results")); !os.IsNotExist(err) {
		t.Errorf("tool-results should stay behind: %v", err)
	}

	// LoadIndex, not BuildIndex: this is what Resume reads, so the session has to
	// be on record the moment Port returns.
	entry := LoadIndex(e.to.Dir).Entries[portID]
	if entry == nil {
		t.Fatal("the session is not in the target's index")
	}
	if entry.Cwd != e.cwd || entry.RelPath != e.rel {
		t.Errorf("entry cwd=%q rel=%q, want %q %q", entry.Cwd, entry.RelPath, e.cwd, e.rel)
	}
	if len(entry.SubPaths) != 2 {
		t.Errorf("SubPaths = %v, want both subagent transcripts readable", entry.SubPaths)
	}
	path, err := ResolvePath(e.to.Dir, entry.RelPath)
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}
	page, err := ReadPage(path, 0, 10)
	if err != nil || page.Total != 4 {
		t.Errorf("reader saw %d turns (err %v), want 4", page.Total, err)
	}
}

func portDailyTotal(t *testing.T, dir string) int64 {
	t.Helper()
	_, day, err := usage.Sync(dir)
	if err != nil {
		t.Fatalf("Sync(%s): %v", dir, err)
	}
	var n int64
	for _, d := range day.Days {
		n += d.Tokens.Total()
	}
	return n
}

func portBlocksTotal(t *testing.T, dir string) int64 {
	t.Helper()
	blocks, err := usage.LoadBlocks(dir, time.Now())
	if err != nil {
		t.Fatalf("LoadBlocks(%s): %v", dir, err)
	}
	var n int64
	for _, b := range blocks {
		n += b.Total
	}
	return n
}

// The source's tokens were spent on the source's account. Counting them under
// the target would inflate its usage on the very session a handoff moves, which
// is also the most recently active one, so the 5-hour block is checked as well.
func TestPortCountsOnlyWhatIsAppendedAfterIt(t *testing.T) {
	e := portFixture(t)
	dir := strings.TrimSuffix(e.src, ".jsonl")
	ts := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	subRel := filepath.Join("subagents", "agent-a.jsonl")
	portWrite(t, filepath.Join(dir, subRel), portTurn(t, portID, e.cwd, ts, "msg_sub", 300))

	// The target already has usage of its own, so "unchanged" is not just "zero".
	own := filepath.Join(e.to.Dir, "projects", usage.EncodeCwd(e.cwd), "own.jsonl")
	portWrite(t, own, portTurn(t, "own", e.cwd, ts, "msg_own", 7))

	srcTotal := portDailyTotal(t, e.from.Dir)
	if srcTotal != 3300 {
		t.Fatalf("source total = %d, want 3300: the fixture must actually count", srcTotal)
	}
	srcBytes := portRead(t, e.src)
	if got := portDailyTotal(t, e.to.Dir); got != 7 {
		t.Fatalf("target total before the port = %d, want 7", got)
	}

	if _, err := Port(e.from, e.to, portID); err != nil {
		t.Fatalf("Port: %v", err)
	}

	if got := portDailyTotal(t, e.to.Dir); got != 7 {
		t.Errorf("target total after the port = %d, want 7 unchanged", got)
	}
	if got := portBlocksTotal(t, e.to.Dir); got != 7 {
		t.Errorf("target 5-hour blocks after the port = %d, want 7 unchanged", got)
	}
	sess, _, err := usage.Load(e.to.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Records[portID] != nil {
		t.Errorf("target gained a usage record before any new turn: %+v", sess.Records[portID])
	}

	// Claude Code appends in place under the same id once resumed.
	appendTo := func(path, line string) {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if _, err := f.WriteString(line); err != nil {
			t.Fatal(err)
		}
	}
	appendTo(e.dst, portTurn(t, portID, e.cwd, ts, "msg_in_labs", 50))
	appendTo(filepath.Join(strings.TrimSuffix(e.dst, ".jsonl"), subRel), portTurn(t, portID, e.cwd, ts, "msg_sub_labs", 5))

	if got := portDailyTotal(t, e.to.Dir); got != 62 {
		t.Errorf("target total after appending = %d, want 7+50+5", got)
	}
	if got := portBlocksTotal(t, e.to.Dir); got != 62 {
		t.Errorf("target 5-hour blocks after appending = %d, want 7+50+5", got)
	}
	sess, _, err = usage.Load(e.to.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if r := sess.Records[portID]; r == nil || r.Tokens.Total() != 55 {
		t.Errorf("target's record for the session = %+v, want only the 55 appended tokens", r)
	}

	// A store rebuild (what a storeVersion bump does) resets every cursor and
	// must still not hand the target the source's tokens.
	for _, name := range []string{"state.json", "sessions.json", "daily.json"} {
		if err := os.Remove(filepath.Join(usage.Dir(e.to.Dir), name)); err != nil {
			t.Fatal(err)
		}
	}
	if got := portDailyTotal(t, e.to.Dir); got != 62 {
		t.Errorf("target total after a store rebuild = %d, want 62", got)
	}

	// The source is untouched throughout.
	if got := portDailyTotal(t, e.from.Dir); got != srcTotal {
		t.Errorf("source total = %d, want %d unchanged", got, srcTotal)
	}
	if portRead(t, e.src) != srcBytes {
		t.Error("the source transcript changed")
	}
}

func TestPortExistingTarget(t *testing.T) {
	e := portFixture(t)
	if _, err := Port(e.from, e.to, portID); err != nil {
		t.Fatalf("first port: %v", err)
	}

	// Identical bytes: already ported, and still on record for Resume even if the
	// sidecar went missing in between.
	if err := os.Remove(IndexPath(e.to.Dir)); err != nil {
		t.Fatal(err)
	}
	res, err := Port(e.from, e.to, portID)
	if err != nil || !res.AlreadyPorted {
		t.Fatalf("second port = %+v, %v; want AlreadyPorted", res, err)
	}
	if LoadIndex(e.to.Dir).Entries[portID] == nil {
		t.Error("an already-ported session is not in the target's index")
	}

	// Different bytes: the target continued, so its copy is never overwritten.
	continued := portRead(t, e.dst) + portTurn(t, portID, e.cwd, "2026-06-27T10:00:00Z", "msg_in_labs", 50)
	portWrite(t, e.dst, continued)
	if _, err = Port(e.from, e.to, portID); err == nil || !strings.Contains(err.Error(), "different content") {
		t.Errorf("port over a continued copy: err = %v, want the different-content refusal", err)
	}
	if portRead(t, e.dst) != continued {
		t.Error("the target's continued copy was overwritten")
	}
}

func TestPortMissingCwdStillCopies(t *testing.T) {
	e := portFixture(t)
	if err := os.RemoveAll(e.cwd); err != nil {
		t.Fatal(err)
	}
	res, err := Port(e.from, e.to, portID)
	if err != nil {
		t.Fatalf("Port: %v", err)
	}
	if !res.CwdMissing || res.Cwd != e.cwd {
		t.Errorf("result = %+v, want CwdMissing with the recorded cwd", res)
	}
	if _, err := os.Stat(e.dst); err != nil {
		t.Errorf("the transcript was not copied: %v", err)
	}
}

func TestPortRefusals(t *testing.T) {
	t.Run("same profile", func(t *testing.T) {
		e := portFixture(t)
		if _, err := Port(e.from, e.from, portID); err == nil {
			t.Error("porting a session onto its own profile succeeded")
		}
	})
	t.Run("unknown session", func(t *testing.T) {
		e := portFixture(t)
		_, err := Port(e.from, e.to, "00000000-0000-0000-0000-000000000000")
		if err == nil || !strings.Contains(err.Error(), "has no session") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("pruned transcript", func(t *testing.T) {
		e := portFixture(t)
		if _, err := BuildIndex(e.from.Dir); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(e.src); err != nil {
			t.Fatal(err)
		}
		_, err := Port(e.from, e.to, portID)
		if err == nil || !strings.Contains(err.Error(), "no longer on disk") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("index path escaping the profile", func(t *testing.T) {
		e := portFixture(t)
		outside := filepath.Join(filepath.Dir(e.from.Dir), portID+".jsonl")
		portWrite(t, outside, "secret\n")
		if err := os.Remove(e.src); err != nil {
			t.Fatal(err)
		}
		ix := newIndex()
		ix.Entries[portID] = &Entry{SessionID: portID, RelPath: "../../" + portID + ".jsonl"}
		if err := saveIndex(e.from.Dir, ix); err != nil {
			t.Fatal(err)
		}
		if _, err := Port(e.from, e.to, portID); err == nil {
			t.Error("a tampered index path was followed out of the profile")
		}
	})
}

// Symlink cases sit apart because creating one needs Developer Mode on Windows.
func TestPortRefusesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests need Developer Mode on Windows")
	}
	link := func(t *testing.T, target, at string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, at); err != nil {
			t.Fatal(err)
		}
	}
	assertRefused := func(t *testing.T, e portEnv) {
		t.Helper()
		if _, err := Port(e.from, e.to, portID); err == nil {
			t.Error("Port succeeded across a symlink")
		}
	}

	t.Run("source transcript", func(t *testing.T) {
		e := portFixture(t)
		// Build the index first: the walk skips a link, so the entry must predate it.
		if _, err := BuildIndex(e.from.Dir); err != nil {
			t.Fatal(err)
		}
		real := e.src + ".real"
		if err := os.Rename(e.src, real); err != nil {
			t.Fatal(err)
		}
		link(t, real, e.src)
		assertRefused(t, e)
		if _, err := os.Lstat(e.dst); !os.IsNotExist(err) {
			t.Errorf("something was written to the target: %v", err)
		}
	})
	t.Run("subagent transcript leaves nothing behind", func(t *testing.T) {
		e := portFixture(t)
		outside := filepath.Join(filepath.Dir(e.from.Dir), "outside.jsonl")
		portWrite(t, outside, "secret\n")
		link(t, outside, filepath.Join(strings.TrimSuffix(e.src, ".jsonl"), "subagents", "agent-a.jsonl"))
		assertRefused(t, e)
		if _, err := os.Lstat(e.dst); !os.IsNotExist(err) {
			t.Errorf("the transcript was copied despite the refusal: %v", err)
		}
	})
	t.Run("target projects directory", func(t *testing.T) {
		e := portFixture(t)
		link(t, t.TempDir(), filepath.Join(e.to.Dir, "projects"))
		assertRefused(t, e)
	})
	t.Run("target encoded directory", func(t *testing.T) {
		e := portFixture(t)
		elsewhere := t.TempDir()
		link(t, elsewhere, filepath.Dir(e.dst))
		assertRefused(t, e)
		if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
			t.Errorf("wrote outside the profile: %v", entries)
		}
	})
	t.Run("target subagent directory", func(t *testing.T) {
		e := portFixture(t)
		ts := time.Now().UTC().Format(time.RFC3339)
		portWrite(t, filepath.Join(strings.TrimSuffix(e.src, ".jsonl"), "subagents", "agent-a.jsonl"), portTurn(t, portID, e.cwd, ts, "msg_sub", 1))
		elsewhere := t.TempDir()
		link(t, elsewhere, strings.TrimSuffix(e.dst, ".jsonl"))
		assertRefused(t, e)
		if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
			t.Errorf("wrote outside the profile: %v", entries)
		}
		if _, err := os.Lstat(e.dst); !os.IsNotExist(err) {
			t.Errorf("the transcript was copied despite the refusal: %v", err)
		}
	})
	t.Run("target transcript", func(t *testing.T) {
		e := portFixture(t)
		link(t, e.src, e.dst)
		assertRefused(t, e)
	})
}

func TestAdoptRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	rel := "enc/" + portID + ".jsonl"
	full := filepath.Join(dir, "projects", "enc", portID+".jsonl")
	portWrite(t, full, "mine\n")
	if err := usage.Adopt(dir, map[string][]byte{rel: []byte("theirs\n")}); err == nil {
		t.Error("Adopt overwrote an existing transcript")
	}
	if got := portRead(t, full); got != "mine\n" {
		t.Errorf("content = %q", got)
	}
}
