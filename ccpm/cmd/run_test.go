package cmd

import (
	"reflect"
	"testing"
)

// TestExtractCCPMRunFlags covers the §2.1 contract: unknown flags after the
// profile name pass through untouched to claude; ccpm-owned flags (--ccpm-env,
// --help, --version, and the -- separator) are intercepted here.
func TestExtractCCPMRunFlags(t *testing.T) {
	cases := []struct {
		name        string
		input       []string
		wantForward []string
		wantEnv     []string
		wantHelp    bool
		wantVersion bool
		wantErr     bool
	}{
		{
			name:        "single unknown flag forwards",
			input:       []string{"work", "--dangerously-skip-permissions"},
			wantForward: []string{"work", "--dangerously-skip-permissions"},
		},
		{
			name:        "unknown flag with value forwards",
			input:       []string{"work", "--model", "claude-sonnet-4-6"},
			wantForward: []string{"work", "--model", "claude-sonnet-4-6"},
		},
		{
			name:        "--ccpm-env two-token form",
			input:       []string{"--ccpm-env", "FOO=bar", "work"},
			wantForward: []string{"work"},
			wantEnv:     []string{"FOO=bar"},
		},
		{
			name:        "--ccpm-env= equals form",
			input:       []string{"--ccpm-env=FOO=bar", "work"},
			wantForward: []string{"work"},
			wantEnv:     []string{"FOO=bar"},
		},
		{
			name:        "-- separator passes everything after verbatim",
			input:       []string{"work", "--", "--help", "--ccpm-env=NOPE=1"},
			wantForward: []string{"work", "--help", "--ccpm-env=NOPE=1"},
		},
		{
			name:     "--help before profile trips help",
			input:    []string{"--help"},
			wantHelp: true,
		},
		{
			name:        "--version trips version",
			input:       []string{"--version"},
			wantVersion: true,
		},
		{
			name:    "--ccpm-env without value errors",
			input:   []string{"--ccpm-env"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			forward, env, help, ver, err := extractCCPMRunFlags(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(forward, tc.wantForward) && (len(forward) != 0 || len(tc.wantForward) != 0) {
				t.Fatalf("forward: got %v, want %v", forward, tc.wantForward)
			}
			if !reflect.DeepEqual(env, tc.wantEnv) && (len(env) != 0 || len(tc.wantEnv) != 0) {
				t.Fatalf("env: got %v, want %v", env, tc.wantEnv)
			}
			if help != tc.wantHelp {
				t.Fatalf("help: got %v, want %v", help, tc.wantHelp)
			}
			if ver != tc.wantVersion {
				t.Fatalf("version: got %v, want %v", ver, tc.wantVersion)
			}
		})
	}
}

func TestParseEnvKVs(t *testing.T) {
	cases := []struct {
		name    string
		input   []string
		want    map[string]string
		wantErr bool
	}{
		{name: "empty", input: nil, want: nil},
		{name: "single kv", input: []string{"FOO=bar"}, want: map[string]string{"FOO": "bar"}},
		{name: "value with equals", input: []string{"URL=https://x.example/path?a=b"}, want: map[string]string{"URL": "https://x.example/path?a=b"}},
		{name: "missing equals", input: []string{"FOO"}, wantErr: true},
		{name: "empty key", input: []string{"=bar"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseEnvKVs(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len mismatch: got %v, want %v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Fatalf("key %q: got %q, want %q", k, got[k], v)
				}
			}
		})
	}
}
