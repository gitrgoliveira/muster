package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gitrgoliveira/muster/internal/adapter"
	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/tmux"
	"github.com/gitrgoliveira/muster/internal/ws"
	"github.com/gitrgoliveira/muster/internal/wt"
)

// ErrRunAlreadyActive is returned by Dispatch when a run is already active for
// the given bead (409 Conflict in the HTTP layer).
var ErrRunAlreadyActive = errors.New("run already active for bead")

// ErrInvalidBeadID is returned when req.BeadID doesn't match the canonical
// bead-ID format. beadID becomes a filesystem path segment and a git branch
// name downstream, so an unvalidated value could escape the worktrees dir.
var ErrInvalidBeadID = errors.New("invalid bead ID")

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

// ErrVCSUnavailable is returned when the configured default VCS binary is not
// installed on PATH at startup. Maps to HTTP 412 VCS_UNAVAILABLE.
var ErrVCSUnavailable = errors.New("VCS binary unavailable")

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
	BeadTitle      string // preserved for queued runs so launchAdmittedRun can build the prompt
	BeadDesc       string // preserved for queued runs so launchAdmittedRun can build the prompt
	StepIdx        int    // 0 for runs Dispatch creates in M2; recovery preserves whatever the session name encodes
	Loop           int    // 0 for runs Dispatch creates in M2; recovery preserves whatever the session name encodes
	Agent          core.AgentID
	Mode           core.Mode
	PermissionMode core.PermissionMode
	Worktree       string          // absolute path to the worktree
	Session        string          // canonical session name muster/<bead>/<step>/<loop>; set in BOTH tmux and fallback (fallback keys its in-memory sessions by it)
	Pane           string          // tmux pane id, e.g. "%3" (empty in fallback; informational only)
	State          core.StepStatus // core.StepStatus: pending | active | done | failed (active = running; cancel/timeout folds into failed)
	ExitCode       int
	StartedAt      time.Time
	EndedAt        time.Time

	// cancel cancels the context for this run's watcher goroutine.
	cancel context.CancelFunc

	// pipe is the pane output reader (tmux FIFO or fallback stdout). Closed in
	// finishRun so the real tmux manager removes the FIFO + temp dir (no leak).
	pipe io.ReadCloser

	// M4 additions (additive; immutable after Dispatch populates them).

	// Chain is the ordered step pipeline for this run. nil means single-step
	// (M2 default behaviour is preserved). Set at dispatch time; US3 advances
	// a per-bead pointer through the chain.
	Chain *StepChain

	// Quota holds the token/cost usage captured at run end. Known=false until
	// US5 wires the on-disk quota reader; runs before US5 leave it zero.
	// Mutable under o.mu (set in finishRun once the session exits).
	Quota QuotaUsage

	// Waiting is true while the run is queued in the scheduler waiting for
	// a free capacity slot (State==StepPending). Flipped to false when the
	// run is admitted and the agent session is actually launched.
	// Mutable under o.mu.
	Waiting bool

	// pendingAdvance is true while an Advance/LoopBack is in progress for this
	// run. When true, finishRun skips eviction and instead relaunches the next
	// step (at pendingAdvanceNextIdx) under the same beadID key.
	// Mutable under o.mu.
	pendingAdvance bool

	// pendingAdvanceNextIdx is the target step index set by Advance/LoopBack.
	// Valid only when pendingAdvance is true. Mutable under o.mu.
	pendingAdvanceNextIdx int
}

// DispatchRequest carries the inputs for Orchestrator.Dispatch.
type DispatchRequest struct {
	BeadID         string
	BeadTitle      string
	BeadDesc       string
	Agent          core.AgentID
	Mode           core.Mode
	PermissionMode core.PermissionMode // empty = use DefaultPermissionMode

	// Chain is an optional step chain override for this dispatch. When non-nil
	// it takes precedence over the orchestrator's configured default chain.
	// nil means: use the configured default chain, or single implicit step 0
	// (M2 behaviour) if no default chain is set. Per-step PermissionMode is
	// never silently defaulted (FR-012a).
	Chain *StepChain
}

// DispatchResult is the return value of Orchestrator.Dispatch.
// Joined is true when the bead was already in-flight (idempotent join, M4 US4).
// Queued is true when the bead was accepted but is waiting for a capacity slot
// (M4 US1). A Joined result with Queued true means the bead is joining a waiter.
// Bead is always the active *core.Bead (existing run on join, new run otherwise).
type DispatchResult struct {
	Bead   *core.Bead
	Joined bool // true when joining an in-flight run (idempotent dispatch)
	Queued bool // true when admitted to the waiting queue (capacity full)
}

// defaultSchedulerCapacity is the capacity used when Config.MaxConcurrent is unset.
const defaultSchedulerCapacity = 4

// Publisher is a function that broadcasts a WS frame to connected clients.
type Publisher func(frame ws.Frame)

// Orchestrator manages agent run lifecycle.
type Orchestrator struct {
	mu   sync.RWMutex
	runs map[string]*Run // keyed by beadID

	// sched is the capacity-gated FIFO scheduler (M4 US1). All sched methods
	// must be called with mu held (write lock). setCapacity acquires the lock
	// internally.
	sched *scheduler

	adapters        *adapter.Registry
	transport       tmux.Manager // may be a fallback transport
	repoMap         RepoMap
	worktreesDir    string
	backend         wt.Backend // VCS backend; defaults to the backend for defaultVCS (git or jj) when nil at construction
	defaultVCS      wt.VCS     // VCS selected at startup; "" defaults to git
	vcsAvailable    wt.Availability
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

	// defaultChain is the per-orchestrator default step chain applied to all
	// dispatches that do not supply an explicit chain (nil DispatchRequest.Chain).
	// nil means single implicit step 0 (M2 behaviour). Set at construction via
	// Config.DefaultChain. Per-step PermissionMode is NEVER silently defaulted
	// (FR-012a) — a chain with an empty PermissionMode on any step will cause
	// an ErrNoPermissionMode when that step is launched.
	defaultChain *StepChain
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
	Backend         wt.Backend // VCS backend; defaults to the backend for DefaultVCS (git or jj) when nil
	DefaultVCS      wt.VCS     // "git" (default) or "jj"; checked against wt.Detect at construction
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

	// MaxConcurrent is the maximum number of concurrently active runs (M4 US1).
	// 0 or negative defaults to 4 (defaultSchedulerCapacity). Use SetCapacity
	// to change it at runtime; Dispatch enforces it via the FIFO scheduler.
	MaxConcurrent int

	// DefaultChain is the default step chain applied to dispatches that do not
	// supply an explicit chain. nil means single implicit step 0 (M2 behaviour).
	// Per-step PermissionMode is NEVER silently defaulted (FR-012a).
	DefaultChain *StepChain
}

// defaultRunRetention is used when Config.RunRetention is unset (0).
const defaultRunRetention = 1 * time.Hour

// New creates a new Orchestrator.
func New(cfg Config) *Orchestrator {
	runRetention := cfg.RunRetention
	if runRetention <= 0 {
		runRetention = defaultRunRetention
	}
	// A nil transport would panic on the first Dispatch/GetAttach/SendKeys.
	// Default to the tmux-absent fallback (the same degraded mode used when
	// tmux Detect fails) rather than crashing.
	transport := cfg.Transport
	if transport == nil {
		transport = tmux.NewFallbackManager()
	}

	// Default VCS to git when unset.
	defaultVCS := cfg.DefaultVCS
	if defaultVCS == "" {
		defaultVCS = wt.VCSGit
	}

	// Startup probe: detect which VCS binaries are available.
	// Use a short background context so a hung `git --version` or `jj --version`
	// at startup doesn't stall server boot indefinitely.
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	avail := wt.Detect(probeCtx)
	probeCancel()

	// Default to the backend matching defaultVCS, with worktreesDir baked in.
	// Selecting by defaultVCS (rather than always git) avoids silently running
	// git operations on an instance configured for jj — no cross-backend fallback.
	backend := cfg.Backend
	if backend == nil {
		switch defaultVCS {
		case wt.VCSJJ:
			backend = wt.NewJJBackend(cfg.WorktreesDir)
		default:
			backend = wt.NewGitBackend(cfg.WorktreesDir)
		}
	}

	// Initialise the FIFO scheduler (M4 US1). Default to 4 concurrent runs
	// when MaxConcurrent is unset (0) or negative.
	schedCap := cfg.MaxConcurrent
	if schedCap <= 0 {
		schedCap = defaultSchedulerCapacity
	}

	return &Orchestrator{
		runs:            make(map[string]*Run),
		sched:           newScheduler(schedCap),
		adapters:        cfg.Adapters,
		transport:       transport,
		repoMap:         cfg.RepoMap,
		worktreesDir:    cfg.WorktreesDir,
		backend:         backend,
		defaultVCS:      defaultVCS,
		vcsAvailable:    avail,
		defaultPermMode: cfg.DefaultPermMode,
		publish:         cfg.Publish,
		runTimeout:      cfg.RunTimeout,
		runRetention:    runRetention,
		onComplete:      cfg.OnComplete,
		defaultChain:    cfg.DefaultChain,
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

// SetCapacity changes the scheduler's maximum concurrency at runtime.
// n must be > 0; returns ErrInvalidCapacity otherwise. Lowering below the
// number of currently active runs drains (never kills running agents).
// Raising above the current active count immediately admits waiters FIFO.
func (o *Orchestrator) SetCapacity(n int) error {
	admitted, err := o.sched.setCapacity(&o.mu, n)
	if err != nil {
		return err
	}
	// Launch newly admitted runs outside the lock (slow IO: Detect/Ensure/Spawn).
	for _, run := range admitted {
		go o.launchAdmittedRun(run)
	}
	return nil
}

// SchedulerSnapshot returns a point-in-time view of the scheduler state.
func (o *Orchestrator) SchedulerSnapshot() SchedulerSnapshot {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.sched.snapshot()
}

// WorktreeCount returns the number of per-bead worktree directories currently
// present under the configured --worktrees-dir. Each entry is counted if it is
// a directory (not a symlink or regular file). Returns 0 when worktreesDir is
// empty or when the directory does not exist yet.
func (o *Orchestrator) WorktreeCount() int {
	if o.worktreesDir == "" {
		return 0
	}
	entries, err := os.ReadDir(o.worktreesDir)
	if err != nil {
		// Dir doesn't exist yet or is unreadable — not an error, just 0.
		return 0
	}
	count := 0
	for _, e := range entries {
		// Explicitly skip symlinks so a symlink-to-directory can't be counted —
		// this makes the "not a symlink" guarantee hold even on filesystems that
		// report an unknown d_type (where DirEntry.IsDir alone could misfire).
		if e.Type()&os.ModeSymlink != 0 {
			continue
		}
		if e.IsDir() {
			count++
		}
	}
	return count
}

// VCSAvailability returns the wt.Availability snapshot captured at startup.
// Used by the status handler to surface which VCS binaries are present.
func (o *Orchestrator) VCSAvailability() wt.Availability {
	return o.vcsAvailable
}

// DefaultVCSString returns the string form of the configured default VCS.
func (o *Orchestrator) DefaultVCSString() string {
	return string(o.defaultVCS)
}

// RunSummary is a lightweight point-in-time snapshot of a Run for status reporting.
// It is returned by ListRunSummaries and consumed by the health status handler via
// an adapter in main.go (which converts to health.RunSummaryDTO to avoid an
// import cycle between orchestrator and api/health).
type RunSummary struct {
	BeadID   string
	StepIdx  int
	ChainLen int // 0 when no chain (single-step M2 run)
	State    core.StepStatus
}

// ListRunSummaries returns a snapshot of all currently-tracked runs, ordered
// by BeadID for deterministic output. Callers may read the returned slice
// without holding a lock.
func (o *Orchestrator) ListRunSummaries() []RunSummary {
	o.mu.RLock()
	defer o.mu.RUnlock()
	summaries := make([]RunSummary, 0, len(o.runs))
	for _, r := range o.runs {
		chainLen := 0
		if r.Chain != nil {
			chainLen = len(*r.Chain)
		}
		summaries = append(summaries, RunSummary{
			BeadID:   r.BeadID,
			StepIdx:  r.StepIdx,
			ChainLen: chainLen,
			State:    r.State,
		})
	}
	return summaries
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
//
// Plan mode is a special case: the claude adapter's Modes() table hardcodes
// "--permission-mode plan" for core.ModePlan regardless of whatever value is
// passed in (see claude.Adapter.Modes) — plan mode is inherently read-only,
// not a user-selectable autonomy level. Without this carve-out, a plan-mode
// dispatch would spuriously require a permissionMode that's discarded before
// it ever reaches the CLI, and — worse — the fallback-transport "prompting
// mode" warning below would judge hang risk against a resolved value that
// isn't actually what gets passed to claude.
func (o *Orchestrator) resolvePermMode(mode core.Mode, requested core.PermissionMode) (core.PermissionMode, error) {
	if mode == core.ModePlan {
		if requested != "" && requested != core.PermPlan {
			return "", &PermModeError{Mode: requested}
		}
		return core.PermPlan, nil
	}
	if requested != "" {
		if !requested.Valid() {
			return "", &PermModeError{Mode: requested}
		}
		// PermPlan is the plan-mode permission. Accepting it for a non-plan
		// dispatch would run the agent in plan mode while the request is still
		// labelled/logged as agent mode — ambiguous and off-contract. Reject.
		if requested == core.PermPlan {
			return "", &PermModeError{Mode: requested}
		}
		return requested, nil
	}
	if o.defaultPermMode != "" {
		// Same carve-out as the requested path: a configured default of "plan"
		// is meaningless for a non-plan dispatch and would silently run plan
		// mode while labelled agent. Reject rather than apply it. (main.go also
		// rejects --default-permission-mode=plan at startup; this is the
		// defense-in-depth guard for any other construction path.)
		if o.defaultPermMode == core.PermPlan {
			return "", &PermModeError{Mode: o.defaultPermMode}
		}
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

// promptFileNameForStep returns the prompt filename for the given step index.
// Each step gets its own file so step 1 does not overwrite step 0's prompt
// (which may still be in use by the watcher goroutine or for debugging).
// For step 0 this returns ".muster-prompt-0.txt" — byte-for-byte identical to
// the M2 constant, preserving backward compatibility.
func promptFileNameForStep(stepIdx int) string {
	return fmt.Sprintf(".muster-prompt-%d.txt", stepIdx)
}

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
//  2. Admits or enqueues the run via the FIFO scheduler.
//  3. If admitted: resolves repo, ensures worktree, writes prompt, invokes adapter, spawns session.
//  4. If queued: registers a StepPending run and returns immediately (Queued:true).
//     The agent session is launched in launchAdmittedRun when a slot opens.
//  5. On success, returns DispatchResult with Queued indicating whether the run
//     is waiting for a capacity slot.
//
// Returns a DispatchResult with the bead and its queuing status.
func (o *Orchestrator) Dispatch(ctx context.Context, req DispatchRequest) (DispatchResult, error) {
	// Defense in depth: validate the bead ID before it flows into a repo-map
	// lookup, a tmux session name, and (via worktree.Ensure) a filesystem path
	// + git branch name. The HTTP handler already allow-lists IDs, but Dispatch
	// is a public entry point and must not trust its caller — the same reason
	// recovery validates session-derived IDs.
	if !core.ValidBeadID(req.BeadID) {
		return DispatchResult{}, ErrInvalidBeadID
	}

	// A missing worktreesDir is a construction/wiring error (cmd/muster always
	// sets it): worktree.Ensure would otherwise filepath.Join("", beadID) into
	// a relative "./<beadID>" and create/reuse a worktree under the current
	// working directory. Fail fast, before reserving a run, rather than
	// scattering worktrees wherever the process happens to be running.
	if o.worktreesDir == "" {
		return DispatchResult{}, fmt.Errorf("orchestrator: worktreesDir is not configured")
	}

	// Refuse dispatch when the configured VCS binary was absent at startup
	// (FR-011). We check the availability snapshot captured in New() rather
	// than re-probing on each dispatch to avoid per-request subprocess overhead.
	switch o.defaultVCS {
	case wt.VCSJJ:
		if !o.vcsAvailable.JJ {
			return DispatchResult{}, fmt.Errorf("%w: jj not found on PATH", ErrVCSUnavailable)
		}
	default: // git
		if !o.vcsAvailable.Git {
			return DispatchResult{}, fmt.Errorf("%w: git not found on PATH", ErrVCSUnavailable)
		}
	}

	// Resolve permission mode.
	pm, err := o.resolvePermMode(req.Mode, req.PermissionMode)
	if err != nil {
		return DispatchResult{}, err
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
	// leaking a goroutine and orphaning a tmux session.
	//
	// M4 US1: the scheduler admits or enqueues the reservation under the same lock.
	// An admitted run is marked StepActive; a queued run is marked StepPending.
	o.mu.Lock()
	if existing, ok := o.runs[req.BeadID]; ok && (existing.State == core.StepActive || existing.State == core.StepPending) {
		o.mu.Unlock()
		return DispatchResult{}, ErrRunAlreadyActive
	}
	// Resolve the step chain for this dispatch. M2 single-step behaviour
	// (nil Chain) is preserved when no chain is supplied in the request and
	// no default chain is configured.
	resolvedChain := o.resolveChain(req)

	reserved := &Run{
		BeadID:         req.BeadID,
		BeadTitle:      req.BeadTitle,
		BeadDesc:       req.BeadDesc,
		Agent:          req.Agent,
		Mode:           req.Mode,
		PermissionMode: pm,
		State:          core.StepPending,
		StartedAt:      time.Now(),
		Chain:          resolvedChain,
	}
	queued := o.sched.admitOrEnqueue(reserved)
	if !queued {
		// Admitted: flip state to active now.
		reserved.State = core.StepActive
	}
	o.registerRun(reserved)

	// Capture the waiting position for the queued event (0-based FIFO index).
	var waitingPos int
	if queued {
		waitingPos = len(o.sched.waiting) - 1 // this run is last in the queue
	}
	o.mu.Unlock()

	// If queued, emit dispatch.queued WS event and return immediately.
	// The agent session is launched in launchAdmittedRun when a slot opens.
	if queued {
		if o.publish != nil {
			o.publish(ws.Frame{
				Type:       ws.EventDispatchQueued,
				BeadID:     req.BeadID,
				WaitingPos: &waitingPos,
			})
		}
		bead := &core.Bead{
			ID:     req.BeadID,
			Title:  req.BeadTitle,
			Desc:   req.BeadDesc,
			Column: core.ColRunning,
		}
		return DispatchResult{Bead: bead, Queued: true}, nil
	}

	// Admitted path: do the slow IO work outside the lock.
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
			o.sched.onRunEnd(req.BeadID) // remove from active set
		}
		o.mu.Unlock()
	}()

	bead, err := o.doLaunch(ctx, reserved, req, pm)
	if err != nil {
		return DispatchResult{}, err
	}
	success = true
	return DispatchResult{Bead: bead}, nil
}

// doLaunch performs the slow IO work for an admitted run: adapter detect,
// worktree ensure, prompt write, adapter invoke, tmux spawn, watcher start.
// Must NOT be called with o.mu held.
func (o *Orchestrator) doLaunch(ctx context.Context, run *Run, req DispatchRequest, pm core.PermissionMode) (*core.Bead, error) {
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
	// cannot pin the run reservation indefinitely — otherwise the bead would
	// remain undispatchable until the server restarts.
	// context.WithoutCancel detaches from ctx's cancellation (ctx is the HTTP
	// request context; a client disconnect cancels it) so only OUR deadline can
	// end the probe — a disconnecting client must not be able to SIGKILL an
	// in-flight `claude` subprocess via exec.CommandContext.
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
	// `git rev-parse` / `git worktree add` would otherwise hold the run
	// reservation forever. context.WithoutCancel detaches from ctx's
	// cancellation for the same reason as the Detect probe above.
	ensureCtx, ensureCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	wtPath, err := o.backend.Create(ensureCtx, o.worktreesDir, repoPath, req.BeadID)
	ensureCancel()
	if err != nil {
		return nil, fmt.Errorf("worktree: %w", err)
	}

	// Write prompt file. 0o600 keeps the file readable only by the muster
	// process owner — the bead prompt may contain sensitive task context, and
	// other local users have no need to read it. filepath.Join (not a
	// hard-coded "/") keeps this portable to Windows, where the adapter's own
	// filepath.Rel(Worktree, PromptFile) contract check expects an OS-native
	// separator throughout.
	// Each step gets its own prompt file (.muster-prompt-<stepIdx>.txt) so
	// step 1 does not overwrite step 0's file. For step 0 this is
	// ".muster-prompt-0.txt" — M2 byte-for-byte identical.
	promptPath := filepath.Join(wtPath, promptFileNameForStep(run.StepIdx))
	prompt := buildPrompt(req.BeadTitle, req.BeadDesc)
	if err := os.WriteFile(promptPath, []byte(prompt), 0o600); err != nil {
		return nil, fmt.Errorf("write prompt: %w", err)
	}

	// Invoke adapter to get the Spec.
	spec, err := adp.Invoke(ctx, adapter.InvokeReq{
		Mode:           req.Mode,
		PermissionMode: pm,
		Worktree:       wtPath,
		PromptFile:     promptPath,
	})
	if err != nil {
		return nil, fmt.Errorf("adapter invoke: %w", err)
	}

	// Spawn tmux session.
	sessionName := tmux.SessionName(req.BeadID, run.StepIdx, run.Loop)
	sess, err := o.transport.Spawn(sessionName, spec.Cwd, spec.Env, spec.Argv)
	if err != nil {
		return nil, fmt.Errorf("tmux spawn: %w", err)
	}

	// Fill the run's fields in place (keeping the same pointer the
	// reservation registered, so the TOCTOU guard's identity check holds).
	runCtx, runCancel := context.WithCancel(context.Background())
	o.mu.Lock()
	run.Worktree = wtPath
	run.Session = sess.Name
	run.Pane = sess.Pane
	run.State = core.StepActive
	run.cancel = runCancel
	o.mu.Unlock()

	// Start pipe + exit watcher goroutine.
	// Pipe the pane output to the WS hub as runlog.line frames. Use sess.Name
	// (what Spawn actually created), not the requested sessionName, so this
	// stays consistent with run.Session and the rest of the lifecycle
	// (tmux.session.opened, DeadStatus/Kill) if a transport ever canonicalizes
	// the name. They're equal for today's managers, but relying on that here
	// would be a latent bug.
	pipeReader, pipeErr := o.transport.Pipe(sess.Name)
	if pipeErr != nil {
		// Pipe failure is non-fatal (output won't stream, but the run continues).
		// Log it: without this, a "no runlog.line events" incident looks like a
		// stuck orchestrator when it's really just a failed/missing pipe.
		slog.Warn("dispatch: runlog pipe failed; streaming disabled for this run",
			"bead", req.BeadID, "session", sess.Name, "err", pipeErr)
		pipeReader = nil
	}
	if pipeReader != nil {
		// Under o.mu like the other post-Spawn field writes above: the run is
		// already visible in o.runs (registered as a reservation), so a
		// concurrent GetRun — which snapshots the whole struct under RLock —
		// would otherwise race this write. watchRun (started below) reads
		// run.pipe only after this point, so its later read is ordered safely.
		o.mu.Lock()
		run.pipe = pipeReader // closed in finishRun (frees the FIFO/temp dir)
		o.mu.Unlock()
		streamer := &runlogStreamer{
			beadID:  req.BeadID,
			stepIdx: run.StepIdx,
			publish: o.publish,
		}
		go streamer.stream(pipeReader)
	}

	go o.watchRun(runCtx, run)

	// Emit tmux.session.opened and dispatch.admitted (if this was a queued run).
	if o.publish != nil {
		o.publish(ws.Frame{
			Type:    ws.EventTmuxOpened,
			BeadID:  req.BeadID,
			StepIdx: intPtr(run.StepIdx),
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

// launchAdmittedRun performs the slow IO work to start a previously-queued run
// that has been admitted from the FIFO queue. It is called in a goroutine so it
// doesn't block the finishRun/onRunEnd path.
func (o *Orchestrator) launchAdmittedRun(run *Run) {
	// Use a background context — the original request's context is long gone.
	ctx := context.Background()

	// Emit dispatch.admitted event.
	if o.publish != nil {
		o.publish(ws.Frame{
			Type:    ws.EventDispatchAdmitted,
			BeadID:  run.BeadID,
			StepIdx: intPtr(run.StepIdx),
		})
	}

	req := DispatchRequest{
		BeadID:         run.BeadID,
		BeadTitle:      run.BeadTitle,
		BeadDesc:       run.BeadDesc,
		Agent:          run.Agent,
		Mode:           run.Mode,
		PermissionMode: run.PermissionMode,
	}

	_, err := o.doLaunch(ctx, run, req, run.PermissionMode)
	if err != nil {
		slog.Error("launchAdmittedRun: failed to launch admitted queued run",
			"bead", run.BeadID, "err", err)
		// Remove from the registry and scheduler active set on failure.
		o.mu.Lock()
		if cur, ok := o.runs[run.BeadID]; ok && cur == run {
			delete(o.runs, run.BeadID)
			o.sched.onRunEnd(run.BeadID)
		}
		o.mu.Unlock()
	}
}

// watchRun monitors a run for exit via DeadStatus polling, then transitions
// the bead and emits tmux.session.closed.
func (o *Orchestrator) watchRun(ctx context.Context, run *Run) {
	// Capture the cancel func once, under the read lock, immediately on entry.
	// doLaunch sets run.cancel before calling go watchRun, so it is already set
	// when this goroutine starts. We must NOT read run.cancel again after this
	// point: relaunchNextStep resets it to nil under the write lock (the advance
	// path), so any later bare read of run.cancel would race.
	o.mu.RLock()
	cancelFn := run.cancel
	o.mu.RUnlock()
	defer func() {
		if cancelFn != nil {
			cancelFn()
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

		// Read run.Session under the lock: the M4 advance path (Advance/LoopBack)
		// clears this field under the write lock so a bare read here would race.
		o.mu.RLock()
		sess := run.Session
		o.mu.RUnlock()

		code, dead, err := o.transport.DeadStatus(sess)
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

	// Snapshot session and pipe under the read lock. These fields were
	// "immutable after launch" in M2 (set once in doLaunch), but the M4
	// advance path (Advance/LoopBack in steps.go) now writes them under the
	// write lock to clear them before relaunchNextStep sets new values. Reading
	// them under the read lock here avoids a data race while preserving the
	// Kill-before-state-flip ordering (Kill happens below, after this lock release,
	// which is still before the write lock's state flip further down).
	o.mu.RLock()
	session := run.Session
	pipe := run.pipe
	o.mu.RUnlock()

	// Kill the tmux session (remain-on-exit keeps it alive; we must clean up).
	// Log a failure: a non-"session gone" error means the session may persist,
	// and a later re-dispatch of this bead would then fail with a duplicate
	// session whose root cause would otherwise be invisible.
	if err := o.transport.Kill(session); err != nil {
		slog.Warn("finishRun: tmux Kill failed; session may persist and block re-dispatch",
			"bead", run.BeadID, "session", session, "err", err)
	}

	// Close the pane pipe so the real tmux manager removes its FIFO + temp dir.
	// The session is killed above, so the stream goroutine has hit (or will hit)
	// EOF; closing is idempotent-safe here as finishRun runs once per run.
	if pipe != nil {
		_ = pipe.Close()
	}

	// Only NOW flip the State off StepActive — see the ordering note above.
	// Also admit the next FIFO waiter (if any) while still holding the lock,
	// then launch it outside the lock (slow IO: Detect/Ensure/Spawn).
	//
	// INTERLOCK (T043b): when pendingAdvance is set, the bead is being advanced
	// to the next step under the SAME beadID key. Do NOT call onRunEnd — that
	// would free the scheduler capacity slot and potentially admit a waiter for
	// a different bead, leaving the advancing bead un-counted. The slot stays
	// occupied for the duration of the next step.
	o.mu.Lock()
	run.State = state
	run.ExitCode = exitCode
	run.EndedAt = time.Now()
	var nextRun *Run
	if !run.pendingAdvance {
		nextRun = o.sched.onRunEnd(run.BeadID)
		if nextRun != nil {
			nextRun.State = core.StepActive
		}
	}
	o.mu.Unlock()

	// Launch the next waiter outside the lock (Detect/Ensure/Invoke/Spawn are slow).
	if nextRun != nil {
		go o.launchAdmittedRun(nextRun)
	}

	// Emit tmux.session.closed. Use the local snapshot of session name
	// (captured under the read lock above) for the same race-safety reason.
	if o.publish != nil {
		ec := exitCode
		o.publish(ws.Frame{
			Type:     ws.EventTmuxClosed,
			BeadID:   run.BeadID,
			StepIdx:  intPtr(run.StepIdx),
			Session:  session,
			ExitCode: &ec,
		})
	}

	// Record completion on the bead (FR-013 / SC-007). M2 limitation: review is
	// not a distinct persisted status (it folds to in_progress per the beads
	// mapper), so the writeback records a note + bead.updated rather than moving
	// the bead to a review column.
	//
	// OnComplete is a caller-supplied extension point (wired to a bd CLI
	// shell-out + WS broadcast in main.go) running on this run's watcher
	// goroutine. A panic there — a bug in the CLI writeback path, a future
	// caller's callback, etc. — would otherwise be unhandled and crash the
	// entire muster process, taking down every other in-flight run along with
	// it. Recover and log instead: this run's own completion has already been
	// fully processed (state flipped, session killed, event published) by the
	// time OnComplete runs, so a failure here only means the bead never got
	// its outcome note, not a corrupted run.
	if o.onComplete != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("onComplete panicked; recovered to protect the watcher goroutine",
						"bead", run.BeadID, "panic", r)
				}
			}()
			o.onComplete(run.BeadID, exitCode, state == core.StepDone)
		}()
	}

	// Evict this run from the registry after the retention window so a
	// long-lived server doesn't accumulate an entry per bead ever dispatched.
	// INTERLOCK (T043b): if pendingAdvance is set, Advance/LoopBack already
	// set pendingAdvanceNextIdx and is waiting for this finishRun to do the
	// Kill+pipe.Close (above) before relaunching the next step. Spawn
	// relaunchNextStep on a new goroutine so this watcher goroutine can exit
	// promptly. relaunchNextStep will schedule eviction on its own if
	// doLaunch fails; on success the next step's finishRun will do so.
	o.mu.RLock()
	pending := run.pendingAdvance
	o.mu.RUnlock()
	if pending {
		go o.relaunchNextStep(run)
	} else {
		o.scheduleRunEviction(run)
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
