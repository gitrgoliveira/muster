package orchestrator

import (
	"context"
	"time"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/tmux"
	"github.com/gitrgoliveira/muster/internal/ws"
)

// defaultOrphanGrace is how long to wait before killing a session with no
// corresponding bead in the run registry after restart.
const defaultOrphanGrace = 30 * time.Second

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
		o.recoverSession(ctx, sess)
	}
}

func (o *Orchestrator) recoverSession(ctx context.Context, sess tmux.Session) {
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

	// Recreate a Run for this session and resume streaming.
	runCtx, runCancel := context.WithCancel(ctx)
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

// killOrphanAfterGrace kills a tmux session after a grace period if it still
// has no corresponding run. Used for sessions discovered at startup that
// belong to no known bead.
func (o *Orchestrator) killOrphanAfterGrace(ctx context.Context, sess tmux.Session, grace time.Duration) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(grace):
	}

	o.mu.RLock()
	existing := o.runs[sess.BeadID]
	o.mu.RUnlock()
	if existing == nil {
		_ = o.transport.Kill(sess.Name)
	}
}
