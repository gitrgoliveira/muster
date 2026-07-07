package ws

import "testing"

// TestEventTypes_AdditiveContract freezes the WS event-type wire values. M0–M5
// values MUST NOT change (Principle V / SC-008); M6 only adds new ones. If this
// test fails, a milestone changed an existing event type — a breaking change.
func TestEventTypes_AdditiveContract(t *testing.T) {
	// M0–M5 (frozen).
	frozen := map[EventType]string{
		EventHello:             "hello",
		EventBeadCreated:       "bead.created",
		EventBeadUpdated:       "bead.updated",
		EventBeadMoved:         "bead.moved",
		EventBeadDeleted:       "bead.deleted",
		EventCommentAdded:      "comment.added",
		EventPong:              "pong",
		EventRunlogLine:        "runlog.line",
		EventTmuxOpened:        "tmux.session.opened",
		EventTmuxClosed:        "tmux.session.closed",
		EventDispatchQueued:    "dispatch.queued",
		EventDispatchAdmitted:  "dispatch.admitted",
		EventStepAdvanced:      "step.advanced",
		EventStepLoopedBack:    "step.loopedback",
		EventWorktreeFinalized: "worktree.finalized",
		EventWorktreePushed:    "worktree.pushed",
		EventWorktreeRemoved:   "worktree.removed",
		EventRunQuota:          "run.quota",
		EventRunFailed:         "run.failed",
	}
	for ev, want := range frozen {
		if string(ev) != want {
			t.Errorf("event type changed: got %q want %q (BREAKING — Principle V)", ev, want)
		}
	}

	// M6 additions (additive only).
	if EventConstitutionChanged != "constitution.changed" {
		t.Errorf("EventConstitutionChanged = %q", EventConstitutionChanged)
	}
	if EventRunlogWarning != "runlog.warning" {
		t.Errorf("EventRunlogWarning = %q", EventRunlogWarning)
	}
}
