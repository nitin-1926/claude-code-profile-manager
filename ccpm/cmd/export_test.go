package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// writeBundle builds a gzipped tar at path from the given name→content entries.
// A name ending in "/" is written as a directory entry.
func writeBundle(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, content := range entries {
		if name[len(name)-1] == '/' {
			if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeDir, Mode: 0o700}); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o600, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestExtractBundleRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	bundle := filepath.Join(tmp, "p.tar.gz")
	writeBundle(t, bundle, map[string]string{
		"settings.json":       `{"a":1}`,
		"skills/":             "",
		"skills/foo/SKILL.md": "hello",
	})

	dest := filepath.Join(tmp, "out")
	if err := os.MkdirAll(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := extractBundle(bundle, dest); err != nil {
		t.Fatalf("extractBundle: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dest, "skills", "foo", "SKILL.md"))
	if err != nil {
		t.Fatalf("expected extracted file: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("content = %q, want %q", got, "hello")
	}
	if _, err := os.Stat(filepath.Join(dest, "settings.json")); err != nil {
		t.Errorf("settings.json missing: %v", err)
	}
}

func TestExtractBundleRejectsTraversal(t *testing.T) {
	tmp := t.TempDir()

	for _, evil := range []string{"../escape.txt", "../../etc/passwd", "a/../../escape"} {
		bundle := filepath.Join(tmp, "evil.tar.gz")
		writeBundle(t, bundle, map[string]string{evil: "pwned"})

		dest := filepath.Join(tmp, "dest")
		if err := os.MkdirAll(dest, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := extractBundle(bundle, dest); err == nil {
			t.Errorf("entry %q: expected extractBundle to reject path traversal, got nil", evil)
		}
		// Ensure nothing escaped above dest.
		if _, err := os.Stat(filepath.Join(tmp, "escape.txt")); err == nil {
			t.Fatalf("entry %q escaped the destination directory!", evil)
		}
		os.RemoveAll(dest)
		os.Remove(bundle)
	}
}

func TestExtractBundleRejectsAbsolutePath(t *testing.T) {
	tmp := t.TempDir()
	bundle := filepath.Join(tmp, "abs.tar.gz")
	// Build a tar with an absolute path entry directly (writeBundle would clean it).
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "/tmp/ccpm-abs-escape", Typeflag: tar.TypeReg, Mode: 0o600, Size: 3})
	_, _ = tw.Write([]byte("bad"))
	_ = tw.Close()
	_ = gz.Close()
	if err := os.WriteFile(bundle, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(tmp, "dest")
	_ = os.MkdirAll(dest, 0o700)
	// On unix the absolute path is cleaned relative; the key property is no
	// write escapes dest. Accept either a rejection or a contained write.
	_ = extractBundle(bundle, dest)
	if _, err := os.Stat("/tmp/ccpm-abs-escape"); err == nil {
		os.Remove("/tmp/ccpm-abs-escape")
		t.Fatal("absolute-path entry escaped the destination directory!")
	}
}
