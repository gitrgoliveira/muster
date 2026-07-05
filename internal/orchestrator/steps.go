package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/ws"
)

// ErrStepOutOfRange is returned by Advance or LoopBack when the requested step
// index is outside the valid range for this run's chain.
// Maps to HTTP 400 / CodeStepOutOfRange.
var ErrStepOutOfRange = errors.New("step index out of range")

// resolveChain resolves the step chain for a dispatch in the priority order:
//
//  1. Explicit chain in DispatchRequest (non-nil).
//  2. Configured default chain (o.defaultChain).
//  3. nil — single implicit step 0 (M2 behaviour).
//
// PromptRef is stored per StepProfile but resolves to the M2 bead prompt in M4
// (real skill/constitution assembly is M6). Per-step PermissionMode is never
// silently defaulted (FR-012a).
func (o *Orchestrator) resolveChain(req DispatchRequest) *StepChain {
	if req.Chain != nil {
		return req.Chain
	}
	if o.defaultChain != nil {
		return o.defaultChain
	}
	return nil
}

// Advance moves the step pointer forward by 1 for the live run keyed by beadID.
// It marks the run with pendingAdvance and records pendingTargetIdx under the
// lock — StepIdx is NOT mutated here; relaunchNextStep applies it after finishRun
// tears down the old session — then cancels the run's watcher goroutine. finishRun
// (called from watchRun) detects the pending advance and calls relaunchNextStep
// instead of evicting.
//
// Returns ErrStepOutOfRange if:
//   - No run exists for beadID.
//   - The run has no chain (single-step, can't advance).
//   - The run is already at the last step (StepIdx+1 >= len(chain)).
func (o *Orchestrator) Advance(beadID string) error {
	o.mu.Lock()
	run, ok := o.runs[beadID]
	if !ok {
		o.mu.Unlock()
		return fmt.Errorf("%w: no run for bead %q", ErrStepOutOfRange, beadID)
	}
	// A step transition is only valid on a live (running) step. A StepPending
	// (queued, not yet launched) run has not started step 0 — advancing its
	// pointer would skip step 0 and double-run the advanced step when the
	// scheduler later launches it (tri-review #1/#3). A terminal run (Done/
	// Failed) has no watcher to relaunch, so an advance would silently no-op
	// (tri-review #4). Guard both here.
	if st := run.State; st != core.StepActive {
		o.mu.Unlock()
		return fmt.Errorf("%w: bead %q run is not active (state=%s)", ErrStepOutOfRange, beadID, st)
	}
	if run.Chain == nil || len(*run.Chain) == 0 {
		o.mu.Unlock()
		return fmt.Errorf("%w: bead %q has no chain (single-step)", ErrStepOutOfRange, beadID)
	}
	chain := *run.Chain
	if cur := run.StepIdx; cur+1 >= len(chain) {
		o.mu.Unlock()
		return fmt.Errorf("%w: bead %q is already at the last step (%d)", ErrStepOutOfRange, beadID, cur)
	}
	// Re-entrancy guard: covers two windows where concurrent Advance is unsafe:
	//   1. A prior transition is in flight (finishRun has not yet run
	//      relaunchNextStep). A second Advance would advance StepIdx again
	//      before the first relaunch, skipping a step (tri-review #5).
	//   2. The run was just admitted but doLaunch has not yet armed run.cancel;
	//      calling cancel() would be a silent no-op (tri-review 6).
	// Both windows set pendingAdvance=true (admission sites for the latter).
	if run.pendingAdvance {
		o.mu.Unlock()
		return fmt.Errorf("%w: bead %q already has a step transition in progress", ErrStepOutOfRange, beadID)
	}

	nextIdx := run.StepIdx + 1
	chainLen := len(chain)
	// Record the target index ONLY; do NOT mutate StepIdx/Loop/Session here.
	// The current step's agent is still running: finishRun (triggered by the
	// cancel below) must kill the OLD session using the still-valid run.Session
	// and emit tmux.session.closed with the OLD run.StepIdx. relaunchNextStep
	// applies the target (sets StepIdx=pendingTargetIdx, clears Session) only
	// AFTER finishRun has cleaned up. Mutating them here orphaned the old tmux
	// session (Kill("")) and reported the wrong closed-event step (Copilot
	// #355/#357). pendingAdvance alone guards against a concurrent double-advance.
	run.pendingAdvance = true
	run.pendingTargetIdx = nextIdx

	// Capture cancel before releasing the lock. The cancel func itself is
	// immutable after launch (set once in doLaunch, never cleared); calling
	// it is idempotent and safe from any goroutine.
	cancel := run.cancel
	o.mu.Unlock()

	// Trigger the watcher goroutine to exit and call finishRun, which will
	// detect pendingAdvance and call relaunchNextStep.
	if cancel != nil {
		cancel()
	}

	// Emit step.advanced event.
	if o.publish != nil {
		o.publish(ws.Frame{
			Type:     ws.EventStepAdvanced,
			BeadID:   beadID,
			StepIdx:  intPtr(nextIdx),
			ChainLen: intPtr(chainLen),
		})
	}
	return nil
}

// LoopBack moves the step pointer back to toIdx for the live run keyed by beadID.
// It marks the run with pendingAdvance and records pendingTargetIdx under the
// lock — StepIdx is NOT mutated here; relaunchNextStep applies it after finishRun
// tears down the current session — then cancels the run's watcher goroutine.
// finishRun detects the pending advance and calls relaunchNextStep instead of evicting.
//
// Returns ErrStepOutOfRange if:
//   - No run exists for beadID.
//   - toIdx < 0.
//   - The run has no chain or toIdx >= run.StepIdx (must be strictly earlier).
func (o *Orchestrator) LoopBack(beadID string, toIdx int) error {
	if toIdx < 0 {
		return fmt.Errorf("%w: toIdx %d is negative", ErrStepOutOfRange, toIdx)
	}

	o.mu.Lock()
	run, ok := o.runs[beadID]
	if !ok {
		o.mu.Unlock()
		return fmt.Errorf("%w: no run for bead %q", ErrStepOutOfRange, beadID)
	}
	// Only a live (running) step can loop back (tri-review #3/#4) — see Advance.
	if st := run.State; st != core.StepActive {
		o.mu.Unlock()
		return fmt.Errorf("%w: bead %q run is not active (state=%s)", ErrStepOutOfRange, beadID, st)
	}
	if run.Chain == nil || len(*run.Chain) == 0 {
		o.mu.Unlock()
		return fmt.Errorf("%w: bead %q has no chain (single-step)", ErrStepOutOfRange, beadID)
	}
	if cur := run.StepIdx; toIdx >= cur {
		o.mu.Unlock()
		return fmt.Errorf("%w: toIdx %d must be < current stepIdx %d", ErrStepOutOfRange, toIdx, cur)
	}
	// No separate "toIdx >= len(chain)" check needed: the invariant StepIdx <
	// len(chain) (upheld by Advance) plus toIdx < StepIdx implies toIdx is in
	// range (tri-review #8, dead code removed).
	// Re-entrancy guard: same two-window coverage as Advance (tri-review #5, #6).
	if run.pendingAdvance {
		o.mu.Unlock()
		return fmt.Errorf("%w: bead %q already has a step transition in progress", ErrStepOutOfRange, beadID)
	}
	chainLen := len(*run.Chain)

	// Record the target index ONLY; do NOT mutate StepIdx/Loop/Session here
	// (same rationale as Advance — finishRun must clean up the OLD session/step
	// first; relaunchNextStep applies the target afterward). Copilot #355/#357.
	run.pendingAdvance = true
	run.pendingTargetIdx = toIdx
	cancel := run.cancel
	o.mu.Unlock()

	// Trigger watcher → finishRun → relaunchNextStep.
	if cancel != nil {
		cancel()
	}

	// Emit step.loopedback event.
	if o.publish != nil {
		o.publish(ws.Frame{
			Type:     ws.EventStepLoopedBack,
			BeadID:   beadID,
			StepIdx:  intPtr(toIdx),
			ChainLen: intPtr(chainLen),
		})
	}
	return nil
}

// relaunchNextStep is called from finishRun (on the watcher goroutine) when
// pendingAdvance is true. By the time it's called, finishRun has already killed
// the session and closed the pipe. This function resets the Run's step-specific
// mutable fields under the lock and relaunches via doLaunch.
//
// This is called as `go o.relaunchNextStep(run)` from finishRun so that the
// watcher goroutine can exit promptly.
func (o *Orchestrator) relaunchNextStep(run *Run) {
	o.mu.Lock()
	// Apply the target index NOW (finishRun has already killed the OLD session
	// and emitted the closed event with the OLD StepIdx — Copilot #355/#357).
	// Capture the values under the lock to pass to doLaunch, so the unlocked
	// slow launch never reads run.StepIdx/Loop that a concurrent Advance could
	// rewrite (tri-review 2 HIGH).
	nextIdx := run.pendingTargetIdx
	// A LOOP-BACK (target earlier than the current step) re-runs a step that has
	// already executed, so bump the loop counter to give the new agent a UNIQUE
	// session name (muster/<bead>/<step>/<loop+1>) — otherwise a repeated
	// loop-back to the same step would collide with the prior session name if a
	// Kill ever failed (tri-review 4). An ADVANCE moves to a fresh step, loop 0.
	nextLoop := 0
	if nextIdx < run.StepIdx {
		nextLoop = run.Loop + 1
	}
	run.StepIdx = nextIdx
	run.Loop = nextLoop

	// Reset session-local fields so doLaunch can set fresh ones. finishRun
	// already killed the old session, closed the pipe, and returned, so no
	// concurrent reader holds Session/Pane/pipe/cancel.
	//
	// pendingAdvance is deliberately LEFT TRUE here (cleared by doLaunch once the
	// new watcher is up, or in the error path below): between this unlock and
	// doLaunch setting run.cancel, run.cancel is nil, and if pendingAdvance were
	// already false a concurrent Advance would pass its guards, capture the nil
	// cancel (a no-op), and corrupt the transition (tri-review 3 HIGH #1).
	// State is guaranteed StepActive here (finishRun skips the state flip while
	// pendingAdvance is true); this write is defensive, not load-bearing.
	run.State = core.StepActive
	run.Session = ""
	run.Pane = ""
	run.pipe = nil
	run.cancel = nil
	o.mu.Unlock()

	// Determine per-step permission mode from the chain profile.
	var stepPM core.PermissionMode
	if run.Chain != nil && nextIdx < len(*run.Chain) {
		stepPM = (*run.Chain)[nextIdx].PermissionMode
	} else {
		stepPM = run.PermissionMode
	}

	// Build a synthetic DispatchRequest carrying the same identity. Chain is
	// omitted: doLaunch never reads req.Chain (only Dispatch's resolveChain does),
	// and the chain already lives on run.Chain (tri-review 4 — dead field).
	req := DispatchRequest{
		BeadID:         run.BeadID,
		BeadTitle:      run.BeadTitle,
		BeadDesc:       run.BeadDesc,
		Agent:          run.Agent,
		Mode:           run.Mode,
		PermissionMode: stepPM,
	}

	_, err := o.doLaunch(context.Background(), run, req, stepPM, nextIdx, nextLoop)
	if err != nil {
		// doLaunch failed; clear the transition flag (the next step never
		// started), mark the run failed, free the scheduler slot (finishRun
		// skipped onRunEnd because pendingAdvance was true), and evict.
		o.mu.Lock()
		run.pendingAdvance = false
		run.State = core.StepFailed
		nextRun := o.sched.onRunEnd(run.BeadID)
		if nextRun != nil {
			nextRun.State = core.StepActive
			// launching sentinel: blocks Advance/LoopBack until doLaunch arms
			// run.cancel, closing the nil-cancel window (tri-review 6).
			nextRun.pendingAdvance = true
		}
		o.mu.Unlock()
		// Emit run.failed so subscribers know this step transition did not complete.
		// step.advanced/step.loopedback was emitted before doLaunch; this closes the loop.
		if o.publish != nil {
			o.publish(ws.Frame{
				Type:    ws.EventRunFailed,
				BeadID:  run.BeadID,
				StepIdx: intPtr(nextIdx),
				Reason:  err.Error(),
			})
		}
		if nextRun != nil {
			go o.launchAdmittedRun(nextRun)
		}
		o.scheduleRunEviction(run)
	}
}
