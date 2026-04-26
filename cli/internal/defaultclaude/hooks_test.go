package defaultclaude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadHookEntries_MissingFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	got, err := LoadHookEntries()
	if err != nil {
		t.Fatalf("missing file should be non-error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty, got %+v", got)
	}
}

func TestLoadHookEntries_FlattensEvents(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	path := filepath.Join(tmp, ".claude")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	body := `{
		"hooks": {
			"PreToolUse": [
				{"matcher": "Bash", "hooks": [
					{"type": "command", "command": "echo pre"},
					{"command": "second"}
				]},
				{"matcher": "", "hooks": [
					{"type": "command", "command": "catch-all"}
				]}
			],
			"SessionStart": [
				{"hooks": [{"type": "command", "command": "startup"}]}
			]
		}
	}`
	if err := os.WriteFile(filepath.Join(path, "settings.json"), []byte(body), 0600); err != nil {
