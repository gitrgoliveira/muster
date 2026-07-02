package tmux

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
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
		// Merge with the provided env winning deterministically. Simply
		// appending would leave duplicate KEY= entries whose lookup precedence
		// is platform/libc-dependent, so an override might not take effect. Drop
		// any inherited entry a provided one overrides, then append — matching
		// RealManager's env(1)-wrapper semantics (provided vars win).
		cmd.Env = mergeEnv(cmd.Environ(), env)
	}

	// Set up stdout/stderr pipe for streaming. Both streams share one
	// io.PipeWriter deliberately: os/exec detects that Stdout and Stderr are the
	// same interface value (interfaceEqual) and routes the child's stdout+stderr
	// through a single OS pipe with a single copy goroutine, so pw is never
	// written concurrently — and the combined, interleaved stream is exactly
	// what we want for a terminal-style runlog. NOTE: keep this a single shared
	// value; wrapping each side in its own writer would defeat that optimization
	// and spawn two copy goroutines. (io.PipeWriter is itself safe for parallel
	// Write per io.Pipe's contract, so even that would not race — just churn.)
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

	beadID, stepIdx, loop, parseErr := ParseSessionName(name)
	if parseErr != nil {
		// Non-canonical name (e.g. a test passing a bare string). Mirror
		// RealManager.Spawn: fall back to BeadID=name rather than returning a
		// Session with an empty BeadID/zero indices, which would surprise
		// callers relying on Session metadata.
		beadID = name
	}
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

// mergeEnv returns base with override applied so override wins deterministically
// regardless of platform duplicate-precedence rules: any base entry whose key an
// override entry also sets is dropped, then override is appended. Keys are the
// text before the first '='.
func mergeEnv(base, override []string) []string {
	overridden := make(map[string]struct{}, len(override))
	for _, e := range override {
		overridden[envKey(e)] = struct{}{}
	}
	merged := make([]string, 0, len(base)+len(override))
	for _, e := range base {
		if _, ok := overridden[envKey(e)]; ok {
			continue // an override provides this key; drop the inherited one
		}
		merged = append(merged, e)
	}
	return append(merged, override...)
}

// envKey returns the variable name of a "KEY=VALUE" entry (text before the
// first '='); an entry with no '=' is treated as its own key.
func envKey(entry string) string {
	key, _, _ := strings.Cut(entry, "=")
	return key
}
