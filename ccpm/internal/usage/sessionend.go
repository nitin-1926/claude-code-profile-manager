package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/atomicwrite"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

// A session ending leaves no unambiguous trace otherwise: limits.json is
// rewritten by every status line render, and the store files by any Sync, which
// `ccpm usage` and the desktop app run too. So the SessionEnd hook drops this
// marker, and only the hook can: it alone receives the payload parsed below.

// SessionEnd is the last session that finished in a profile.
type SessionEnd struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	EndedAt        int64  `json:"ended_at"`
}

func sessionEndPath(profileDir string) string {
	return filepath.Join(Dir(profileDir), "session-end.json")
}

// MarkSessionEnd records the session a SessionEnd hook payload describes.
// Anything else on stdin (a manual `ccpm usage sync`) records nothing.
func MarkSessionEnd(profileDir string, hookJSON []byte, now time.Time) error {
	var in struct {
		HookEventName  string `json:"hook_event_name"`
		SessionID      string `json:"session_id"`
		TranscriptPath string `json:"transcript_path"`
		Reason         string `json:"reason"`
	}
	if json.Unmarshal(hookJSON, &in) != nil || in.HookEventName != "SessionEnd" || in.SessionID == "" {
		return nil
	}
	// /clear and /resume retire a session id, but the person is still sitting in
	// the same running Claude Code: nothing has finished from where they are.
	if in.Reason == "clear" || in.Reason == "resume" {
		return nil
	}
	if err := os.MkdirAll(Dir(profileDir), config.DirPerm); err != nil {
		return err
	}
	data, err := json.Marshal(SessionEnd{SessionID: in.SessionID, TranscriptPath: in.TranscriptPath, EndedAt: now.Unix()})
	if err != nil {
		return err
	}
	return atomicwrite.Apply([]atomicwrite.FileChange{
		atomicwrite.WriteFile(sessionEndPath(profileDir), data, config.FilePerm),
	})
}

// LoadSessionEnd reads the marker; the zero value when no session has ended yet.
func LoadSessionEnd(profileDir string) SessionEnd {
	var e SessionEnd
	data, err := os.ReadFile(sessionEndPath(profileDir))
	if err != nil || json.Unmarshal(data, &e) != nil {
		return SessionEnd{}
	}
	return e
}
