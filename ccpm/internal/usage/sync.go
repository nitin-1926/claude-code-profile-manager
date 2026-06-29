package usage

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"

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

		walkErr := WalkTranscripts(profileDir, "", func(abs, rel string) error {
			ns, ferr := ingestFile(abs, st.Files[rel], sess, day)
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

	// Seed dedup with the last message id counted from this file so a request
	// whose duplicate lines straddle the offset boundary isn't recounted.
	seen := map[string]bool{}
	lastMsgID := prev.LastMsgID
	if start == 0 {
		lastMsgID = "" // re-reading from the top; the stale cursor id is gone
	} else if lastMsgID != "" {
		seen[lastMsgID] = true
	}

	reader := bufio.NewReaderSize(f, 1024*1024)
	consumed := start
	for {
		lineBytes, rerr := reader.ReadBytes('\n')
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				// Trailing bytes without '\n' = an incomplete line still being
				// written; stop before it and pick it up once it's complete.
				break
			}
			return prev, rerr
		}
		var l transcriptLine
		if jerr := json.Unmarshal(lineBytes, &l); jerr == nil {
			if foldLine(l, sess, day, seen) {
				if k := l.dedupKey(); k != "" {
					lastMsgID = k
				}
			}
		}
		// A complete line (even malformed JSON) is permanently consumed.
		consumed += int64(len(lineBytes))
	}

	return FileState{
		Offset:    consumed,
		Size:      info.Size(),
		ModTime:   info.ModTime().Unix(),
		LastMsgID: lastMsgID,
	}, nil
}
