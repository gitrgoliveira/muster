package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
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

// ErrUnsupportedMode is returned when the requested mode is not in the
// adapter's Modes() table. This is a client error (4xx), not a server error:
// it is rejected before any side effects (no worktree/session is created).
var ErrUnsupportedMode = errors.New("unsupported mode for adapter")

// Run holds the in-memory state of an active (or recently completed) agent run.
// The registry is rebuilt on restart from tmux.List().
//
// Field mutability contract:
//   - Immutable after Dispatch/recoverSession populate them and launch
//     watchRun (the Go memory model's happens-before on the `go` statement
//     ensures the watcher goroutine sees these): BeadID, StepIdx, Loop, Agent,
//     Mode, PermissionMode, Worktree, Session, StartedAt, cancel, pipe. These
//     may be read (e.g. by finishRun) without holding o.mu.
//   - Mutable; must be read/written under o.mu: State, ExitCode, EndedAt.
type Run struct {
	BeadID         string
	StepIdx        int // always 0 in M2
	Loop           int // always 0 in M2
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

	// pipe is the pane output reader (tmux FIFO or fallback stdout). Closed in
	// finishRun so the real tmux manager removes the FIFO + temp dir (no leak).
	pipe io.ReadCloser
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
	mu   sync.RWMutex
	runs map[string]*Run // keyed by beadID

	adapters        *adapter.Registry
	transport       tmux.Manager // may be a fallback transport
	repoMap         RepoMap
	worktreesDir    string
	defaultPermMode core.PermissionMode
	publish         Publisher
	runTimeout      time.Duration // 0 = no timeout
	runRetention    time.Duration // how long a finished run stays in o.runs before eviction

	// onComplete is invoked (nil-guarded) when a run finishes — on normal exit
	// (success = exit 0) and on the timeout/cancel path (exitCode -1, success
	// false). It runs on the watcher goroutine, so it must be non-blocking-safe.
	// M2 limitation: review is NOT a distinct persisted status (it folds to
	// in_progress per the beads mapper), so completion is recorded via a bead
	// note + a bead.updated WS frame rather than a column move.
	onComplete func(beadID string, exitCode int, success bool)
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

	// RunRetention bounds how long a finished run's entry stays in the
	// in-memory registry (o.runs) after completion, so a long-lived server
	// does not accumulate one entry per bead ever dispatched, unbounded.
	// GetRun/RunCount remain valid for this long after a run finishes (useful
	// for the API/UI to show a just-finished run's outcome). 0 = use
	// defaultRunRetention.
	RunRetention time.Duration

	// OnComplete, if set, is called when a run finishes (success = exit 0).
	// Wired in main.go to record the agent-run outcome on the bead (FR-013):
	// a note + bead.updated frame. It runs on the watcher goroutine.
	OnComplete func(beadID string, exitCode int, success bool)
}

// defaultRunRetention is used when Config.RunRetention is unset (0).
const defaultRunRetention = 1 * time.Hour

// New creates a new Orchestrator.
func New(cfg Config) *Orchestrator {
	runRetention := cfg.RunRetention
	if runRetention <= 0 {
		runRetention = defaultRunRetention
	}
	return &Orchestrator{
		runs:            make(map[string]*Run),
		adapters:        cfg.Adapters,
		transport:       cfg.Transport,
		repoMap:         cfg.RepoMap,
		worktreesDir:    cfg.WorktreesDir,
		defaultPermMode: cfg.DefaultPermMode,
		publish:         cfg.Publish,
		runTimeout:      cfg.RunTimeout,
		runRetention:    runRetention,
		onComplete:      cfg.OnComplete,
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

// scheduleRunEviction removes a finished run from the registry after
// o.runRetention has elapsed, so a long-lived server does not accumulate one
// o.runs entry per bead ever dispatched, unbounded — only StepActive runs are
// needed for concurrency control (Dispatch's duplicate check) and attach/send,
// but GetRun/RunCount stay valid for finished runs briefly (debugging, tests,
// and the API/UI showing a just-finished run's outcome).
//
// Guarded by pointer identity: if the bead is re-dispatched (a new *Run
// registered under the same beadID) before the eviction fires, the delete is
// a no-op — it must never remove a run that isn't the one that finished.
func (o *Orchestrator) scheduleRunEviction(run *Run) {
	time.AfterFunc(o.runRetention, func() {
		o.mu.Lock()
		if cur, ok := o.runs[run.BeadID]; ok && cur == run {
			delete(o.runs, run.BeadID)
		}
		o.mu.Unlock()
	})
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

// modeSupported reports whether the adapter's Modes() table contains mode.
func modeSupported(adp adapter.Adapter, mode core.Mode) bool {
	for _, m := range adp.Modes() {
		if m.ID == mode {
			return true
		}
	}
	return false
}

// isFallbackTransport returns true if the transport is a FallbackManager.
// Used to emit a warning when an interactive permission mode is used without tmux.
func isFallbackTransport(t tmux.Manager) bool {
	_, ok := t.(*tmux.FallbackManager)
	return ok
}

// promptingModes are permission modes that may block on a user prompt.
// Under the fallback transport (no tmux) there is no attachable session to
// answer such prompts, so the run can hang. FR-021 requires muster to warn.
var promptingModes = map[core.PermissionMode]bool{
	core.PermDefault:     true, // prompts for most actions
	core.PermAcceptEdits: true, // prompts for non-edit actions
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

	// FR-021: under the tmux-absent fallback there is no attachable session to
	// answer prompts, so a prompting permission mode can hang. Warn, don't block.
	if isFallbackTransport(o.transport) && promptingModes[pm] {
		slog.Warn("dispatching with a prompting permission mode without tmux; no attachable session to answer prompts — the run may hang",
			"bead", req.BeadID, "permissionMode", pm)
	}

	// Check for duplicate run and immediately insert a reservation under the
	// same lock to close the TOCTOU window. Without the reservation, two
	// concurrent dispatches for the same bead could both pass the check, both
	// do the slow Detect/Ensure/Invoke/Spawn work, and both spawn a session —
	// leaking a goroutine and orphaning a tmux session. The reservation marks
	// the bead StepActive up-front so the second caller gets ErrRunAlreadyActive.
	o.mu.Lock()
	if existing, ok := o.runs[req.BeadID]; ok && existing.State == core.StepActive {
		o.mu.Unlock()
		return nil, ErrRunAlreadyActive
	}
	reserved := &Run{
		BeadID:    req.BeadID,
		State:     core.StepActive,
		StartedAt: time.Now(),
	}
	o.registerRun(reserved)
	o.mu.Unlock()

	// On any early-return error, release the reservation (only if it's still
	// ours — a later success replaces the pointer's fields in place, not the
	// pointer itself, so identity holds; but a subsequent dispatch could have
	// recreated it after we failed, so guard on pointer identity).
	success := false
	defer func() {
		if success {
			return
		}
		o.mu.Lock()
		if cur, ok := o.runs[req.BeadID]; ok && cur == reserved {
			delete(o.runs, req.BeadID)
		}
		o.mu.Unlock()
	}()

	// Resolve adapter.
	if o.adapters == nil {
		return nil, ErrAdapterNotFound
	}
	adp, ok := o.adapters.Get(req.Agent)
	if !ok {
		return nil, ErrAdapterNotFound
	}

	// Detect adapter (installed + logged in). Bound the probe with an
	// independent short deadline so a hung claude binary or slow HTTP client
	// cannot pin the run reservation (`State=StepActive`, registered above)
	// indefinitely — otherwise the bead would remain undispatchable until the
	// server restarts. context.WithoutCancel detaches from ctx's cancellation
	// (ctx is the HTTP request context; a client disconnect cancels it) so
	// only OUR deadline can end the probe — a disconnecting client must not
	// be able to SIGKILL an in-flight `claude` subprocess via
	// exec.CommandContext, which would otherwise be indistinguishable from a
	// hung probe.
	detectCtx, detectCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	detectResult, err := adp.Detect(detectCtx)
	detectCancel()
	if err != nil {
		return nil, fmt.Errorf("adapter detect: %w", err)
	}
	if !detectResult.Installed {
		return nil, ErrAdapterNotInstalled
	}
	if !detectResult.LoggedIn {
		return nil, ErrAdapterNotLoggedIn
	}

	// Reject unsupported modes BEFORE any side effects (worktree creation,
	// prompt write, Invoke). Otherwise mode=build/review/apply/yolo passes core
	// validation, creates a worktree, then fails deep in Invoke → 500.
	if !modeSupported(adp, req.Mode) {
		return nil, ErrUnsupportedMode
	}

	// Resolve repo.
	repoPath, err := o.repoMap.Resolve(req.BeadID)
	if err != nil {
		return nil, err // ErrUnmappedPrefix
	}

	// Security audit: a fully-autonomous run (bypassPermissions) acts without
	// any confirmation prompts. Record it so the operator has a trail.
	if pm == core.PermBypassPermissions {
		slog.Warn("audit: dispatching with bypassPermissions (fully autonomous)",
			"bead", req.BeadID, "repo", repoPath)
	}

	// Ensure worktree. Bound the git subprocesses with an independent
	// short deadline for the same reason as the Detect probe above: a hung
	// `git rev-parse` / `git worktree add` (e.g. against a slow NFS mount or a
	// stuck `.git` lock) would otherwise hold the run reservation forever.
	// context.WithoutCancel detaches from ctx's cancellation for the same
	// reason as the Detect probe above: a client disconnect must not
	// SIGKILL a `git worktree add` mid-checkout via exec.CommandContext —
	// git writes the worktree's `.git` gitlink file before the checkout
	// completes, so a kill at the wrong moment would leave a directory that
	// LOOKS like a complete worktree (isWorktreeDir returns true) but has an
	// incomplete file checkout, and Ensure's reuse fast-path has no way to
	// distinguish that from a legitimately-reusable worktree. Only OUR
	// deadline (a genuinely hung subprocess) should be able to trigger this.
	ensureCtx, ensureCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	wt, err := worktree.Ensure(ensureCtx, o.worktreesDir, repoPath, req.BeadID)
	ensureCancel()
	if err != nil {
		return nil, fmt.Errorf("worktree: %w", err)
	}

	// Write prompt file. 0o600 keeps the file readable only by the muster
	// process owner — the bead prompt may contain sensitive task context, and
	// other local users have no need to read it.
	promptPath := wt.Path + "/" + promptFileName
	prompt := buildPrompt(req.BeadTitle, req.BeadDesc)
	if err := os.WriteFile(promptPath, []byte(prompt), 0o600); err != nil {
		return nil, fmt.Errorf("write prompt: %w", err)
	}

	// Invoke adapter to get the Spec.
	spec, err := adp.Invoke(ctx, adapter.InvokeReq{
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

	// Fill the reserved run's fields in place (keeping the same pointer the
	// reservation registered, so the TOCTOU guard's identity check holds).
	// StepIdx and Loop are omitted: both are already zero-valued in the
	// reservation allocation and M2 pins them at zero anyway (see the Run
	// struct field comments).
	runCtx, runCancel := context.WithCancel(context.Background())
	run := reserved
	o.mu.Lock()
	run.Agent = req.Agent
	run.Mode = req.Mode
	run.PermissionMode = pm
	run.Worktree = wt.Path
	run.Session = sess.Name
	run.State = core.StepActive
	run.cancel = runCancel
	o.mu.Unlock()

	// Mark success so the deferred reservation-cleanup is a no-op.
	success = true

	// Start pipe + exit watcher goroutine.
	// Pipe the pane output to the WS hub as runlog.line frames.
	pipeReader, pipeErr := o.transport.Pipe(sessionName)
	if pipeErr != nil {
		// Pipe failure is non-fatal (output won't stream, but the run continues).
		pipeReader = nil
	}
	if pipeReader != nil {
		run.pipe = pipeReader // closed in finishRun (frees the FIFO/temp dir)
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
			StepIdx: intPtr(0),
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

	// One ticker for the whole poll loop (avoids allocating a timer per iteration).
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-watchCtx.Done():
			// watchCtx fires here for the configured RunTimeout (FR-017) or
			// an explicit run cancellation via run.cancel (future cancel
			// endpoint). NOT graceful shutdown: watchRun is intentionally
			// rooted in context.Background() (FR-018), so server SIGTERM does
			// not cancel it — agent tmux sessions survive a muster restart.
			// finishRun kills the session, sets ExitCode=-1, emits closed,
			// records the outcome, and closes the pipe (same as non-zero exit).
			o.finishRun(run, -1, false)
			return
		case <-ticker.C:
		}

		code, dead, err := o.transport.DeadStatus(run.Session)
		if err != nil {
			// Session vanished (e.g. killed externally) — unknown exit, treat as
			// a failure with code -1 (not 0, which would look successful).
			o.finishRun(run, -1, false)
			return
		}
		if dead {
			o.finishRun(run, code, code == 0)
			return
		}
	}
}

// finishRun transitions a run to done/failed, emits closed WS event, and kills the session.
//
// Ordering matters: the tmux session is killed and the pipe is closed BEFORE
// the run's State flips off StepActive. Dispatch reuses a deterministic
// session name (`tmux.SessionName(beadID, 0, 0)`) and gates duplicate dispatch
// on `State == StepActive`. If the state flipped first, a concurrent
// re-dispatch could pass the duplicate check while tmux's remain-on-exit
// still held the previous session under the same name, surfacing a confusing
// "duplicate session" 500 from tmux new-session. Killing first frees the
// session name before any other caller can race in.
func (o *Orchestrator) finishRun(run *Run, exitCode int, success bool) {
	state := core.StepDone
	if !success {
		state = core.StepFailed
	}

	// Kill the tmux session (remain-on-exit keeps it alive; we must clean up).
	_ = o.transport.Kill(run.Session)

	// Close the pane pipe so the real tmux manager removes its FIFO + temp dir.
	// The session is killed above, so the stream goroutine has hit (or will hit)
	// EOF; closing is idempotent-safe here as finishRun runs once per run.
	if run.pipe != nil {
		_ = run.pipe.Close()
	}

	// Only NOW flip the State off StepActive — see the ordering note above.
	o.mu.Lock()
	run.State = state
	run.ExitCode = exitCode
	run.EndedAt = time.Now()
	o.mu.Unlock()

	// Emit tmux.session.closed.
	if o.publish != nil {
		ec := exitCode
		o.publish(ws.Frame{
			Type:     ws.EventTmuxClosed,
			BeadID:   run.BeadID,
			StepIdx:  intPtr(run.StepIdx),
			Session:  run.Session,
			ExitCode: &ec,
		})
	}

	// Record completion on the bead (FR-013 / SC-007). M2 limitation: review is
	// not a distinct persisted status (it folds to in_progress per the beads
	// mapper), so the writeback records a note + bead.updated rather than moving
	// the bead to a review column.
	if o.onComplete != nil {
		o.onComplete(run.BeadID, exitCode, state == core.StepDone)
	}

	// Evict this run from the registry after the retention window so a
	// long-lived server doesn't accumulate an entry per bead ever dispatched.
	o.scheduleRunEviction(run)
}

// buildPrompt assembles the bead title and description into a prompt string.
func buildPrompt(title, desc string) string {
	prompt := "# " + title + "\n\n"
	if desc != "" {
		prompt += desc + "\n"
	}
	return prompt
}
