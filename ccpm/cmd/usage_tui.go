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

