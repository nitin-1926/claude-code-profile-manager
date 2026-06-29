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

