package filetree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSymlinkToDirectory(t *testing.T) {
	tmp := t.TempDir()
	real := filepath.Join(tmp, "real")
	if err := os.MkdirAll(real, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	ok, err := SymlinkToDirectory(link)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected symlink to directory")
	}
	ok, err = SymlinkToDirectory(real)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("real directory is not symlink-to-dir")
	}
}
