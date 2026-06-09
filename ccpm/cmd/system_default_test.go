package cmd

import (
	"runtime"
	"strings"
	"testing"
)

func TestBuildSystemDefaultPlistContainsRequiredKeys(t *testing.T) {
	out := buildSystemDefaultPlist("/Users/me/.ccpm/profiles/work")

	// Plist header is non-negotiable — without it launchd refuses the file.
	if !strings.HasPrefix(out, `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Fatal("plist must start with XML declaration")
	}
	if !strings.Contains(out, `<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"`) {
		t.Fatal("plist must declare Apple DOCTYPE")
	}
	for _, must := range []string{
		"<key>Label</key>",
		"<string>" + systemDefaultAgentLabel + "</string>",
		"<key>ProgramArguments</key>",
		"<string>/bin/launchctl</string>",
		"<string>setenv</string>",
		"<string>CLAUDE_CONFIG_DIR</string>",
		"<string>/Users/me/.ccpm/profiles/work</string>",
		"<key>RunAtLoad</key>",
		"<true/>",
	} {
		if !strings.Contains(out, must) {
			t.Errorf("plist missing required fragment: %q", must)
		}
	}
}

func TestBuildSystemDefaultPlistEscapesXmlSpecialsInPath(t *testing.T) {
	// macOS lets users put `&` `<` `>` in directory names (though rare).
	// We must escape them or launchd refuses to load the plist.
	out := buildSystemDefaultPlist("/Users/me/A&B<C>/profiles/x")

	if strings.Contains(out, "/Users/me/A&B<C>/") {
		t.Error("path with raw &/</> leaked into plist — launchd would reject as malformed XML")
	}
	if !strings.Contains(out, "/Users/me/A&amp;B&lt;C&gt;/profiles/x") {
		t.Errorf("path not properly XML-escaped; got:\n%s", out)
	}
}

func TestBuildSystemDefaultPlistIsDeterministic(t *testing.T) {
	// Same input → same output. Important because we don't want set-default
	// to constantly churn the plist file mtime (which would re-trigger
	// launchd on every set-default of the same profile).
	a := buildSystemDefaultPlist("/path/to/dir")
	b := buildSystemDefaultPlist("/path/to/dir")
	if a != b {
		t.Error("buildSystemDefaultPlist must be deterministic for the same input")
	}
}

func TestSystemDefaultPlistPathIsUserScoped(t *testing.T) {
	// The plist/LaunchAgents system-default mechanism is macOS-only (its
	// callers early-return on non-darwin), and the expected path below uses
	// macOS "/"-separated semantics that filepath.Join wouldn't match on Windows.
	if runtime.GOOS != "darwin" {
		t.Skip("plist/LaunchAgents system default is macOS-only")
	}
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	got, err := systemDefaultPlistPath()
	if err != nil {
		t.Fatal(err)
	}
	want := tmp + "/Library/LaunchAgents/" + systemDefaultAgentLabel + ".plist"
	if got != want {
		t.Errorf("plist path = %q; want %q", got, want)
	}
}

func TestXmlEscape(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain", "plain"},
		{"a&b", "a&amp;b"},
		{"<tag>", "&lt;tag&gt;"},
		{"a & b < c > d", "a &amp; b &lt; c &gt; d"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := xmlEscape(tc.in); got != tc.want {
			t.Errorf("xmlEscape(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}
