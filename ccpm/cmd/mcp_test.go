package cmd

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/manifest"
)

func TestParseKVSlice(t *testing.T) {
	cases := []struct {
		name    string
		in      []string
		want    map[string]interface{}
		wantErr string
	}{
		{
			name: "stdio env pairs",
			in:   []string{"FOO=bar", "BAZ=qux"},
			want: map[string]interface{}{"FOO": "bar", "BAZ": "qux"},
		},
		{
			name: "value may contain equals",
			in:   []string{"URL=https://host/path?q=1"},
			want: map[string]interface{}{"URL": "https://host/path?q=1"},
		},
		{
			name: "empty value permitted",
			in:   []string{"EMPTY="},
			want: map[string]interface{}{"EMPTY": ""},
		},
		{
			name:    "missing equals is rejected",
			in:      []string{"NO_EQUALS"},
			wantErr: "--env entry \"NO_EQUALS\" must be KEY=VALUE",
		},
		{
			name:    "leading equals rejected (empty key)",
			in:      []string{"=value"},
			wantErr: "must be KEY=VALUE",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseKVSlice(c.in, "--env")
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestBuildServerDef(t *testing.T) {
	stateStdio := &mcpState{transport: "stdio", command: "npx", args: []string{"-y", "@x/y"}}
	def, err := buildServerDef(stateStdio)
	if err != nil {
		t.Fatalf("stdio: %v", err)
	}
	if def["type"] != "stdio" || def["command"] != "npx" {
		t.Errorf("stdio def = %v", def)
	}
	args, _ := def["args"].([]interface{})
	if len(args) != 2 || args[0] != "-y" {
		t.Errorf("stdio args = %v", args)
	}

	stateHTTP := &mcpState{transport: "http", url: "https://example.com/mcp", headers: []string{"Authorization=Bearer x"}}
	def, err = buildServerDef(stateHTTP)
	if err != nil {
		t.Fatalf("http: %v", err)
	}
	if def["type"] != "http" || def["url"] != "https://example.com/mcp" {
		t.Errorf("http def = %v", def)
	}
	hdrs, _ := def["headers"].(map[string]interface{})
	if hdrs["Authorization"] != "Bearer x" {
		t.Errorf("http headers = %v", hdrs)
	}

	stateBad := &mcpState{transport: "stdio", command: "x", url: "https://oops"}
	if _, err := buildServerDef(stateBad); err == nil {
		t.Error("stdio + url should be rejected")
	}

	stateMissing := &mcpState{transport: "stdio"}
	if _, err := buildServerDef(stateMissing); err == nil {
		t.Error("stdio without command should error")
	}

	stateBadTransport := &mcpState{transport: "websocket", url: "x"}
	if _, err := buildServerDef(stateBadTransport); err == nil {
		t.Error("unknown transport should error")
	}
}

func TestTypeOfMCPDef(t *testing.T) {
	cases := map[string]struct {
		in   interface{}
		want string
	}{
		"explicit-http":   {map[string]interface{}{"type": "http", "url": "u"}, "http"},
		"command-stdio":   {map[string]interface{}{"command": "npx"}, "stdio"},
		"url-only":        {map[string]interface{}{"url": "u"}, "http"},
		"opaque":          {map[string]interface{}{"foo": "bar"}, "—"},
		"not-a-map":       {"string", "—"},
	}
	for name, c := range cases {
		if got := typeOfMCPDef(c.in); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}

func TestStringSliceContains(t *testing.T) {
	if !stringSliceContains([]string{"a", "b"}, "b") {
		t.Error("should find b")
	}
	if stringSliceContains([]string{"a", "b"}, "c") {
		t.Error("should not find c")
	}
	if stringSliceContains(nil, "a") {
		t.Error("nil slice must not match")
	}
}

// TestConcurrentManifestMutationsUnderLock pins the H7 fix: two concurrent
// mutators that follow the command pattern (withConfigLock around
// Load→mutate→Save) must not lose each other's manifest entries.
func TestConcurrentManifestMutationsUnderLock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	const n = 8
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			errs <- withConfigLock(func() error {
				return recordMCPInstall(fmt.Sprintf("server-%d", i), manifest.ScopeGlobal, nil)
			})
		}(i)
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("locked mutation failed: %v", err)
		}
	}

	m, err := manifest.Load()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("server-%d", i)
		if m.Find(id, manifest.KindMCP) == nil {
			t.Errorf("entry %q lost — read-modify-write race not serialized", id)
		}
	}
}
