package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

// TestUsageTUIRendersAllTabs drives the dashboard model through every tab,
// profile switch, and window change, asserting it renders without panicking on
// empty data (the common first-run state).
func TestUsageTUIRendersAllTabs(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Profiles: map[string]config.ProfileConfig{
		"work": {Name: "work", Dir: filepath.Join(dir, "work")},
		"labs": {Name: "labs", Dir: filepath.Join(dir, "labs")},
	}}

	m := newUsageTUI(cfg, []string{"labs", "work"}, "work")
	m.width, m.height = 100, 30

	for range usageTabs {
		out := m.View()
		if !strings.Contains(out, usageTabs[m.tab]) {
			t.Fatalf("current tab %q missing from view", usageTabs[m.tab])
		}
		if !strings.Contains(out, "tokens") {
			t.Fatalf("totals header missing on tab %q", usageTabs[m.tab])
		}
		m.Update(tea.KeyMsg{Type: tea.KeyRight}) // next tab
	}

	// Profile switch, window cycle, and scrolling must not panic.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.View() == "" {
		t.Fatal("empty view after interactions")
	}

	// Quit key returns a Quit command.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}); cmd == nil {
		t.Fatal("expected quit command from 'q'")
	}
}
