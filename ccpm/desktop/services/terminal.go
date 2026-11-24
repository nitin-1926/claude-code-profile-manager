//go:build darwin

package services

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
	claudepkg "github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/claude"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/profile"
	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

// TerminalSession describes one embedded terminal.
type TerminalSession struct {
	ID      string `json:"id"`
	Profile string `json:"profile"`
	Cwd     string `json:"cwd"`
	Title   string `json:"title"`
	Running bool   `json:"running"`
	// ExitCode is meaningful once Running is false; -1 means a signal ended it.
	ExitCode int `json:"exitCode"`
}

// TerminalAttach is what a (re)mounting terminal view starts from.
type TerminalAttach struct {
	Session TerminalSession `json:"session"`
	// Data is the buffered scrollback, base64 because a PTY read can end in the
	// middle of a UTF-8 sequence and a JSON string cannot carry half a rune.
	Data string `json:"data"`
	// Seq is the total byte count Data ends at. Live chunks carry the same
	// counter, so the view drops any chunk Data already contains instead of
	// printing it twice.
	Seq int64 `json:"seq"`
}

// terminalChunk is the payload of the terminal:data:<id> event.
type terminalChunk struct {
	Data string `json:"data"`
	Seq  int64  `json:"seq"`
}

const (
	// terminalScrollback bounds what a session keeps for a remounted view.
	terminalScrollback = 512 << 10
	// terminalGrace is how long Close waits after SIGHUP before SIGKILL.
	terminalGrace = time.Second
	// terminalDrain is how long an exited session waits for its last output
	// before closing the PTY under a grandchild that still holds it open.
	terminalDrain = 500 * time.Millisecond

	errNoTerminal = "no such terminal session"
)

// TerminalService owns the embedded terminals. It lives in Go, not React,
// because ProfileView remounts every tab on a profile switch: a PTY owned by a
// component would die with it. A view attaches, replays the scrollback, and
// follows the live events; unmounting the view leaves the session running.
type TerminalService struct {
	// Ctx is the Wails runtime context, set once from the App's startup.
	Ctx context.Context

	// command builds the process for `ccpm run <profile> <args...>`, and emit
	// sends a frontend event. Fields so tests run /bin/sh and capture events
	// instead of needing ccpm and a webview.
	command func(profile string, args []string) (*exec.Cmd, error)
	emit    func(event string, payload any)
	// scrollback is the per-session buffer bound; a field so the bound test
	// does not have to push half a megabyte through a PTY.
	scrollback int
	// quiet silences events once the app is shutting down.
	quiet atomic.Bool

	mu       sync.Mutex
	sessions map[string]*termSession
	order    []string
	next     int
}

type termSession struct {
	id   string
	cmd  *exec.Cmd
	ptmx *os.File
	// readDone closes when the reader stops; done closes once the child is
	// reaped, the PTY is closed, and info records the exit.
	readDone chan struct{}
	done     chan struct{}

	mu   sync.Mutex
	info TerminalSession
	buf  []byte
	seq  int64
}

func NewTerminal() *TerminalService {
	t := &TerminalService{
		sessions:   map[string]*termSession{},
		command:    ccpmRunCommand,
		scrollback: terminalScrollback,
	}
	t.emit = func(event string, payload any) {
		if t.Ctx != nil && !t.quiet.Load() {
			wr.EventsEmit(t.Ctx, event, payload)
		}
	}
	return t
}

// Start runs `ccpm run <profile> <args...>` in a new PTY in cwd (the home
// directory when empty).
//
// No shell is involved: args reach ccpm as argv, so the control-character
// check below is defence in depth rather than the quoting guard it is for the
// Terminal.app path. It stays so both launch paths refuse the same input.
func (t *TerminalService) Start(name, cwd string, args []string) (TerminalSession, error) {
	if !terminalArgsOK(cwd, append([]string{name}, args...)) {
		return TerminalSession{}, errors.New(errControlChar)
	}
	if err := profile.ValidateName(name); err != nil {
		return TerminalSession{}, err
	}
	if profileDir(name) == "" {
		return TerminalSession{}, errors.New("unknown profile: " + name)
	}
	if cwd == "" {
		cwd, _ = os.UserHomeDir()
	}
	if fi, err := os.Stat(cwd); err != nil || !fi.IsDir() {
		return TerminalSession{}, errors.New("directory does not exist: " + cwd)
	}

	cmd, err := t.command(name, args)
	if err != nil {
		return TerminalSession{}, err
	}
	cmd.Dir = cwd
	// PWD too: the inherited one is the app's, and shells trust it over getcwd.
	cmd.Env = append(terminalEnv(cmd.Env), "PWD="+cwd)
	// StartWithSize sets Setsid+Setctty, so the child leads its own session and
	// process group (pgid == pid): Close can signal claude and everything it
	// spawned in one call. The view sends its real size right after attaching.
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 80, Rows: 24})
	if err != nil {
		return TerminalSession{}, fmt.Errorf("starting terminal: %w", err)
	}

	t.mu.Lock()
	t.next++
	s := &termSession{
		id:       "t" + strconv.Itoa(t.next),
		cmd:      cmd,
		ptmx:     ptmx,
		readDone: make(chan struct{}),
		done:     make(chan struct{}),
	}
	s.info = TerminalSession{ID: s.id, Profile: name, Cwd: cwd, Title: terminalTitle(args), Running: true}
	t.sessions[s.id] = s
	t.order = append(t.order, s.id)
	t.mu.Unlock()

	go t.read(s)
	go t.wait(s)
	t.emit("terminal:changed", nil)
	return s.snapshot(), nil
}

// read copies PTY output into the scrollback and out to the frontend.
func (t *TerminalService) read(s *termSession) {
	defer close(s.readDone)
	buf := make([]byte, 32<<10)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			s.mu.Lock()
			s.buf = append(s.buf, buf[:n]...)
			if over := len(s.buf) - t.scrollback; over > 0 {
				// ponytail: reslice + append realloc keeps memory under ~2x the
				// bound; a true ring only matters if that copy ever shows up.
				s.buf = s.buf[over:]
			}
			s.seq += int64(n)
			seq := s.seq
			s.mu.Unlock()
			// Encoded here, before buf is reused by the next Read.
			t.emit("terminal:data:"+s.id, terminalChunk{Data: base64.StdEncoding.EncodeToString(buf[:n]), Seq: seq})
		}
		if err != nil {
			return
		}
	}
}

// wait reaps the child so it never lingers as a zombie, then records the exit.
func (t *TerminalService) wait(s *termSession) {
	err := s.cmd.Wait()
	code := 0
	if err != nil {
		code = -1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		}
	}
	// Let the reader drain the child's last words. A grandchild still holding
	// the tty keeps Read blocked; closing the master unblocks it.
	select {
	case <-s.readDone:
	case <-time.After(terminalDrain):
	}
	_ = s.ptmx.Close()
	<-s.readDone

	s.mu.Lock()
	s.info.Running = false
	s.info.ExitCode = code
	s.mu.Unlock()
	// After every data event (the reader has stopped), so a view prints the
	// exit line below the final output; and before done, so CloseAll's quiet
	// window covers these too.
	t.emit("terminal:exit:"+s.id, code)
	t.emit("terminal:changed", nil)
	close(s.done)
}

func (s *termSession) snapshot() TerminalSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.info
}

func (t *TerminalService) get(id string) *termSession {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sessions[id]
}

// List returns every session, oldest first, exited ones included until closed.
func (t *TerminalService) List() []TerminalSession {
	t.mu.Lock()
	ss := make([]*termSession, 0, len(t.order))
	for _, id := range t.order {
		ss = append(ss, t.sessions[id])
	}
	t.mu.Unlock()
	out := make([]TerminalSession, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.snapshot())
	}
	return out
}

// Attach returns the session and its buffered scrollback.
func (t *TerminalService) Attach(id string) (TerminalAttach, error) {
	s := t.get(id)
	if s == nil {
		return TerminalAttach{}, errors.New(errNoTerminal)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return TerminalAttach{Session: s.info, Data: base64.StdEncoding.EncodeToString(s.buf), Seq: s.seq}, nil
}

// Write sends keyboard input. Input to an exited session is dropped.
func (t *TerminalService) Write(id, data string) error {
	s := t.get(id)
	if s == nil {
		return errors.New(errNoTerminal)
	}
	if !s.snapshot().Running {
		return nil
	}
	_, err := s.ptmx.Write([]byte(data))
	if errors.Is(err, os.ErrClosed) {
		return nil // exited between the check and the write
	}
	return err
}

// Resize sets the PTY window size; the kernel sends the child SIGWINCH.
func (t *TerminalService) Resize(id string, cols, rows int) error {
	if cols < 1 || rows < 1 || cols > 4096 || rows > 4096 {
		return fmt.Errorf("invalid terminal size %dx%d", cols, rows)
	}
	s := t.get(id)
	if s == nil {
		return errors.New(errNoTerminal)
	}
	if !s.snapshot().Running {
		return nil
	}
	err := pty.Setsize(s.ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if errors.Is(err, os.ErrClosed) {
		return nil
	}
	return err
}

// Close ends a session and forgets it. Unknown ids are a no-op, so a view can
// close without first checking the session is still there.
func (t *TerminalService) Close(id string) {
	t.mu.Lock()
	s := t.sessions[id]
	delete(t.sessions, id)
	t.order = slices.DeleteFunc(t.order, func(x string) bool { return x == id })
	t.mu.Unlock()
	if s == nil {
		return
	}
	s.kill()
	t.emit("terminal:changed", nil)
}

// CloseAll ends every session; the app calls it on shutdown so quitting leaves
// no process behind. Sessions close in parallel so the wait is one grace
// period, not one per session.
func (t *TerminalService) CloseAll() {
	t.quiet.Store(true)
	t.mu.Lock()
	ids := slices.Clone(t.order)
	t.mu.Unlock()
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Go(func() { t.Close(id) })
	}
	wg.Wait()
	t.quiet.Store(false)
}

// kill hangs up the session's process group the way closing a terminal does,
// then SIGKILLs whatever ignored that, and returns once the child is reaped.
func (s *termSession) kill() {
	select {
	case <-s.done:
		// Already reaped: the pgid may since have been reused, so never
		// signal it.
		return
	default:
	}
	pgid := s.cmd.Process.Pid
	_ = syscall.Kill(-pgid, syscall.SIGHUP)
	select {
	case <-s.done:
	case <-time.After(terminalGrace):
	}
	// Unconditionally: the leader can exit on SIGHUP while a grandchild that
	// ignores it lives on in the group.
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	<-s.done
}

// terminalTitle labels a session for its tab.
func terminalTitle(args []string) string {
	if i := slices.Index(args, "--resume"); i >= 0 && i+1 < len(args) {
		id := args[i+1]
		if len(id) > 8 {
			id = id[:8]
		}
		return "resume " + id
	}
	return "claude"
}

// claudeSessionMarkers are what a running Claude Code puts in its children's
// environment. An app started from a Claude Code terminal inherits them, and
// passing them on makes every embedded claude a "child" of that session: it
// turns transcript saving off and is handed the parent's messaging socket and
// token. Named one by one, not CLAUDE_CODE_*: users configure claude through
// that prefix too (CLAUDE_CODE_USE_BEDROCK). CLAUDE_CONFIG_DIR is replaced by
// `ccpm run` anyway; dropped so the child never sees another profile's.
var claudeSessionMarkers = []string{
	"CLAUDECODE",
	"CLAUDE_PID",
	"CLAUDE_EFFORT",
	"CLAUDE_CONFIG_DIR",
	"CLAUDE_CODE_BRIDGE_SESSION_ID",
	"CLAUDE_CODE_CHILD_SESSION",
	"CLAUDE_CODE_ENTRYPOINT",
	"CLAUDE_CODE_EXECPATH",
	"CLAUDE_CODE_MESSAGING_SOCKET",
	"CLAUDE_CODE_MESSAGING_TOKEN",
	"CLAUDE_CODE_SESSION_ATTENDED",
	"CLAUDE_CODE_SESSION_ID",
}

// terminalEnv is env (the process environment when nil) without any inherited
// Claude Code session markers, plus what a terminal needs and a
// Finder-launched app lacks.
func terminalEnv(env []string) []string {
	if env == nil {
		env = os.Environ()
	}
	env = slices.DeleteFunc(slices.Clone(env), func(kv string) bool {
		k, _, _ := strings.Cut(kv, "=")
		return slices.Contains(claudeSessionMarkers, k)
	})
	env = append(env, "TERM=xterm-256color", "COLORTERM=truecolor")
	// A GUI app gets no locale, and without one claude's TUI falls back to
	// ASCII or prints mojibake. Terminal.app sets this too.
	if envValue(env, "LANG") == "" && envValue(env, "LC_ALL") == "" && envValue(env, "LC_CTYPE") == "" {
		env = append(env, "LANG=en_US.UTF-8")
	}
	return env
}

// envValue returns the last value of key in env, as exec resolves duplicates.
func envValue(env []string, key string) string {
	v := ""
	for _, kv := range env {
		if k, val, ok := strings.Cut(kv, "="); ok && k == key {
			v = val
		}
	}
	return v
}

// ccpmRunCommand is the real command builder: the ccpm that findCCPM resolves,
// with a PATH that can also find claude and node.
func ccpmRunCommand(name string, args []string) (*exec.Cmd, error) {
	bin := findCCPM()
	if bin == "" {
		return nil, errors.New("ccpm CLI not found on PATH")
	}
	cmd := exec.Command(bin, append([]string{"run", name}, args...)...)
	cmd.Env = append(os.Environ(), "PATH="+terminalPATH(bin))
	return cmd, nil
}

// terminalPATH extends the inherited PATH for a GUI app, whose PATH from
// launchd is only /usr/bin:/bin:/usr/sbin:/sbin. ccpm finds claude without
// PATH, but an npm-installed claude is a `#!/usr/bin/env node` script, and its
// hooks and MCP servers need the usual tools; so add claude's own directory
// (where nvm also keeps node) and the install locations findCCPM probes.
//
// ponytail: fixed directories, not the login shell's PATH. Tools only an rc
// file puts on PATH (pyenv, cargo) are missing; resolve `$SHELL -ilc` if that
// bites.
func terminalPATH(ccpmBin string) string {
	home, _ := os.UserHomeDir()
	extra := []string{filepath.Dir(ccpmBin)}
	if c, err := claudepkg.FindBinary(); err == nil {
		extra = append(extra, filepath.Dir(c))
	}
	extra = append(extra,
		"/opt/homebrew/bin",
		"/usr/local/bin",
		filepath.Join(home, ".npm-global", "bin"),
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, "go", "bin"),
		"/usr/bin", "/bin", "/usr/sbin", "/sbin",
	)
	return mergePath(os.Getenv("PATH"), extra...)
}

// mergePath appends each of extra to base that it does not already hold. The
// inherited entries keep their precedence.
func mergePath(base string, extra ...string) string {
	parts := filepath.SplitList(base)
	for _, d := range extra {
		if d != "" && d != "." && !slices.Contains(parts, d) {
			parts = append(parts, d)
		}
	}
	return strings.Join(parts, string(os.PathListSeparator))
}
