package usage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/atomicwrite"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/lock"
)

// Sync performs a lazy incremental catch-up for one profile: it reads only the
// bytes appended to each transcript since the last run, folds them into the
// session and daily indexes, and atomically persists all three store files. It
// returns the post-ingest indexes so a caller can render without re-reading.
//
// Concurrency is serialised per profile by an advisory lock, so a hook-driven
// sync and an interactive `ccpm usage` can run at once safely. On lock
// contention or any error it returns that error; callers that must never fail
// (the SessionEnd hook) should treat an error as a silent no-op.
func Sync(profileDir string) (*Sessions, *Daily, error) {
	// The lock file lives in the store dir, so it must exist before we lock.
	if err := os.MkdirAll(Dir(profileDir), config.DirPerm); err != nil {
		return nil, nil, err
	}

	var outSess *Sessions
	var outDaily *Daily
	err := lock.Guard(lockPath(profileDir), lock.DefaultTimeout, func() error {
		st, err := loadState(profileDir)
		if err != nil {
			return err
		}
		sess, err := loadSessions(profileDir)
		if err != nil {
			return err
		}
		day, err := loadDaily(profileDir)
		if err != nil {
			return err
		}
		// A store written by an older schema is discarded and rebuilt from the
		// transcripts (reset cursors → full re-ingest; dedup keeps it correct).
		if st.Version != storeVersion {
			st, sess, day = newState(), newSessions(), newDaily()
		}

		newFiles := make(map[string]FileState, len(st.Files))
		for k, v := range st.Files {
			newFiles[k] = v
		}

		ported := loadPorted(profileDir)
		visited := make(map[string]bool, len(st.Files))
		walkErr := WalkTranscripts(profileDir, func(abs, rel string) error {
			visited[rel] = true
			prev := st.Files[rel]
			if base := ported[rel]; prev.Offset < base {
				prev = FileState{Offset: base}
			}
			ns, ferr := ingestFile(abs, prev, sess, day)
			if ferr != nil {
				// Unreadable right now (e.g. native claude mid-write); leave the
				// cursor untouched and retry on the next sync.
				return nil
			}
			newFiles[rel] = ns
			return nil
		})
		if walkErr != nil {
			return walkErr
		}
		// Drop cursors for transcripts Claude Code has pruned, or state.json
		// grows with dead entries forever and is rewritten whole every sync.
		// Only a confirmed not-exist counts: a file the walk could not reach
		// this time (an unreadable dir) keeps its cursor, since losing it
		// would re-count the whole file once it is readable again.
		projects := filepath.Join(profileDir, "projects")
		for rel := range newFiles {
			if visited[rel] {
				continue
			}
			if _, err := os.Lstat(filepath.Join(projects, rel)); errors.Is(err, fs.ErrNotExist) {
				delete(newFiles, rel)
			}
		}
		st.Files = newFiles

		if err := commit(profileDir, st, sess, day); err != nil {
			return err
		}
		outSess, outDaily = sess, day
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return outSess, outDaily, nil
}

// ingestFile folds the new (post-offset) complete lines of one transcript into
// the indexes and returns the updated cursor. It never advances past an
// incomplete trailing line (still being written), and resets to the top if the
// file shrank below the stored offset (rotation/truncation — rare, since
// Claude's transcripts are append-only UUID files; dedup by message.id keeps a
// re-read from double-counting within the same pass).
func ingestFile(path string, prev FileState, sess *Sessions, day *Daily) (FileState, error) {
	info, err := os.Stat(path)
	if err != nil {
		return prev, err
	}
	start := prev.Offset
	if info.Size() < prev.Offset {
		start = 0
	}
	// Unchanged and fully consumed: nothing to do, keep the cursor.
	if start == info.Size() && info.Size() == prev.Size && info.ModTime().Unix() == prev.ModTime {
		return prev, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return prev, err
	}
	defer f.Close()
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return prev, err
	}

	// Seed dedup with the tail of keys already counted from this file, WITH the
	// tokens attributed to each, so a request whose duplicate lines straddle the
	// offset boundary is neither recounted (its duplicates compare as not-larger)
	// nor frozen (a genuinely larger final snapshot still revises it by the
	// delta). Seeding only the single last key — or seeding it with a max
	// sentinel — got both of those wrong.
	counted := map[string]Tokens{}
	order := make([]string, 0, len(prev.Recent)+16)
	inOrder := make(map[string]bool, len(prev.Recent))
	if start > 0 {
		for _, ck := range prev.Recent {
			counted[ck.Key] = ck.Tokens
			if !inOrder[ck.Key] {
				inOrder[ck.Key] = true
				order = append(order, ck.Key)
			}
		}
	}

	// EachLine stops before a trailing line with no '\n' (still being written),
	// so the cursor never passes it and the next sync picks it up complete.
	consumed := start
	rerr := EachLine(f, func(lineBytes []byte, n int64) bool {
		var l transcriptLine
		if lineBytes != nil && json.Unmarshal(lineBytes, &l) == nil {
			if foldLine(l, sess, day, counted) {
				if k := l.dedupKey(); k != "" && !inOrder[k] {
					inOrder[k] = true
					order = append(order, k)
				}
			}
		}
		// A complete line (even malformed JSON, or skipped for exceeding
		// MaxLineBytes) is permanently consumed.
		consumed += n
		return true
	})
	if rerr != nil {
		return prev, rerr
	}

	// ponytail: bounded to the last recentWindow keys; persisting every key ever
	// seen would grow state.json without limit. A response's duplicate lines are
	// USUALLY near each other, but not always — a duplicate has been observed
	// more than 128 keys after its first line. If a sync boundary falls between
	// such a pair, the late duplicate finds no seeded key and is counted a
	// second time. Raise the window if that ever shows up in totals.
	if len(order) > recentWindow {
		order = order[len(order)-recentWindow:]
	}
	recent := make([]CountedKey, 0, len(order))
	for _, k := range order {
		recent = append(recent, CountedKey{Key: k, Tokens: counted[k]})
	}

	return FileState{
		Offset:  consumed,
		Size:    info.Size(),
		ModTime: info.ModTime().Unix(),
		Recent:  recent,
	}, nil
}

// recentWindow is how many trailing dedup keys are carried across a sync
// boundary per transcript.
const recentWindow = 128

// portedPath records, for each transcript copied in from another profile, the
// byte offset the copy ended at. Everything below that offset was spent on the
// source profile's account, so it never counts here.
//
// It is a sidecar rather than a seeded cursor in state.json because a
// storeVersion bump discards state.json and re-ingests every transcript from
// zero, which would hand this profile all of those tokens. LoadBlocks also reads
// transcripts directly without consulting cursors, and needs the same floor.
func portedPath(profileDir string) string { return filepath.Join(Dir(profileDir), "ported.json") }

// loadPorted reads the sidecar, keyed like State.Files. A missing or unreadable
// file yields an empty map.
func loadPorted(profileDir string) map[string]int64 {
	var m map[string]int64
	if _, err := loadJSON(portedPath(profileDir), &m); err != nil || m == nil {
		return map[string]int64{}
	}
	return m
}

// Adopt installs files copied from another profile under <profileDir>/projects,
// keyed by slash-separated projects-relative path, and records where each
// transcript ends so only lines appended afterwards count toward this profile.
//
// It holds the usage lock and writes the files and the sidecar in one
// transaction: a Sync running between the two would ingest the whole copy. A
// path that already exists is refused rather than overwritten.
//
// Measured against Claude Code 2.1.318: a resume only appends, and appends no
// message id already in the file, so the offset alone is a complete boundary and
// no dedup keys need seeding. A trailing line with no newline stays above the
// offset, matching how ingest treats a line still being written.
func Adopt(profileDir string, files map[string][]byte) error {
	if err := os.MkdirAll(Dir(profileDir), config.DirPerm); err != nil {
		return err
	}
	return lock.Guard(lockPath(profileDir), lock.DefaultTimeout, func() error {
		ported := loadPorted(profileDir)
		changes := make([]atomicwrite.FileChange, 0, len(files)+1)
		for slashRel, data := range files {
			rel := filepath.FromSlash(slashRel)
			full := filepath.Join(profileDir, "projects", rel)
			if _, err := os.Lstat(full); err == nil {
				return fmt.Errorf("%s already exists", slashRel)
			}
			if strings.HasSuffix(rel, ".jsonl") {
				ported[rel] = int64(bytes.LastIndexByte(data, '\n') + 1)
			}
			changes = append(changes, atomicwrite.WriteFile(full, data, config.FilePerm))
		}
		b, err := json.MarshalIndent(ported, "", "  ")
		if err != nil {
			return err
		}
		return atomicwrite.Apply(append(changes, atomicwrite.WriteFile(portedPath(profileDir), b, config.FilePerm)))
	})
}
