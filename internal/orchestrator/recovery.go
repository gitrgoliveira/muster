package orchestrator

import (
	"context"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/tmux"
	"github.com/gitrgoliveira/muster/internal/ws"
)

// RecoverSessions scans existing muster tmux sessions and re-attaches streaming
// for those that correspond to active runs. Sessions with no matching bead are
// killed after the grace period.
//
// Called once at startup (T037 wires this into main.go).
func (o *Orchestrator) RecoverSessions(ctx context.Context) {
	sessions, err := o.transport.List()
	if err != nil {
		// Non-fatal: if tmux isn't running or has no sessions, proceed normally.
		return
	}

	for _, sess := range sessions {
		// Allow aborting a long scan, but a recovered run's own lifetime is NOT
		// tied to this ctx (see recoverSession) — runs must survive shutdown.
		if ctx.Err() != nil {
			return
		}
		o.recoverSession(sess)
	}
}

func (o *Orchestrator) recoverSession(sess tmux.Session) {
	beadID := sess.BeadID
	sessionName := sess.Name

	// Check if we already have a run for this bead (e.g., dispatched before recovery).
	o.mu.RLock()
	existing := o.runs[beadID]
	o.mu.RUnlock()

	if existing != nil {
		// Already tracked — nothing to do.
		return
	}

	// Check if the pane is already dead.
	code, dead, err := o.transport.DeadStatus(sessionName)
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
	// exactly like Dispatch does (FR-018). Explicit cancellation is still
	// possible via run.cancel.
	runCtx, runCancel := context.WithCancel(context.Background())
	run := &Run{
		BeadID:    beadID,
		StepIdx:   sess.StepIdx,
		Loop:      sess.Loop,
		Session:   sessionName,
		State:     core.StepActive,
		StartedAt: sess.StartedAt,
		cancel:    runCancel,
	}

	o.mu.Lock()
	o.registerRun(run)
	o.mu.Unlock()

	// Re-attach pipe for streaming.
	pipeReader, pipeErr := o.transport.Pipe(sessionName)
	if pipeErr == nil && pipeReader != nil {
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
			StepIdx: sess.StepIdx,
			Session: sessionName,
		})
	}

	_ = code // used to check dead above
}
