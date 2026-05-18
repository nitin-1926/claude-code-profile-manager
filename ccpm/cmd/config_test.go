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

// captureStdout runs fn while redirecting os.Stdout to a buffer and returns
// what was written. Used to assert what `runConfigGet` prints.
func captureStdout(t *testing.T, fn func() error) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	errCh := make(chan error, 1)
	go func() { errCh <- fn() }()
	_ = w.Close
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() { _, _ = io.Copy(&buf, r); close(done) }()
	if err := <-errCh; err != nil {
		t.Fatalf("fn: %v", err)
	}
	_ = w.Close()
	<-done
	return buf.String()
}

func writeConfig(t *testing.T, tmp string, cfg *config.Config) {
	t.Helper()
	ccpmDir := filepath.Join(tmp, ".ccpm")
	if err := os.MkdirAll(ccpmDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("config.Save: %v", err)
	}
}

func TestConfigGetDefaultDirReturnsProfilePath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	profDir := filepath.Join(tmp, ".ccpm", "profiles", "demo")
	if err := os.MkdirAll(profDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Version:        "1",
		DefaultProfile: "demo",
		Profiles: map[string]config.ProfileConfig{
			"demo": {Name: "demo", Dir: profDir, AuthMethod: "oauth"},
		},
	}
	writeConfig(t, tmp, cfg)

	out := captureStdout(t, func() error { return runConfigGet(nil, []string{"default_dir"}) })
	got := strings.TrimSpace(out)
	if got != profDir {
		t.Errorf("default_dir = %q; want %q", got, profDir)
	}
}

func TestConfigGetDefaultDirEmptyWhenNoDefault(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	cfg := &config.Config{
		Version:        "1",
		DefaultProfile: "",
		Profiles:       map[string]config.ProfileConfig{},
	}
	writeConfig(t, tmp, cfg)

	out := captureStdout(t, func() error { return runConfigGet(nil, []string{"default_dir"}) })
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected empty output when no default profile; got %q", out)
	}
}

func TestConfigGetDefaultDirEmptyWhenProfileMissing(t *testing.T) {
	// Defensive: config points at a profile that has since been deleted —
	// we must print empty, not panic, not error. The shell wrapper relies on
	// empty-output-means-fall-through.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	cfg := &config.Config{
		Version:        "1",
		DefaultProfile: "ghost",
		Profiles:       map[string]config.ProfileConfig{}, // 'ghost' not present
	}
	writeConfig(t, tmp, cfg)

	out := captureStdout(t, func() error { return runConfigGet(nil, []string{"default_dir"}) })
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected empty output when default profile is missing from Profiles map; got %q", out)
	}
}
