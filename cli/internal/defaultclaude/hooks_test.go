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
		t.Fatal(err)
	}

	got, err := LoadHookEntries()
	if err != nil {
		t.Fatalf("LoadHookEntries: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 flattened hooks, got %d: %+v", len(got), got)
	}

	// Entries are sorted event→index→subidx.
	if got[0].Event != "PreToolUse" || got[0].Matcher != "Bash" || got[0].SubIdx != 0 {
		t.Errorf("first entry unexpected: %+v", got[0])
	}
	if got[1].SubIdx != 1 || got[1].Type != "command" {
		t.Errorf("second sub-command missing default type: %+v", got[1])
	}
	if got[2].Index != 1 || got[2].Matcher != "" {
		t.Errorf("second matcher block unexpected: %+v", got[2])
	}
	if got[3].Event != "SessionStart" {
		t.Errorf("SessionStart should come last alphabetically; got %+v", got[3])
	}

	// IDs must be unique so the picker can key on them.
	seen := map[string]bool{}
	for _, e := range got {
		if seen[e.ID()] {
			t.Errorf("duplicate ID: %s", e.ID())
		}
		seen[e.ID()] = true
	}
}

func TestLoadHookEntries_MalformedJSONIsSurfaced(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	path := filepath.Join(tmp, ".claude")
	os.MkdirAll(path, 0700)
	os.WriteFile(filepath.Join(path, "settings.json"), []byte("not json"), 0600)

	if _, err := LoadHookEntries(); err == nil {
		t.Error("malformed settings.json should surface a parse error")
	}
}
