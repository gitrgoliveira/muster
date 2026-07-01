package orchestrator

import (
	"context"
	"log/slog"
	"regexp"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/tmux"
	"github.com/gitrgoliveira/muster/internal/ws"
)

// beadIDPattern validates a bead ID parsed from a tmux session name before we
// trust it. Session names are user-controllable (the user can create arbitrary
// `muster/*` tmux sessions), so a malformed/hostile name must not be registered
// as a run. Format: lowercase prefix, hyphen, alphanumeric suffix (e.g. mp-abc).
var beadIDPattern = regexp.MustCompile(`^[a-z]+-[0-9a-z]+$`)

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

	// Security: the bead ID comes from a tmux session name, which is
	// user-controllable. Reject anything that doesn't look like a real bead ID
	// and kill the stray session rather than registering it as a run.
	if !beadIDPattern.MatchString(beadID) {
		slog.Warn("recovery: killing session with invalid bead ID", "session", sessionName, "beadID", beadID)
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
	}

	o.mu.Lock()
	o.registerRun(run)
	o.mu.Unlock()

	// Re-attach pipe for streaming. Set before watchRun starts so finishRun
	// can close it (frees the FIFO/temp dir); no race — the watcher goroutine is
	// launched after this assignment.
	pipeReader, pipeErr := o.transport.Pipe(sessionName)
	if pipeErr == nil && pipeReader != nil {
		run.pipe = pipeReader
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
