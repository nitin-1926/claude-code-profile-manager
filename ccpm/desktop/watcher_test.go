//go:build darwin

package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestShouldWatch(t *testing.T) {
	root := "/h/.ccpm"
	roots := []string{root, "/h/.claude"}
	cases := map[string]bool{
		root:                                 true,
		root + "/profiles":                   true,
		root + "/profiles/work":              true,
		root + "/profiles/work/skills":       true,
		root + "/profiles/work/skills/x":     false, // past the depth cap
		root + "/profiles/work/projects":     false, // transcripts
		root + "/profiles/work/node_modules": false,
		root + "/share/.git":                 false,
		"/h/.claude/projects":                false,
		"/h/.claude/projects/-Users-x":       false,
		"/elsewhere":                         false,
		"/h/.ccpm-other":                     false,
	}
	for p, want := range cases {
		if got := shouldWatch(roots, p); got != want {
			t.Errorf("shouldWatch(%q) = %v, want %v", p, got, want)
		}
	}
}

// A profile created while the app runs is added to the watch lazily. Those
// lazy adds used to skip the skip-list and depth cap, so the new profile's
// projects/ was watched and every transcript append fired ccpm:changed. Driven
// through the real watchLoop.
func TestLazilyAddedDirsHonourTheSkipList(t *testing.T) {
	root := t.TempDir()
	profiles := filepath.Join(root, "profiles")
	if err := os.Mkdir(profiles, 0o700); err != nil {
		t.Fatal(err)
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	a := &App{watcher: w, watchRoots: []string{root}}
	addTree(w, a.watchRoots, root)
	go a.watchLoop()
	t.Cleanup(func() { _ = w.Close() })

	waitWatched := func(p string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !slices.Contains(w.WatchList(), p) {
			if time.Now().After(deadline) {
				t.Fatalf("%s never watched; watching %v", p, w.WatchList())
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	mkdir := func(p string) {
		t.Helper()
		if err := os.Mkdir(p, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	fresh := filepath.Join(profiles, "fresh")
	mkdir(fresh)
	waitWatched(fresh)
	mkdir(filepath.Join(fresh, "projects"))
	// Events arrive in order: once skills is watched, projects was handled.
	mkdir(filepath.Join(fresh, "skills"))
	waitWatched(filepath.Join(fresh, "skills"))

	if slices.Contains(w.WatchList(), filepath.Join(fresh, "projects")) {
		t.Errorf("lazily added projects/ is watched: %v", w.WatchList())
	}
}
