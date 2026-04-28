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

