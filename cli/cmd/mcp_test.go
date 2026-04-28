package cmd

import (
	"reflect"
	"strings"
	"testing"
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
