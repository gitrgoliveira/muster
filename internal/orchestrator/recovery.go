package orchestrator

import (
	"context"
	"log/slog"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/tmux"
	"github.com/gitrgoliveira/muster/internal/ws"
)

// RecoverSessions scans existing muster tmux sessions and re-attaches streaming.
// For each surviving session it: validates the bead ID; kills sessions whose
// pane is already dead; and re-registers + resumes streaming for live ones.
//
// NOTE: it does NOT verify the bead still exists or apply a grace period —
// killing orphaned-but-live sessions whose bead was deleted is a tracked
// follow-up (the orchestrator has no bead-existence lookup). Only already-dead
// sessions are reaped here.
//
// Concurrency precondition: recoverSession checks o.runs for an existing entry
// (RLock) and later registers a new Run (separate Lock) as two distinct
// critical sections, not one atomic check-and-register. This is safe only
// because RecoverSessions runs once at startup, before the HTTP server
// accepts requests — Dispatch cannot observe or race the gap. Do not call
// RecoverSessions concurrently with request handling.
//
// Called once at startup (wired into main.go).
func (o *Orchestrator) RecoverSessions(ctx context.Context) {
	// Note: transport.List() is NOT bounded by ctx — the tmux Manager interface
	// takes no context, so this initial scan runs to completion regardless of
	// ctx. ctx only gates the per-session loop below (between recoverSession
	// calls). In practice List() is a single fast `tmux list-sessions`.
	sessions, err := o.transport.List()
	if err != nil {
		// RealManager.List already maps the common "no server running" case to an
		// empty list (nil, nil), so an error reaching here is a real failure
		// (tmux missing, list-sessions broke). Recovery is best-effort, so we
		// still proceed without it — but log, so an operator can tell "nothing to
		// recover" apart from "recovery couldn't run".
		slog.Warn("recovery: transport.List failed; skipping session recovery", "err", err)
		return
	}

	for _, sess := range sessions {
		// Allow aborting a long scan between sessions, but a recovered run's own
		// lifetime is NOT tied to this ctx (see recoverSession) — runs must
		// survive shutdown.
		if ctx.Err() != nil {
			return
		}
		o.recoverSession(sess)
	}
}

func (o *Orchestrator) recoverSession(sess tmux.Session) {
	beadID := sess.BeadID
	sessionName := sess.Name

	// Security: the bead ID comes from a tmux session name, which is
	// user-controllable. Reject anything that doesn't look like a real bead ID
	// and kill the stray session rather than registering it as a run. Uses the
	// same canonical validator as the HTTP handler's request-path check
	// (core.ValidBeadID) so the two can never disagree on what a bead ID is.
	if !core.ValidBeadID(beadID) {
		slog.Warn("recovery: killing session with invalid bead ID", "session", sessionName, "beadID", beadID)
		_ = o.transport.Kill(sessionName)
		return
	}

	// Security: the step/loop indices are parsed from the user-controllable
	// session name. Negative values and non-zero Loop are treated as malformed
	// (not real muster runs) and are killed to prevent DoS wedging.
	//
	// M4 T053a relaxed guard: a non-negative StepIdx (≥0) is now accepted and
	// re-registered so that a live multi-step build/review agent (StepIdx 1, 2,
	// …) survives a muster restart. The chain is unknown at recovery time — it
	// was in-memory only — so the recovered run is a single-step run pinned at
	// the recovered StepIdx. Advance/LoopBack refuse to move it until the bead
	// is re-dispatched (which re-supplies the chain); this is the safe, honest
	// consequence of chain-unknown-at-recovery (see research.md R9).
	//
	// Loop is treated symmetrically with StepIdx: a non-negative Loop is a valid
	// (if currently rare — muster resets Loop to 0 on loop-back) session-name
	// component and is re-registered so a live agent survives a restart, rather
	// than being killed. Only genuinely malformed *negative* indices are killed
	// (they cannot be produced by a real muster session name).
	//
	// Upper sanity bound: StepIdx > 4096 is rejected as malformed (no real chain
	// has more than a few thousand steps; this prevents absurdly large values from
	// a tampered session name from wedging the recovered run).
	const maxRecoveryStepIdx = 4096
	if sess.StepIdx < 0 || sess.Loop < 0 || sess.StepIdx > maxRecoveryStepIdx {
		slog.Warn("recovery: killing session with malformed step/loop indices",
			"session", sessionName, "beadID", beadID, "stepIdx", sess.StepIdx, "loop", sess.Loop)
		_ = o.transport.Kill(sessionName)
		return
	}

	// Check if we already have a run for this bead (e.g., dispatched before recovery).
	o.mu.RLock()
	existing := o.runs[beadID]
	o.mu.RUnlock()

	if existing != nil {
		// Already tracked — nothing to do.
		return
	}

	// Check if the pane is already dead.
	_, dead, err := o.transport.DeadStatus(sessionName)
	if err != nil {
		// Session may have already been cleaned up. Skip.
		return
	}
	if dead {
		// Dead session — clean up immediately.
		_ = o.transport.Kill(sessionName)
		return
	}

	// Recreate a Run for this session and resume streaming. Root the run in
	// Background (NOT the recovery scan ctx) so it survives muster shutdown,
	// exactly like Dispatch does (FR-018). run.cancel cancels the watcher
	// context; it is wired for a future cancel path but is not yet reachable
	// externally (M2 has no cancel endpoint — see the run-cancel follow-up).
	//
	// M4 T053: Chain is NOT persisted (Constitution II). The recovered run is
	// reconstructed as a single-step run pinned at sess.StepIdx with no chain.
	// Advance/LoopBack refuse to move it (ErrStepOutOfRange when Chain==nil)
	// until the bead is re-dispatched with a new chain. See research.md R9.
	runCtx, runCancel := context.WithCancel(context.Background())
	run := &Run{
		BeadID:    beadID,
		StepIdx:   sess.StepIdx,
		Loop:      sess.Loop,
		Session:   sessionName,
		Pane:      sess.Pane,
		State:     core.StepActive,
		StartedAt: sess.StartedAt,
		cancel:    runCancel,
		// Chain is intentionally nil: chain-unknown-at-recovery (see R9).
	}

	o.mu.Lock()
	o.registerRun(run)
	// M4 T053c: register into the scheduler active set so capacity accounting is
	// correct. Transient over-capacity is allowed (drains as recovered runs end).
	o.sched.recoverActive(beadID)
	o.mu.Unlock()

	// Re-attach pipe for streaming. Set before watchRun starts so finishRun can
	// close it (frees the FIFO/temp dir). Assign under o.mu, matching Dispatch's
	// post-Spawn field writes: the run is already registered above (visible to
	// GetRun, which snapshots the whole struct under RLock), so an unlocked
	// write here would race that read. watchRun (launched below) reads run.pipe
	// only after this point, so its later read is safely ordered.
	pipeReader, pipeErr := o.transport.Pipe(sessionName)
	if pipeErr == nil && pipeReader != nil {
		o.mu.Lock()
		run.pipe = pipeReader
		o.mu.Unlock()
		streamer := &runlogStreamer{
			beadID:  beadID,
			stepIdx: sess.StepIdx,
			publish: o.publish,
		}
		go streamer.stream(pipeReader)
	}

	// Resume exit watcher.
	go o.watchRun(runCtx, run)

	// Emit tmux.session.opened to signal the resumed session.
	if o.publish != nil {
		o.publish(ws.Frame{
			Type:    ws.EventTmuxOpened,
			BeadID:  beadID,
			StepIdx: intPtr(sess.StepIdx),
			Session: sessionName,
		})
	}
}
