package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestEncodeCwd pins the encoding to what Claude Code actually writes on disk:
// one dash per non-alphanumeric character, nothing collapsed, nothing trimmed.
func TestEncodeCwd(t *testing.T) {
	cases := []struct {
		name string
		cwd  string
		want string
	}{
		{"leading slash keeps its dash", "/Users/x/Desktop/Foo", "-Users-x-Desktop-Foo"},
		{"dotfile run is not collapsed", "/Users/x/.claude-brain", "-Users-x--claude-brain"},
		{"trailing separator keeps its dash", "/Users/x/", "-Users-x-"},
		{"consecutive separators each map", "/a//b", "-a--b"},
		{"already alphanumeric is unchanged", "abc123", "abc123"},
		{"empty stays empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EncodeCwd(tc.cwd); got != tc.want {
				t.Errorf("EncodeCwd(%q) = %q, want %q", tc.cwd, got, tc.want)
			}
		})
	}
}

// TestEncodeCwdMatchesRealLayout is the regression that the old implementation
// could never have passed: for every project directory in a real profile, the
// cwd recorded inside its transcripts must encode back to that directory name.
// Skips when the machine has no profiles, so CI on Linux/Windows still runs it
// as a no-op rather than failing.
func TestEncodeCwdMatchesRealLayout(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	profiles, err := os.ReadDir(filepath.Join(home, ".ccpm", "profiles"))
	if err != nil {
		t.Skip("no ~/.ccpm/profiles on this machine")
	}

	checked := 0
	for _, p := range profiles {
		if !p.IsDir() {
			continue
		}
		root := filepath.Join(home, ".ccpm", "profiles", p.Name(), "projects")
		dirs, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, d := range dirs {
			if !d.IsDir() {
				continue
			}
			cwd := firstCwdIn(filepath.Join(root, d.Name()))
			if cwd == "" {
				continue
			}
			checked++
			if got := EncodeCwd(cwd); got != d.Name() {
				t.Errorf("EncodeCwd(%q) = %q, but the directory on disk is %q", cwd, got, d.Name())
			}
		}
	}
	if checked == 0 {
		t.Skip("no transcripts with a cwd field found")
	}
	t.Logf("verified %d project directories", checked)
}

// firstCwdIn returns the cwd recorded by the first transcript line in dir that
// carries one, or "" when none does.
func firstCwdIn(dir string) string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		dec := json.NewDecoder(f)
		for i := 0; i < 40; i++ {
			var line struct {
				Cwd string `json:"cwd"`
			}
			if err := dec.Decode(&line); err != nil {
				break
			}
			if line.Cwd != "" {
				f.Close()
				return line.Cwd
			}
		}
		f.Close()
	}
	return ""
}

// TestEncodeCwdMatchesJavaScriptOnAstralCharacters pins the one place the Go
// and JavaScript encoders can disagree.
//
// Claude Code's encoder is JS and replaces per UTF-16 code unit. A non-BMP
// character is a surrogate pair there and produces TWO dashes; a Go `for range`
// sees one rune and produced one. The expected values below were taken from
// node: String.prototype.replace(/[^a-zA-Z0-9]/g, "-").
//
// It matters because every caller uses the result as a directory lookup, so a
// mismatch is not an error — it is silently finding nothing, which is exactly
// how the collapse-and-trim bug hid.
func TestEncodeCwdMatchesJavaScriptOnAstralCharacters(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"/Users/x/\U0001F680proj", "-Users-x---proj"}, // rocket: one surrogate pair
		{"/Users/x/proj", "-Users-x-proj"},             // BMP-only control case
		{"/a/éb", "-a--b"},                             // é is BMP: still one dash
		{"\U0001F600\U0001F600", "----"},               // two pairs, four dashes
	} {
		if got := EncodeCwd(tc.in); got != tc.want {
			t.Errorf("EncodeCwd(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// A link (or any other non-regular entry) named *.jsonl must never reach a
// caller. The profile directory can be shared or restored from elsewhere, and
// every caller opens what it is handed: projects/-x/evil.jsonl -> /dev/zero made
// `ccpm usage`, the SessionEnd hook, the TUI and the desktop Usage tab read
// without bound.
func TestWalkTranscriptsSkipsNonRegularFiles(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "projects", "-repo")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "s1.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "secret")
	if err := os.WriteFile(outside, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(proj, "evil.jsonl")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	var got []string
	if err := WalkTranscripts(dir, "", func(abs, rel string) error {
		got = append(got, filepath.Base(rel))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "s1.jsonl" {
		t.Fatalf("walked %v, want only [s1.jsonl]", got)
	}
}

// End to end: a link to an endless device must not stall Sync.
func TestSyncIgnoresLinkToEndlessDevice(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no /dev/zero")
	}
	dir := t.TempDir()
	proj := filepath.Join(dir, "projects", "-repo")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/dev/zero", filepath.Join(proj, "evil.jsonl")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	done := make(chan error, 1)
	go func() { _, _, err := Sync(dir); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Sync is still reading a symlink to /dev/zero after 10s")
	}
}
