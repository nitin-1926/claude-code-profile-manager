//go:build windows

package atomicwrite

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestApplyWaitsOutAnOpenReader is the regression for concurrent ccpm
// commands on Windows: a rename over a file another handle has open fails
// with "Access is denied" until that handle closes, which made a config save
// lose its write whenever a parallel `ccpm run` was reading config.json.
func TestApplyWaitsOutAnOpenReader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(path) // no FILE_SHARE_DELETE, as every Go open
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = reader.Close()
	}()

	if err := Apply([]FileChange{WriteFile(path, []byte("new"), 0o600)}); err != nil {
		t.Fatalf("Apply with a reader open: %v", err)
	}
	got, err := ReadFile(path)
	if err != nil || string(got) != "new" {
		t.Fatalf("got %q, %v; want \"new\"", got, err)
	}
}
