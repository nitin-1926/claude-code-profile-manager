package wizard

import (
	"bytes"
	"strings"
	"testing"
)

func TestPromptImportSource_NoOptionsAvailable(t *testing.T) {
	// No default tree, no existing profiles → only Scratch is valid.
	var out bytes.Buffer
	d, err := PromptImportSource(strings.NewReader(""), &out, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if d.Source != SourceScratch {
		t.Errorf("want SourceScratch, got %v", d.Source)
	}
}

func TestPromptImportSource_DefaultChoiceIsScratch(t *testing.T) {
	var out bytes.Buffer
	// User hits enter with no input.
	d, err := PromptImportSource(strings.NewReader("\n"), &out, []string{"work"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if d.Source != SourceScratch {
		t.Errorf("empty input should default to SourceScratch, got %v", d.Source)
	}
}

func TestPromptImportSource_ImportFromDefault(t *testing.T) {
	var out bytes.Buffer
	d, err := PromptImportSource(strings.NewReader("2\n"), &out, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if d.Source != SourceDefault {
		t.Errorf("want SourceDefault, got %v", d.Source)
	}
	if len(d.Targets) == 0 {
		t.Error("default import should pre-select targets")
	}
}

func TestPromptImportSource_ImportFromProfile_ByIndex(t *testing.T) {
	var out bytes.Buffer
	// option 2 = Import from profile (no default → profile is second)
	// then "1" to pick first profile by index
	d, err := PromptImportSource(strings.NewReader("2\n1\n"), &out, []string{"work", "home"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if d.Source != SourceProfile {
		t.Fatalf("want SourceProfile, got %v", d.Source)
	}
	if d.ProfileName != "work" {
		t.Errorf("want profile work, got %q", d.ProfileName)
	}
}

func TestPromptImportSource_ImportFromProfile_ByName(t *testing.T) {
	var out bytes.Buffer
	// With default available: 1 scratch, 2 default, 3 profile
	d, err := PromptImportSource(strings.NewReader("3\nhome\n"), &out, []string{"work", "home"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if d.Source != SourceProfile {
		t.Fatalf("want SourceProfile, got %v", d.Source)
	}
	if d.ProfileName != "home" {
		t.Errorf("want profile home, got %q", d.ProfileName)
	}
}

func TestPromptImportSource_InvalidChoice(t *testing.T) {
	var out bytes.Buffer
	_, err := PromptImportSource(strings.NewReader("99\n"), &out, []string{"work"}, true)
	if err == nil {
		t.Error("expected error for invalid choice")
	}
}
