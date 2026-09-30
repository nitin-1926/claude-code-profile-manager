package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
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

// A decompression bomb must be refused, not written until the disk fills.
func TestExtractBundleCapsEntryAndTotalSize(t *testing.T) {
	oldFile, oldTotal := maxBundleFileBytes, maxBundleTotalBytes
	t.Cleanup(func() { maxBundleFileBytes, maxBundleTotalBytes = oldFile, oldTotal })
	maxBundleFileBytes, maxBundleTotalBytes = 10, 15
	tmp := t.TempDir()

	cases := map[string]map[string]string{
		"per-file": {"big.txt": strings.Repeat("x", 11)},
		"total":    {"a.txt": strings.Repeat("x", 8), "b.txt": strings.Repeat("y", 8)},
	}
	for name, entries := range cases {
		bundle := filepath.Join(tmp, name+".tar.gz")
		writeBundle(t, bundle, entries)
		dest := filepath.Join(tmp, name)
		if err := os.MkdirAll(dest, 0o700); err != nil {
			t.Fatal(err)
		}
		err := extractBundle(bundle, dest)
		if err == nil || !strings.Contains(err.Error(), "limit") {
			t.Errorf("%s: extractBundle error = %v, want a size-limit error", name, err)
		}
	}

	ok := filepath.Join(tmp, "ok.tar.gz")
	writeBundle(t, ok, map[string]string{"a.txt": strings.Repeat("x", 10)})
	dest := filepath.Join(tmp, "okdest")
	_ = os.MkdirAll(dest, 0o700)
	if err := extractBundle(ok, dest); err != nil {
		t.Errorf("entry exactly at the limit rejected: %v", err)
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

// bundleSandbox isolates HOME and returns it plus a helper that writes a file
// (creating parents) under it.
func bundleSandbox(t *testing.T) (string, func(rel, content string) string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CCPM_NO_TTY", "1")
	write := func(rel, content string) string {
		t.Helper()
		p := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	return home, write
}

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

func registerProfile(t *testing.T, name, dir string) {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.AddProfile(name, dir, "oauth")
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
}

func bundleEntries(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	out := map[string]string{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(tr)
		out[hdr.Name] = string(data)
	}
}

func setFlag(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()
	f := cmd.Flags().Lookup(name)
	old := f.Value.String()
	if err := f.Value.Set(value); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Value.Set(old) })
}

// Export must carry what makes the profile work elsewhere (share-store
// assets behind the profile's symlinks, its settings/MCP fragments) and
// leave out session history, which can hold pasted secrets.
func TestExportImportBundleRoundTrip(t *testing.T) {
	home, write := bundleSandbox(t)
	src := filepath.Join(home, ".ccpm", "profiles", "src")
	write(".ccpm/profiles/src/settings.json", `{"model":"opus"}`)
	write(".ccpm/profiles/src/projects/-repo/session.jsonl", `{"pasted":"sk-secret"}`)
	write(".ccpm/profiles/src/todos/t.json", `[]`)
	write(".ccpm/share/skills/foo/SKILL.md", "skill body")
	write(".ccpm/share/agents/bar.md", "agent body")
	write("private/id_rsa", "KEY")
	write(".ccpm/share/settings/src.json", `{"model":"opus"}`)
	write(".ccpm/share/settings/src.owned.json", `["model"]`)
	write(".ccpm/share/mcp/src.json", `{"srv":{"command":"srv-bin"}}`)
	symlinkOrSkip(t, filepath.Join(home, ".ccpm", "share", "skills", "foo"), filepath.Join(src, "skills", "foo"))
	symlinkOrSkip(t, filepath.Join(home, ".ccpm", "share", "agents", "bar.md"), filepath.Join(src, "agents", "bar.md"))
	symlinkOrSkip(t, filepath.Join(home, "private"), filepath.Join(src, "skills", "escape"))
	registerProfile(t, "src", src)

	bundle := filepath.Join(home, "src.ccpm.tar.gz")
	oldOut := exportOut
	exportOut = bundle
	t.Cleanup(func() { exportOut = oldOut })
	if err := runExport(exportCmd, []string{"src"}); err != nil {
		t.Fatalf("export: %v", err)
	}

	got := bundleEntries(t, bundle)
	for name, want := range map[string]string{
		"skills/foo/SKILL.md":              "skill body",
		"agents/bar.md":                    "agent body",
		".ccpm-bundle/settings.json":       `{"model":"opus"}`,
		".ccpm-bundle/settings.owned.json": `["model"]`,
		".ccpm-bundle/mcp.json":            `{"srv":{"command":"srv-bin"}}`,
	} {
		if got[name] != want {
			t.Errorf("bundle %s = %q, want %q", name, got[name], want)
		}
	}
	for name := range got {
		if strings.HasPrefix(name, "projects") || strings.HasPrefix(name, "todos") {
			t.Errorf("bundle carries session data %s", name)
		}
		if strings.HasPrefix(name, "skills/escape") {
			t.Errorf("bundle followed a symlink outside the share store: %s", name)
		}
	}

	setFlag(t, importBundleCmd, "profile", "dst")
	if err := runImportBundle(importBundleCmd, []string{bundle}); err != nil {
		t.Fatalf("import-bundle: %v", err)
	}
	dst := filepath.Join(home, ".ccpm", "profiles", "dst")
	for rel, want := range map[string]string{
		".ccpm/share/settings/dst.json":          `{"model":"opus"}`,
		".ccpm/share/settings/dst.owned.json":    `["model"]`,
		".ccpm/share/mcp/dst.json":               `{"srv":{"command":"srv-bin"}}`,
		".ccpm/profiles/dst/skills/foo/SKILL.md": "skill body",
	} {
		if b, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(rel))); err != nil || string(b) != want {
			t.Errorf("restored %s = %q, %v; want %q", rel, b, err, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, ".ccpm-bundle")); !os.IsNotExist(err) {
		t.Errorf("fragment staging dir left in the restored profile (err=%v)", err)
	}
}

// Bundles written before fragments were carried must still import.
func TestImportBundleOldFormat(t *testing.T) {
	home, _ := bundleSandbox(t)
	bundle := filepath.Join(home, "old.ccpm.tar.gz")
	writeBundle(t, bundle, map[string]string{"settings.json": `{"model":"opus"}`, "skills/foo/SKILL.md": "x"})
	setFlag(t, importBundleCmd, "profile", "old")
	if err := runImportBundle(importBundleCmd, []string{bundle}); err != nil {
		t.Fatalf("import-bundle old format: %v", err)
	}
	cfg, _ := config.Load()
	if _, ok := cfg.Profiles["old"]; !ok {
		t.Error("old-format bundle did not register the profile")
	}
}
