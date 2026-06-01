package tmux

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// FallbackManager implements Manager via direct exec.Command when tmux is absent.
// Attach/Send/Capture return ErrAttachUnavailable; exit code comes from cmd.Wait().
type FallbackManager struct {
	mu       sync.Mutex
	sessions map[string]*fallbackSession
}

type fallbackSession struct {
	name      string
	cmd       *exec.Cmd
	done      chan struct{}
	exitCode  int
	startedAt time.Time
	pipeR     io.ReadCloser
}

// NewFallbackManager creates a new FallbackManager.
func NewFallbackManager() *FallbackManager {
	return &FallbackManager{sessions: make(map[string]*fallbackSession)}
}

// Detect always returns an error (tmux absent) when the FallbackManager is used.
func (f *FallbackManager) Detect() (string, error) {
	return "", fmt.Errorf("tmux not available (fallback mode)")
}

// Spawn starts the argv as a direct child process and stores it in the session map.
func (f *FallbackManager) Spawn(name, cwd string, env, argv []string) (*Session, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("fallback Spawn: argv must not be empty")
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = cwd
	if len(env) > 0 {
		// Append to existing env.
		cmd.Env = append(cmd.Environ(), env...)
	}

	// Set up stdout/stderr pipe for streaming.
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		_ = pr.Close()
		_ = pw.Close()
		return nil, fmt.Errorf("fallback spawn %q: %w", name, err)
	}

	sess := &fallbackSession{
		name:      name,
		cmd:       cmd,
		done:      make(chan struct{}),
		startedAt: time.Now(),
		pipeR:     pr,
	}

	// Wait for cmd exit in a goroutine.
	go func() {
		defer close(sess.done)
		defer pw.Close() //nolint:errcheck
		if err := cmd.Wait(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				sess.exitCode = exitErr.ExitCode()
			} else {
				sess.exitCode = 1
			}
		}
	}()

	f.mu.Lock()
	f.sessions[name] = sess
	f.mu.Unlock()

	beadID, stepIdx, loop, _ := ParseSessionName(name)
	return &Session{
		Name:      name,
		BeadID:    beadID,
		StepIdx:   stepIdx,
		Loop:      loop,
		StartedAt: sess.startedAt,
	}, nil
}

// Pipe returns the stdout/stderr reader for the process.
func (f *FallbackManager) Pipe(name string) (io.ReadCloser, error) {
	f.mu.Lock()
	sess, ok := f.sessions[name]
	f.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("fallback Pipe: session %q not found", name)
	}
	return sess.pipeR, nil
}

// Capture returns ErrAttachUnavailable — no scrollback in fallback mode.
func (f *FallbackManager) Capture(name string, withEscapes bool) (string, error) {
	return "", ErrAttachUnavailable
}

// Send returns ErrAttachUnavailable — no tmux pane to send keys to.
func (f *FallbackManager) Send(name, keys string) error {
	return fmt.Errorf("%w: tmux not installed", ErrAttachUnavailable)
}

// Attach returns ErrAttachUnavailable.
func (f *FallbackManager) Attach(name string) (string, error) {
	return "", fmt.Errorf("%w: tmux not installed", ErrAttachUnavailable)
}

// DeadStatus reports whether the process has exited.
func (f *FallbackManager) DeadStatus(name string) (code int, dead bool, err error) {
	f.mu.Lock()
	sess, ok := f.sessions[name]
	f.mu.Unlock()
	if !ok {
		// Session not found = consider dead, but report -1 so callers do NOT
		// interpret this as a successful (exit-0) completion. This matches
		// RealManager.DeadStatus's "no numeric exit status" sentinel; the
		// orchestrator's watchRun/finishRun must see a non-zero code so the
		// run is marked failed, not done.
		return -1, true, nil
	}
	select {
	case <-sess.done:
		return sess.exitCode, true, nil
	default:
		return 0, false, nil
	}
}

// Kill terminates the child process.
func (f *FallbackManager) Kill(name string) error {
	f.mu.Lock()
	sess, ok := f.sessions[name]
	if ok {
		delete(f.sessions, name)
	}
	f.mu.Unlock()
	if !ok {
		return nil
	}
	if sess.cmd.Process != nil {
		_ = sess.cmd.Process.Kill()
	}
	return nil
}

// List returns an empty slice — fallback has no persistent sessions to discover.
func (f *FallbackManager) List() ([]Session, error) {
	return nil, nil
}

// IsAttachUnavailable reports whether err (or any error in its chain) is
// ErrAttachUnavailable. It is equivalent to errors.Is(err, ErrAttachUnavailable).
func IsAttachUnavailable(err error) bool {
	return errors.Is(err, ErrAttachUnavailable)
}
