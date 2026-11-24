//go:build darwin

package rail

import (
	"fmt"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/desktop/services"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

// MenuRow is one profile's line in the menu bar menu.
type MenuRow struct {
	// Profile is what a click runs. It travels apart from Title so the click
	// never has to parse a name back out of display text.
	Profile string `json:"profile"`
	Title   string `json:"title"`
}

// menuWindows fixes the order and the short labels, whatever order the reading
// listed its windows in.
var menuWindows = []struct{ key, label string }{
	{usage.KeyFiveHour, "5h"},
	{usage.KeySevenDay, "7d"},
}

// BuildMenuRows turns the limits the notch already holds into menu rows, one per
// profile, in the caller's order. The result is never nil, so it encodes as [].
//
// Percentages read the same as the notch's: a window past its reset is 0%, and
// a figure over 100 is printed as reported. A profile with no reading gets its
// bare name, because no number is the honest answer to "we do not know".
func BuildMenuRows(limits []services.ProfileLimits, now time.Time) []MenuRow {
	rows := make([]MenuRow, 0, len(limits))
	for _, l := range limits {
		title := l.Profile
		if l.Available {
			for _, mw := range menuWindows {
				for _, w := range l.Windows {
					if w.Key == mw.key {
						title += fmt.Sprintf("  %s %d%%", mw.label, int(usedNow(w, now)+0.5))
					}
				}
			}
		}
		rows = append(rows, MenuRow{Profile: l.Profile, Title: title})
	}
	return rows
}
