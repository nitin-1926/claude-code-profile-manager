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
