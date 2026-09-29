package cmd

import "testing"

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
