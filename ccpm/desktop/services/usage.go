//go:build darwin

package services

import (
	"path/filepath"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

type UsageTokens struct {
	Input         int64 `json:"input"`
	Output        int64 `json:"output"`
	CacheCreation int64 `json:"cacheCreation"`
	CacheRead     int64 `json:"cacheRead"`
	Total         int64 `json:"total"`
}

type UsageDay struct {
	Date     string  `json:"date"`
	Total    int64   `json:"total"`
	Messages int64   `json:"messages"`
	Cost     float64 `json:"cost"`
}

type UsageNamed struct {
	Name  string  `json:"name"`
	Total int64   `json:"total"`
	Cost  float64 `json:"cost"`
}

type UsageSession struct {
	ID       string `json:"id"`
	Cwd      string `json:"cwd"`
	Branch   string `json:"branch"`
	LastTS   string `json:"lastTs"`
	Total    int64  `json:"total"`
	Messages int64  `json:"messages"`
}

type Usage struct {
	Profile         string         `json:"profile"`
	Window          string         `json:"window"`
	TrackingEnabled bool           `json:"trackingEnabled"`
	Totals          UsageTokens    `json:"totals"`
	Messages        int64          `json:"messages"`
	Cost            float64        `json:"cost"`
	ByDay           []UsageDay     `json:"byDay"`
	ByModel         []UsageNamed   `json:"byModel"`
	ByProject       []UsageNamed   `json:"byProject"`
	Sessions        []UsageSession `json:"sessions"`
}

// UsageService renders per-profile token usage from the same store the CLI uses.
type UsageService struct{}

func NewUsage() *UsageService { return &UsageService{} }

// Blocks returns the profile's 5-hour usage blocks (newest last; the last may be
// the active block with burn-rate + projection). Empty slice on any error.
func (s *UsageService) Blocks(profile string) ([]usage.Block, error) {
	cfg, err := config.Load()
	if err != nil {
		return []usage.Block{}, nil
	}
	pc, ok := cfg.Profiles[profile]
	if !ok {
		return []usage.Block{}, nil
	}
	blocks, err := usage.LoadBlocks(pc.Dir, time.Now())
	if err != nil || blocks == nil {
		return []usage.Block{}, nil
	}
	return blocks, nil
}

func emptyUsage(profile, window string, tracking bool) *Usage {
	return &Usage{
		Profile: profile, Window: window, TrackingEnabled: tracking,
		ByDay: []UsageDay{}, ByModel: []UsageNamed{}, ByProject: []UsageNamed{}, Sessions: []UsageSession{},
	}
}

// Get returns usage for a profile within a window ("all"|"7d"|"30d"|"90d").
func (s *UsageService) Get(profile, window string) (*Usage, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	pc, ok := cfg.Profiles[profile]
	if !ok {
		return emptyUsage(profile, window, false), nil
	}

	// Sync (incremental ingest) then fall back to a plain load, mirroring cmd/usage.go.
	sess, daily, err := usage.Sync(pc.Dir)
	if err != nil {
		sess, daily, err = usage.Load(pc.Dir)
		if err != nil {
			return emptyUsage(profile, window, cfg.Settings.UsageTrackingEnabled()), nil
		}
	}

	since := ""
	if window != "" && window != "all" {
		if d, perr := usage.ParseSince(window, time.Now()); perr == nil {
			since = d
		}
	}
	view := usage.BuildView(sess, daily, since)

	out := &Usage{
		Profile:         profile,
		Window:          window,
		TrackingEnabled: cfg.Settings.UsageTrackingEnabled(),
		ByDay:           []UsageDay{},
		ByModel:         []UsageNamed{},
		ByProject:       []UsageNamed{},
		Sessions:        []UsageSession{},
		Totals: UsageTokens{
			Input:         view.Totals.Input,
			Output:        view.Totals.Output,
			CacheCreation: view.Totals.CacheCreation,
			CacheRead:     view.Totals.CacheRead,
			Total:         view.Totals.Total(),
		},
		Messages: view.Messages,
		Cost:     view.Cost,
	}
	for _, d := range view.ByDay {
		out.ByDay = append(out.ByDay, UsageDay{Date: d.Date, Total: d.Tokens.Total(), Messages: d.Messages, Cost: d.Cost})
	}
	for _, m := range view.ByModel {
		out.ByModel = append(out.ByModel, UsageNamed{Name: m.Name, Total: m.Tokens.Total(), Cost: m.Cost})
	}
	for _, p := range view.ByProject {
		out.ByProject = append(out.ByProject, UsageNamed{Name: filepath.Base(p.Name), Total: p.Tokens.Total(), Cost: p.Cost})
	}
	for _, r := range view.Sessions {
		out.Sessions = append(out.Sessions, UsageSession{
			ID:       r.SessionID,
			Cwd:      filepath.Base(r.Cwd),
			Branch:   r.GitBranch,
			LastTS:   r.LastTS,
			Total:    r.Tokens.Total(),
			Messages: r.Messages,
		})
	}
	return out, nil
}
