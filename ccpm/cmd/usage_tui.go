package cmd

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

// usageTabs and usageWindows drive the two switchable axes of the dashboard.
var usageTabs = []string{"Overview", "Days", "Models", "Projects", "Sessions"}

var usageWindows = []struct{ label, since string }{
	{"All", ""}, {"90d", "90d"}, {"30d", "30d"}, {"7d", "7d"},
}

type loadedUsage struct {
	sess *usage.Sessions
	day  *usage.Daily
}

type usageTUI struct {
	cfg      *config.Config
	profiles []string
	profIdx  int
	tab      int
	winIdx   int
	cursor   int
	cache    map[string]loadedUsage
	view     usage.View
	width    int
	height   int
}

var (
	tuiActiveTab  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("171")).Underline(true)
	tuiDim        = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	tuiActiveProf = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75"))
	tuiSelRow     = lipgloss.NewStyle().Foreground(lipgloss.Color("171")).Bold(true)
	tuiHeader     = lipgloss.NewStyle().Bold(true)
)

func newUsageTUI(cfg *config.Config, profiles []string, start string) *usageTUI {
	m := &usageTUI{cfg: cfg, profiles: profiles, cache: map[string]loadedUsage{}, width: 80, height: 24}
	for i, n := range profiles {
		if n == start {
			m.profIdx = i
		}
	}
	m.resolve()
	return m
}

// resolve syncs (once per profile) and rebuilds the view for the current
// profile + window selection.
func (m *usageTUI) resolve() {
	name := m.profiles[m.profIdx]
	d, ok := m.cache[name]
	if !ok {
		dir := m.cfg.Profiles[name].Dir
		sess, day, err := usage.Sync(dir)
		if err != nil {
			sess, day, _ = usage.Load(dir)
		}
		d = loadedUsage{sess, day}
		m.cache[name] = d
	}
	since, _ := usage.ParseSince(usageWindows[m.winIdx].since, time.Now())
	if d.sess != nil && d.day != nil {
		m.view = usage.BuildView(d.sess, d.day, since)
	} else {
		m.view = usage.View{}
	}
	m.cursor = 0
}

func (m *usageTUI) listLen() int {
	switch usageTabs[m.tab] {
	case "Days":
		return len(m.view.ByDay)
	case "Models":
		return len(m.view.ByModel)
	case "Projects":
		return len(m.view.ByProject)
	case "Sessions":
		return len(m.view.Sessions)
	}
	return 0
}

func (m *usageTUI) Init() tea.Cmd { return nil }

func (m *usageTUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "left", "h":
			m.tab = (m.tab - 1 + len(usageTabs)) % len(usageTabs)
			m.cursor = 0
		case "right", "l", "tab":
			m.tab = (m.tab + 1) % len(usageTabs)
			m.cursor = 0
		case "[":
			m.profIdx = (m.profIdx - 1 + len(m.profiles)) % len(m.profiles)
			m.resolve()
		case "]", "p":
			m.profIdx = (m.profIdx + 1) % len(m.profiles)
			m.resolve()
		case "w":
			m.winIdx = (m.winIdx + 1) % len(usageWindows)
			m.resolve()
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < m.listLen()-1 {
				m.cursor++
			}
		case "g", "home":
			m.cursor = 0
		case "G", "end":
			if n := m.listLen(); n > 0 {
				m.cursor = n - 1
			}
		}
	}
	return m, nil
}

