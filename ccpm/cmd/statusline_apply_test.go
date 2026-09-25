package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

// scratchConfig points HOME at a temp dir holding a config with one profile,
// "work", and returns that home.
func scratchConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, ".ccpm", "profiles", "work")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, home, &config.Config{
		Version: "1",
		Profiles: map[string]config.ProfileConfig{
			"work": {Name: "work", Dir: dir, AuthMethod: "oauth"},
		},
	})
	return home
}

// captureStdoutErr is captureStdout for callers that need to assert on the
// error rather than fail the test on it.
func captureStdoutErr(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() { _, _ = io.Copy(&buf, r); close(done) }()
	ferr := fn()
	_ = w.Close()
	<-done
	return buf.String(), ferr
}

var sampleLayout = &config.StatusLineLayout{
	Row1: []string{"profile", "model"},
	Row2: []string{"cost"},
	Off:  []string{"workspace", "branch", "context", "effort", "five_hour", "seven_day"},
}

// TestApplyStatusLineLayoutKeepsAConcurrentChange is the reason the write
// re-reads config.json under the lock instead of saving the snapshot it was
// handed. The picker holds that snapshot for as long as a human takes to choose;
// a `ccpm add` in the meantime must survive the save. Every desktop status line
// save goes through this function, and until now nothing tested it.
func TestApplyStatusLineLayoutKeepsAConcurrentChange(t *testing.T) {
	scratchConfig(t)
	stale, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	// Another command adds a profile while the picker is open.
	fresh, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	fresh.Profiles["other"] = config.ProfileConfig{Name: "other", Dir: t.TempDir(), AuthMethod: "oauth"}
	if err := config.Save(fresh); err != nil {
		t.Fatal(err)
	}

	if _, err := captureStdoutErr(t, func() error { return applyStatusLineLayout(stale, "work", sampleLayout) }); err != nil {
		t.Fatalf("apply: %v", err)
	}

	after, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := after.Profiles["other"]; !ok {
		t.Error("a profile added while the picker was open was erased by the save")
	}
	got := after.Profiles["work"].StatusLine
	if got == nil || strings.Join(got.Row1, ",") != "profile,model" {
		t.Errorf("work's layout was not written: %+v", got)
	}
	if after.Settings.StatusLine != nil {
		t.Error("a per-profile save wrote the global layout too")
	}
}

func TestApplyStatusLineLayoutRefusesUnknownAndRemovedProfiles(t *testing.T) {
	scratchConfig(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := captureStdoutErr(t, func() error { return applyStatusLineLayout(cfg, "ghost", sampleLayout) }); err == nil {
		t.Error("an unknown profile was accepted")
	}

	// Present in the snapshot, gone by the time of the write.
	gone, _ := config.Load()
	delete(gone.Profiles, "work")
	if err := config.Save(gone); err != nil {
		t.Fatal(err)
	}
	_, err = captureStdoutErr(t, func() error { return applyStatusLineLayout(cfg, "work", sampleLayout) })
	if err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Errorf("profile removed mid-picker: err = %v, want 'no longer exists'", err)
	}
}

func TestApplyStatusLineLayoutGlobalAndReset(t *testing.T) {
	scratchConfig(t)
	cfg, _ := config.Load()
	if _, err := captureStdoutErr(t, func() error { return applyStatusLineLayout(cfg, "", sampleLayout) }); err != nil {
		t.Fatal(err)
	}
	after, _ := config.Load()
	if after.Settings.StatusLine == nil {
		t.Fatal("global layout was not written")
	}
	if _, err := captureStdoutErr(t, func() error { return applyStatusLineLayout(after, "", nil) }); err != nil {
		t.Fatal(err)
	}
	reset, _ := config.Load()
	if reset.Settings.StatusLine != nil {
		t.Error("reset did not clear the global layout")
	}
}

// TestWorkspaceLabelNeverEndsInASlash: safeLabel refuses a subpath that is too
// long or holds a control character, and the label used to be built as
// name + "/" + "", rendering "repo/".
func TestWorkspaceLabelNeverEndsInASlash(t *testing.T) {
	for name, sub := range map[string]string{
		"control character": "sub\x1bdir",
		"over the rune cap": strings.Repeat("d", 200),
	} {
		var in statusLineInput
		in.Workspace.ProjectDir = "/x/repo"
		in.Workspace.CurrentDir = "/x/repo/" + sub
		if got := workspaceLabel(in); got != "repo" {
			t.Errorf("%s: workspaceLabel = %q, want %q", name, got, "repo")
		}
	}
	var ok statusLineInput
	ok.Workspace.ProjectDir = "/x/repo"
	ok.Workspace.CurrentDir = "/x/repo/sub"
	if got := workspaceLabel(ok); got != "repo/sub" {
		t.Errorf("ordinary subdir: %q, want repo/sub", got)
	}
}
