package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
