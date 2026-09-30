package cmd

import (
	"encoding/json"
	"testing"
	"time"
)

// The listing as GitHub returned it on 2026-09-29, trimmed to the fields read:
// the desktop release was published last, so it heads the list.
func TestNewestCLITagSkipsDesktopReleases(t *testing.T) {
	body := []byte(`[
		{"tag_name":"desktop-v0.1.1","draft":false,"prerelease":false},
		{"tag_name":"v0.6.3","draft":false,"prerelease":false},
		{"tag_name":"v0.6.2","draft":false,"prerelease":false}
	]`)
	got, err := newestCLITag(body)
	if err != nil || got != "v0.6.3" {
		t.Fatalf("newestCLITag = %q, %v; want v0.6.3", got, err)
	}
}

func TestNewestCLITagSkipsPrereleases(t *testing.T) {
	body := []byte(`[
		{"tag_name":"v0.7.0-rc.1","draft":false,"prerelease":true},
		{"tag_name":"v0.6.3","draft":false,"prerelease":false}
	]`)
	if got, _ := newestCLITag(body); got != "v0.6.3" {
		t.Fatalf("newestCLITag = %q; want v0.6.3", got)
	}
}

func TestNewestCLITagWithNoCLIRelease(t *testing.T) {
	if got, err := newestCLITag([]byte(`[{"tag_name":"desktop-v0.1.1"}]`)); err == nil {
		t.Fatalf("newestCLITag = %q, nil; want an error", got)
	}
}

// A tag that merely starts with "v" is not a CLI release, and a prerelease tag
// published without GitHub's prerelease flag is still a prerelease.
func TestNewestCLITagWantsAStableSemverTag(t *testing.T) {
	body := []byte(`[
		{"tag_name":"vscode-extension-1","draft":false,"prerelease":false},
		{"tag_name":"v0.7.0-rc.1","draft":false,"prerelease":false},
		{"tag_name":"v0.6.3","draft":false,"prerelease":false}
	]`)
	if got, _ := newestCLITag(body); got != "v0.6.3" {
		t.Fatalf("newestCLITag = %q; want v0.6.3", got)
	}
}

// A cache written by a pre-0.7.0 binary can hold a desktop tag; it must be
// refetched, not trusted for the rest of its 24 hours.
func TestFreshCachedTagRejectsWhatANewerCheckWouldNotPick(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	entry := func(tag string, age time.Duration) []byte {
		b, _ := json.Marshal(latestVersionCache{Tag: tag, CheckedAt: now.Add(-age)})
		return b
	}
	cases := []struct {
		name string
		data []byte
		want string
		ok   bool
	}{
		{"fresh CLI tag", entry("v0.7.0", time.Hour), "v0.7.0", true},
		{"desktop tag from an older binary", entry("desktop-v0.1.1", time.Hour), "", false},
		{"prerelease tag", entry("v0.8.0-rc.1", time.Hour), "", false},
		{"expired", entry("v0.7.0", 25*time.Hour), "", false},
		{"corrupt", []byte("{"), "", false},
	}
	for _, tc := range cases {
		if got, ok := freshCachedTag(tc.data, now); got != tc.want || ok != tc.ok {
			t.Errorf("%s: freshCachedTag = %q, %v; want %q, %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}
