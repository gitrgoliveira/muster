package tmux

import (
	"errors"
	"io"
	"time"
)

// ErrAttachUnavailable is returned by Attach/Send/Capture when the fallback
// transport is in use (tmux absent) or when the session is not active.
var ErrAttachUnavailable = errors.New("tmux attach unavailable")

// Session describes a running (or recently dead) tmux session owned by muster.
type Session struct {
	Name      string // "muster/<bead>/<step>/<loop>"
	BeadID    string
	StepIdx   int
	Loop      int
	Pane      string // tmux pane id (e.g. "%3")
	StartedAt time.Time
}

// Manager is the interface the orchestrator uses to interact with tmux.
// The real implementation shells out to the `tmux` binary; a fallback
// implementation uses exec.Command directly when tmux is absent.
type Manager interface {
	// Detect probes for tmux and returns its version string (output of `tmux -V`).
	// Returns an error if tmux is not available. Implementations do NOT enforce
	// a minimum-version check today; callers that need one should parse the
	// returned string. (Plan §5 notes tmux ≥3.2 is the supported floor, but
	// M2 only requires the binary be present.)
	Detect() (version string, err error)

	// Spawn creates a new tmux session on the default socket with remain-on-exit on.
	// name follows the "muster/<bead>/<step>/<loop>" convention.
	// argv is the command to run inside the pane (e.g. ["sh", "-c", "claude … < prompt"]).
	Spawn(name, cwd string, env, argv []string) (*Session, error)

	// Pipe connects a pipe-pane to the named session and returns a reader that
	// receives raw terminal bytes (ANSI preserved — plan D1).
	Pipe(name string) (io.ReadCloser, error)

	// Capture returns the current pane scrollback via capture-pane -p -S -.
	// withEscapes=true adds -e to preserve ANSI codes for faithful catch-up rendering.
	Capture(name string, withEscapes bool) (string, error)

	// Send delivers keystrokes to the named session via send-keys.
	Send(name, keys string) error

	// Attach returns the shell command string the user runs to attach to the session,
	// e.g. "tmux attach -t muster/mp-abc/0/0".
	Attach(name string) (cmd string, err error)

	// DeadStatus queries whether the pane is dead and its exit code.
	// dead=true means the process has exited; code is the exit code.
	DeadStatus(name string) (code int, dead bool, err error)

	// Kill terminates the named session (kill-session).
	Kill(name string) error

	// List returns all muster-owned sessions (prefix "muster/") from the default socket.
	List() ([]Session, error)
}
