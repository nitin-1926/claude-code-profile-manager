package usage

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// NamedTotal is a sorted breakdown row (a model name or project path -> tokens).
type NamedTotal struct {
	Name   string
	Tokens Tokens
}

// DayTotal is one day's tally in the time series.
type DayTotal struct {
	Date     string
	Tokens   Tokens
	Messages int64
}

// View is the aggregated, render-ready usage for a profile over an optional
// since-window. Every time-based total is folded from the daily ledger so a
// --since filter applies uniformly; Sessions comes from the session index.
type View struct {
	Totals    Tokens
	Messages  int64
	ByModel   []NamedTotal     // desc by total
	ByProject []NamedTotal     // desc by total
	ByDay     []DayTotal       // asc by date
	Sessions  []*SessionRecord // desc by LastTS
}

// BuildView folds the daily ledger (dates >= sinceDate when non-empty) and the
// session index into a render-ready View. sinceDate is "YYYY-MM-DD" or "" (all).
func BuildView(sess *Sessions, day *Daily, sinceDate string) View {
	var v View
	byModel := map[string]Tokens{}

	// Time-based facts (totals, by-model, time series) fold from the daily
	// ledger, so a --since window filters by day.
	dates := make([]string, 0, len(day.Days))
	for d := range day.Days {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	for _, d := range dates {
		if sinceDate != "" && d < sinceDate {
			continue
		}
		dr := day.Days[d]
		v.Totals.Add(dr.Tokens)
		v.Messages += dr.Messages
		v.ByDay = append(v.ByDay, DayTotal{Date: d, Tokens: dr.Tokens, Messages: dr.Messages})
		for m, tk := range dr.ByModel {
			x := byModel[m]
			x.Add(tk)
			byModel[m] = x
		}
	}
	v.ByModel = sortedTotals(byModel)

	// Sessions (and the by-project view derived from them) come from the session
	// index. cwd is intrinsic to a session, so by-project = group by cwd. With
	// --since a session is included whole when its last activity is in-window
	// (session-granular, not day-granular — a fair approximation since sessions
	// are short and single-project).
	byProject := map[string]Tokens{}
	for _, r := range sess.Records {
		if sinceDate != "" && dayBucket(r.LastTS) < sinceDate {
			continue
		}
		v.Sessions = append(v.Sessions, r)
		key := r.Cwd
		if key == "" {
			key = "unknown"
		}
		x := byProject[key]
		x.Add(r.Tokens)
		byProject[key] = x
	}
	v.ByProject = sortedTotals(byProject)
	sort.Slice(v.Sessions, func(i, j int) bool { return v.Sessions[i].LastTS > v.Sessions[j].LastTS })
	return v
}

func sortedTotals(m map[string]Tokens) []NamedTotal {
	out := make([]NamedTotal, 0, len(m))
	for k, t := range m {
		out = append(out, NamedTotal{Name: k, Tokens: t})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tokens.Total() != out[j].Tokens.Total() {
			return out[i].Tokens.Total() > out[j].Tokens.Total()
		}
		return out[i].Name < out[j].Name
	})
	return out
}

