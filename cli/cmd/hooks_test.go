package cmd

import (
	"testing"
)

func TestIsKnownHookEvent(t *testing.T) {
	cases := map[string]bool{
		"PreToolUse":       true,
		"PostToolUse":      true,
		"SessionStart":     true,
		"UserPromptSubmit": true,
		"SomethingMade":    false,
		"":                 false,
	}
	for in, want := range cases {
		if got := isKnownHookEvent(in); got != want {
			t.Errorf("isKnownHookEvent(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestEnsureHooksRoot_CreatesWhenMissing(t *testing.T) {
	frag := map[string]interface{}{}
	root := ensureHooksRoot(frag)
	if root == nil {
		t.Fatal("ensureHooksRoot returned nil")
	}
	if _, ok := frag["hooks"].(map[string]interface{}); !ok {
		t.Error("frag.hooks was not initialized")
	}

	// Idempotent: second call returns same map.
	root["PreToolUse"] = []interface{}{"marker"}
	again := ensureHooksRoot(frag)
	if len(again["PreToolUse"].([]interface{})) != 1 {
		t.Error("second call should preserve existing state")
	}
}

func TestDescribeHookCommands(t *testing.T) {
	entry := map[string]interface{}{
		"matcher": "Bash",
		"hooks": []interface{}{
			map[string]interface{}{"type": "command", "command": "echo A"},
			map[string]interface{}{"command": "echo B"}, // type defaults to "command"
		},
	}
	got := describeHookCommands(entry)
	want := `command="echo A", command="echo B"`
	if got != want {
		t.Errorf("describeHookCommands = %q, want %q", got, want)
	}

	empty := describeHookCommands(map[string]interface{}{})
	if empty != "(no commands)" {
		t.Errorf("empty case = %q, want %q", empty, "(no commands)")
	}
}
