package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/gitrgoliveira/muster/internal/adapter"
	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/tmux"
	"github.com/gitrgoliveira/muster/internal/worktree"
	"github.com/gitrgoliveira/muster/internal/ws"
)

// ErrRunAlreadyActive is returned by Dispatch when a run is already active for
// the given bead (409 Conflict in the HTTP layer).
var ErrRunAlreadyActive = errors.New("run already active for bead")

// ErrUnmappedPrefix is returned when the bead's ID prefix has no repo mapping.
var ErrUnmappedPrefix = errors.New("bead prefix has no repo mapping")

// ErrNoPermissionMode is returned when neither the request nor the default
// provides a permission mode (FR-021: muster never silently defaults autonomy).
var ErrNoPermissionMode = errors.New("permissionMode is required (no default configured)")

// ErrAdapterNotFound is returned when the requested adapter is not registered.
var ErrAdapterNotFound = errors.New("adapter not registered")

// ErrAdapterNotInstalled is returned when the adapter binary is not installed.
var ErrAdapterNotInstalled = errors.New("adapter not installed")

// ErrAdapterNotLoggedIn is returned when the adapter is not logged in.
var ErrAdapterNotLoggedIn = errors.New("adapter not logged in; run: claude auth login")

// Run holds the in-memory state of an active (or recently completed) agent run.
// The registry is rebuilt on restart from tmux.List().
type Run struct {
	BeadID         string
	StepIdx        int             // always 0 in M2
	Loop           int             // always 0 in M2
	Agent          core.AgentID
	Mode           core.Mode
	PermissionMode core.PermissionMode
	Worktree       string          // absolute path to the worktree
	Session        string          // tmux session name (empty in fallback)
	State          core.StepStatus // running | done | failed | cancelled
	ExitCode       int
	StartedAt      time.Time
	EndedAt        time.Time

	// cancel cancels the context for this run's watcher goroutine.
	cancel context.CancelFunc
}

// DispatchRequest carries the inputs for Orchestrator.Dispatch.
type DispatchRequest struct {
	BeadID         string
	BeadTitle      string
	BeadDesc       string
	Agent          core.AgentID
	Mode           core.Mode
	PermissionMode core.PermissionMode // empty = use DefaultPermissionMode
}

// Publisher is a function that broadcasts a WS frame to connected clients.
type Publisher func(frame ws.Frame)

// Orchestrator manages agent run lifecycle.
type Orchestrator struct {
	mu              sync.RWMutex
	runs            map[string]*Run // keyed by beadID

	adapters        *adapter.Registry
	transport       tmux.Manager    // may be a fallback transport
	repoMap         RepoMap
	worktreesDir    string
	defaultPermMode core.PermissionMode
	publish         Publisher
	runTimeout      time.Duration // 0 = no timeout
}

// RepoMap maps bead-ID prefixes to absolute repo paths.
type RepoMap map[string]string

// Resolve returns the repo path for a given beadID by extracting the prefix
// (everything before the first '-').
func (m RepoMap) Resolve(beadID string) (string, error) {
	prefix := prefixOf(beadID)
	path, ok := m[prefix]
	if !ok {
		return "", ErrUnmappedPrefix
	}
	return path, nil
}

// Config holds Orchestrator constructor options.
type Config struct {
	Adapters        *adapter.Registry
	Transport       tmux.Manager
	RepoMap         RepoMap
	WorktreesDir    string
	DefaultPermMode core.PermissionMode
	Publish         Publisher
	RunTimeout      time.Duration // 0 = no timeout (FR-017: opt-in only)
}

// New creates a new Orchestrator.
func New(cfg Config) *Orchestrator {
	return &Orchestrator{
		runs:            make(map[string]*Run),
		adapters:        cfg.Adapters,
		transport:       cfg.Transport,
		repoMap:         cfg.RepoMap,
		worktreesDir:    cfg.WorktreesDir,
		defaultPermMode: cfg.DefaultPermMode,
		publish:         cfg.Publish,
		runTimeout:      cfg.RunTimeout,
	}
}

// RunCount returns the number of currently active runs.
func (o *Orchestrator) RunCount() int {
	o.mu.RLock()
	defer o.mu.RUnlock()
	count := 0
	for _, r := range o.runs {
		if r.State == core.StepActive {
			count++
		}
	}
	return count
}

// GetRun returns a snapshot copy of the Run for a beadID, or nil if not found.
// Callers get a stable copy that can be read without further locking.
func (o *Orchestrator) GetRun(beadID string) *Run {
	o.mu.RLock()
	defer o.mu.RUnlock()
	r := o.runs[beadID]
	if r == nil {
		return nil
	}
	// Return a copy to prevent data races on individual field reads.
	copy := *r
	return &copy
}

// registerRun adds a run to the registry. Must be called with write lock held.
func (o *Orchestrator) registerRun(r *Run) {
	o.runs[r.BeadID] = r
}

// removeRun removes a run from the registry.
func (o *Orchestrator) removeRun(beadID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.runs, beadID)
}

// resolvePermMode returns the effective permission mode, applying the default
// if the request omits it, or returning ErrNoPermissionMode if neither is set.
func (o *Orchestrator) resolvePermMode(requested core.PermissionMode) (core.PermissionMode, error) {
	if requested != "" {
		if !requested.Valid() {
			return "", &PermModeError{Mode: requested}
		}
		return requested, nil
	}
	if o.defaultPermMode != "" {
		return o.defaultPermMode, nil
	}
	return "", ErrNoPermissionMode
}

// PermModeError is returned when an invalid permission mode is supplied.
type PermModeError struct {
	Mode core.PermissionMode
}

func (e *PermModeError) Error() string {
	return "invalid permissionMode: " + string(e.Mode)
}

// promptFileName is the filename of the prompt file within the worktree.
const promptFileName = ".muster-prompt-0.txt"

// isFallbackTransport returns true if the transport is a FallbackManager.
// Used to emit a warning when an interactive permission mode is used without tmux.
func isFallbackTransport(t tmux.Manager) bool {
	_, ok := t.(*tmux.FallbackManager)
	return ok
}

// promptingModes are permission modes that require user interaction (attach/send).
// When using the fallback transport (no tmux), these modes work but attach/send
// will be unavailable. FR-021 requires muster to warn the user.
var promptingModes = map[core.PermissionMode]bool{
	core.PermDefault: true, // default may prompt
}

// Dispatch launches an agent run for the given bead. It:
//  1. Validates inputs and checks for duplicate run.
//  2. Resolves the repo from the RepoMap.
//  3. Ensures the per-bead worktree exists.
//  4. Writes the assembled prompt to <worktree>/.muster-prompt-0.txt.
//  5. Calls adapter.Invoke to get the Spec.
//  6. Spawns the tmux session.
//  7. Registers the Run and starts the exit-watcher goroutine.
//  8. Emits tmux.session.opened.
//
// Returns a stub *core.Bead with updated column (the caller publishes bead.updated).
func (o *Orchestrator) Dispatch(ctx context.Context, req DispatchRequest) (*core.Bead, error) {
	// Resolve permission mode.
	pm, err := o.resolvePermMode(req.PermissionMode)
	if err != nil {
		return nil, err
	}

	// Check for duplicate run.
	o.mu.Lock()
	if existing, ok := o.runs[req.BeadID]; ok && existing.State == core.StepActive {
		o.mu.Unlock()
		return nil, ErrRunAlreadyActive
	}
	o.mu.Unlock()

	// Resolve adapter.
	if o.adapters == nil {
		return nil, ErrAdapterNotFound
	}
	adp, ok := o.adapters.Get(req.Agent)
	if !ok {
		return nil, ErrAdapterNotFound
	}

	// Detect adapter (installed + logged in).
	detectResult, err := adp.Detect(ctx)
	if err != nil {
		return nil, fmt.Errorf("adapter detect: %w", err)
	}
	if !detectResult.Installed {
		return nil, ErrAdapterNotInstalled
	}
	if !detectResult.LoggedIn {
		return nil, ErrAdapterNotLoggedIn
	}

	// Resolve repo.
	repoPath, err := o.repoMap.Resolve(req.BeadID)
	if err != nil {
		return nil, err // ErrUnmappedPrefix
	}

	// Ensure worktree.
	wt, err := worktree.Ensure(o.worktreesDir, repoPath, req.BeadID)
	if err != nil {
		return nil, fmt.Errorf("worktree: %w", err)
	}

	// Write prompt file.
	promptPath := wt.Path + "/" + promptFileName
	prompt := buildPrompt(req.BeadTitle, req.BeadDesc)
	if err := os.WriteFile(promptPath, []byte(prompt), 0644); err != nil {
		return nil, fmt.Errorf("write prompt: %w", err)
	}

	// Invoke adapter to get the Spec.
	spec, err := adp.Invoke(ctx, adapter.InvokeReq{
		Bead:           core.Bead{ID: req.BeadID, Title: req.BeadTitle, Desc: req.BeadDesc},
		Mode:           req.Mode,
		PermissionMode: pm,
		Worktree:       wt.Path,
		PromptFile:     promptPath,
	})
	if err != nil {
		return nil, fmt.Errorf("adapter invoke: %w", err)
	}

	// Spawn tmux session.
	sessionName := tmux.SessionName(req.BeadID, 0, 0)
	sess, err := o.transport.Spawn(sessionName, spec.Cwd, spec.Env, spec.Argv)
	if err != nil {
		return nil, fmt.Errorf("tmux spawn: %w", err)
	}

	// Register run.
	runCtx, runCancel := context.WithCancel(context.Background())
	run := &Run{
		BeadID:         req.BeadID,
		StepIdx:        0,
		Loop:           0,
		Agent:          req.Agent,
		Mode:           req.Mode,
		PermissionMode: pm,
		Worktree:       wt.Path,
		Session:        sess.Name,
		State:          core.StepActive,
		StartedAt:      time.Now(),
		cancel:         runCancel,
	}
	o.mu.Lock()
	o.registerRun(run)
	o.mu.Unlock()

	// Start pipe + exit watcher goroutine.
	// Pipe the pane output to the WS hub as runlog.line frames.
	pipeReader, pipeErr := o.transport.Pipe(sessionName)
	if pipeErr != nil {
		// Pipe failure is non-fatal (output won't stream, but the run continues).
		pipeReader = nil
	}
	if pipeReader != nil {
		streamer := &runlogStreamer{
			beadID:  req.BeadID,
			stepIdx: 0,
			publish: o.publish,
		}
		go streamer.stream(pipeReader)
	}

	go o.watchRun(runCtx, run)

	// Emit tmux.session.opened.
	if o.publish != nil {
		o.publish(ws.Frame{
			Type:    ws.EventTmuxOpened,
			BeadID:  req.BeadID,
			StepIdx: 0,
			Session: sess.Name,
		})
	}

	// Return a stub bead to signal the caller that the run is active.
	bead := &core.Bead{
		ID:     req.BeadID,
		Title:  req.BeadTitle,
		Desc:   req.BeadDesc,
		Column: core.ColRunning,
	}
	return bead, nil
}

// watchRun monitors a run for exit via DeadStatus polling, then transitions
// the bead and emits tmux.session.closed.
func (o *Orchestrator) watchRun(ctx context.Context, run *Run) {
	defer func() {
		if run.cancel != nil {
			run.cancel()
		}
	}()

	const pollInterval = 500 * time.Millisecond

	// Apply run timeout if configured (FR-017: opt-in only).
	watchCtx := ctx
	if o.runTimeout > 0 {
		var cancel context.CancelFunc
		watchCtx, cancel = context.WithTimeout(ctx, o.runTimeout)
		defer cancel()
	}

	for {
		select {
		case <-watchCtx.Done():
			// Cancelled due to timeout or graceful shutdown.
			// Kill the session and mark as cancelled.
			if run.Session != "" {
				_ = o.transport.Kill(run.Session)
			}
			o.mu.Lock()
			run.State = core.StepFailed // cancelled/timeout → failed
			run.EndedAt = time.Now()
			o.mu.Unlock()

			// Emit closed event.
			if o.publish != nil {
				ec := -1
				o.publish(ws.Frame{
					Type:     ws.EventTmuxClosed,
					BeadID:   run.BeadID,
					StepIdx:  run.StepIdx,
					Session:  run.Session,
					ExitCode: &ec,
				})
			}
			return
		case <-time.After(pollInterval):
		}

		code, dead, err := o.transport.DeadStatus(run.Session)
		if err != nil {
			// Session may have been killed externally; treat as done.
			o.finishRun(run, 0, false)
			return
		}
		if dead {
			o.finishRun(run, code, code == 0)
			return
		}
	}
}

// finishRun transitions a run to done/failed, emits closed WS event, and kills the session.
func (o *Orchestrator) finishRun(run *Run, exitCode int, success bool) {
	state := core.StepDone
	if !success {
		state = core.StepFailed
	}

	o.mu.Lock()
	run.State = state
	run.ExitCode = exitCode
	run.EndedAt = time.Now()
	o.mu.Unlock()

	// Kill the tmux session (remain-on-exit keeps it alive; we must clean up).
	_ = o.transport.Kill(run.Session)

	// Emit tmux.session.closed.
	if o.publish != nil {
		ec := exitCode
		o.publish(ws.Frame{
			Type:     ws.EventTmuxClosed,
			BeadID:   run.BeadID,
			StepIdx:  run.StepIdx,
			Session:  run.Session,
			ExitCode: &ec,
		})
	}
}

// buildPrompt assembles the bead title and description into a prompt string.
func buildPrompt(title, desc string) string {
	prompt := "# " + title + "\n\n"
	if desc != "" {
		prompt += desc + "\n"
	}
	return prompt
}
