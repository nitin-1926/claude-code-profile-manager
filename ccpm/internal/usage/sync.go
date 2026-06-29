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

