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
// It marks the run with pendingAdvance and pendingAdvanceNextIdx, then cancels
// the run's watcher goroutine. finishRun (called from watchRun) detects the
// pending advance and calls relaunchNextStep instead of evicting.
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
	if run.StepIdx+1 >= len(chain) {
		o.mu.Unlock()
		return fmt.Errorf("%w: bead %q is already at the last step (%d)", ErrStepOutOfRange, beadID, run.StepIdx)
	}
	// Re-entrancy guard: a transition is already in flight (finishRun has not
	// yet run relaunchNextStep). A second Advance would advance StepIdx again
	// before the first relaunch, skipping a step (tri-review #5).
	if run.pendingAdvance {
		o.mu.Unlock()
		return fmt.Errorf("%w: bead %q already has a step transition in progress", ErrStepOutOfRange, beadID)
	}

	nextIdx := run.StepIdx + 1
	chainLen := len(chain)
	// Set the interlock flag AND advance StepIdx now (synchronously) under
	// the lock, so subsequent Advance/LoopBack calls see the new index and the
	// pendingAdvance guard immediately. relaunchNextStep reads run.StepIdx.
	// Clear Session so observers can tell that the old session is gone and the
	// new one is not yet established (avoids seeing stale step-0 session
	// name while StepIdx is already 1).
	run.pendingAdvance = true
	run.StepIdx = nextIdx
	run.Loop = 0     // reset loop counter for the new step
	run.Session = "" // clear now so GetRun observers don't see stale step-0 name
	// while StepIdx is already nextIdx. Safe: all reads of
	// run.Session now go through o.mu.RLock() (finishRun,
	// watchRun DeadStatus poll).

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
// It marks the run with pendingAdvance and pendingAdvanceNextIdx, then cancels
// the run's watcher goroutine. finishRun detects the pending advance and calls
// relaunchNextStep instead of evicting.
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
	if toIdx >= run.StepIdx {
		o.mu.Unlock()
		return fmt.Errorf("%w: toIdx %d must be < current stepIdx %d", ErrStepOutOfRange, toIdx, run.StepIdx)
	}
	// No separate "toIdx >= len(chain)" check needed: the invariant StepIdx <
	// len(chain) (upheld by Advance) plus toIdx < StepIdx implies toIdx is in
	// range (tri-review #8, dead code removed).
	// Re-entrancy guard (tri-review #5), same as Advance.
	if run.pendingAdvance {
		o.mu.Unlock()
		return fmt.Errorf("%w: bead %q already has a step transition in progress", ErrStepOutOfRange, beadID)
	}
	chainLen := len(*run.Chain)

	// Set the interlock flag AND update StepIdx synchronously (same rationale as
	// Advance). Clear Session for the same reason as Advance (see above).
	run.pendingAdvance = true
	run.StepIdx = toIdx
	run.Loop = 0     // reset loop counter for the new step
	run.Session = "" // clear now; safe because all readers use o.mu.RLock()
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
	// StepIdx is already the target index (Advance/LoopBack set it synchronously),
	// so it IS the next step to launch (tri-review #6: no separate field needed).
	// Capture StepIdx/Loop under the lock to pass to doLaunch — reading them
	// unlocked during the slow launch would race a concurrent Advance that
	// arrives after we clear pendingAdvance below (tri-review 2 HIGH).
	nextIdx := run.StepIdx
	nextLoop := run.Loop

	// Clear the interlock flag and reset session-local fields so doLaunch
	// can set fresh ones.
	run.State = core.StepActive // semantic pre-set; doLaunch will confirm
	run.pendingAdvance = false
	// Session already cleared by Advance/LoopBack and not re-set since; Pane/
	// pipe/cancel are immutable after launch — finishRun already read them
	// (Kill/pipe.Close) and returned, so no concurrent reader holds them.
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

	// Build a synthetic DispatchRequest carrying the same identity.
	req := DispatchRequest{
		BeadID:         run.BeadID,
		BeadTitle:      run.BeadTitle,
		BeadDesc:       run.BeadDesc,
		Agent:          run.Agent,
		Mode:           run.Mode,
		PermissionMode: stepPM,
		Chain:          run.Chain,
	}

	_, err := o.doLaunch(context.Background(), run, req, stepPM, nextIdx, nextLoop)
	if err != nil {
		// doLaunch failed; mark the run as failed, free the scheduler slot
		// (finishRun skipped onRunEnd because pendingAdvance was true), and
		// evict normally.
		o.mu.Lock()
		run.State = core.StepFailed
		nextRun := o.sched.onRunEnd(run.BeadID)
		if nextRun != nil {
			nextRun.State = core.StepActive
		}
		o.mu.Unlock()
		if nextRun != nil {
			go o.launchAdmittedRun(nextRun)
		}
		o.scheduleRunEviction(run)
	}
}
