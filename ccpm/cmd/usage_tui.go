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

func (m *usageTUI) View() string {
	var b strings.Builder

	// Profile bar + window indicator.
	var profs []string
	for i, n := range m.profiles {
		if i == m.profIdx {
			profs = append(profs, tuiActiveProf.Render("⬢ "+n))
		} else {
			profs = append(profs, tuiDim.Render(n))
		}
	}
	b.WriteString(strings.Join(profs, "  "))
	b.WriteString("   " + tuiDim.Render("window: ") + tuiHeader.Render("["+usageWindows[m.winIdx].label+"]"))
	b.WriteString("\n")

	// Tab bar.
	var tabs []string
	for i, t := range usageTabs {
		if i == m.tab {
			tabs = append(tabs, tuiActiveTab.Render(t))
		} else {
			tabs = append(tabs, tuiDim.Render(t))
		}
	}
	b.WriteString(strings.Join(tabs, tuiDim.Render(" │ ")))
	b.WriteString("\n\n")

	// Totals header (always shown).
	v := m.view
	fmt.Fprintf(&b, "  Total %s tokens · %s messages\n",
		tuiHeader.Render(humanTokens(v.Totals.Total())), humanInt(v.Messages))
	b.WriteString("  " + tuiDim.Render(fmt.Sprintf("input %s · output %s · cache-write %s · cache-read %s",
		humanTokens(v.Totals.Input), humanTokens(v.Totals.Output),
		humanTokens(v.Totals.CacheCreation), humanTokens(v.Totals.CacheRead))) + "\n\n")

	// Body — leave room for header (~7 lines) + footer (2).
	bodyRows := m.height - 9
	if bodyRows < 4 {
		bodyRows = 4
	}
	b.WriteString(m.body(bodyRows))

	// Footer.
	b.WriteString("\n" + tuiDim.Render("←/→ tabs · ↑/↓ scroll · [ ] profile · w window · q quit"))
	return b.String()
}

func (m *usageTUI) body(maxRows int) string {
	v := m.view
	switch usageTabs[m.tab] {
	case "Overview":
		weeks := 26
		if s := usageWindows[m.winIdx].since; s != "" {
			weeks = heatmapWeeks(mustSince(s))
		}
		out := usage.RenderHeatmap(m.cache[m.profiles[m.profIdx]].day.Days, time.Now(), weeks, true)
		out += "\n"
		out += renderTUIRows(namedRows(topN(v.ByModel, 5), 26), -1, maxRows-12)
		return out
	case "Days":
		rows := make([]usage.NamedTotal, 0, len(v.ByDay))
		for _, d := range v.ByDay {
			rows = append(rows, usage.NamedTotal{Name: d.Date, Tokens: d.Tokens})
		}
		// Newest first for the list.
		reverse(rows)
		return renderTUIRows(namedRows(rows, 12), m.cursor, maxRows)
	case "Models":
		return renderTUIRows(namedRows(v.ByModel, 28), m.cursor, maxRows)
	case "Projects":
		pr := make([]usage.NamedTotal, len(v.ByProject))
		for i, p := range v.ByProject {
			pr[i] = usage.NamedTotal{Name: baseName(p.Name), Tokens: p.Tokens}
		}
		return renderTUIRows(namedRows(pr, 28), m.cursor, maxRows)
	case "Sessions":
		var rows []string
		for _, s := range v.Sessions {
			last := s.LastTS
			if t, err := time.Parse(time.RFC3339, s.LastTS); err == nil {
				last = t.Local().Format("2006-01-02 15:04")
			}
			rows = append(rows, fmt.Sprintf("%-16s  %-22s  %10s  %s",
				last, truncate(baseName(s.Cwd), 22), humanTokens(s.Tokens.Total()), tuiDim.Render(s.SessionID)))
		}
		return renderTUIRows(rows, m.cursor, maxRows)
	}
	return ""
}

// namedRows formats a breakdown into aligned, bar-decorated row strings.
func namedRows(rows []usage.NamedTotal, nameW int) []string {
	if len(rows) == 0 {
		return nil
	}
	var max int64
	for _, r := range rows {
		if t := r.Tokens.Total(); t > max {
			max = t
		}
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%-*s %10s  %s", nameW, truncate(r.Name, nameW), humanTokens(r.Tokens.Total()), miniBar(r.Tokens.Total(), max, 20)))
	}
	return out
}

// renderTUIRows shows a scrollable window of rows around cursor (cursor<0 = no
// selection, e.g. the Overview's static model summary).
func renderTUIRows(rows []string, cursor, maxRows int) string {
	if len(rows) == 0 {
		return "  " + tuiDim.Render("(no data in this window)")
	}
	if maxRows < 1 {
		maxRows = 1
	}
	start := 0
	if cursor >= maxRows {
		start = cursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(rows) {
		end = len(rows)
	}
	var b strings.Builder
	for i := start; i < end; i++ {
		if i == cursor {
			b.WriteString(tuiSelRow.Render("▸ "+rows[i]) + "\n")
		} else {
			b.WriteString("  " + rows[i] + "\n")
		}
	}
	if len(rows) > maxRows {
		b.WriteString(tuiDim.Render(fmt.Sprintf("  %d–%d of %d", start+1, end, len(rows))))
	}
	return b.String()
}

func reverse(s []usage.NamedTotal) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func mustSince(s string) string {
	d, _ := usage.ParseSince(s, time.Now())
	return d
}

// runUsageTUI launches the interactive dashboard.
func runUsageTUI(cfg *config.Config, profiles []string, start string) error {
	m := newUsageTUI(cfg, profiles, start)
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}
