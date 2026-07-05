package ws_test

import (
	"testing"

	"github.com/gitrgoliveira/muster/internal/ws"
)

// ── T002: M4 EventType constants — no collision with M0–M3 types ─────────────

func TestEventTypeConstants_NoDuplicates(t *testing.T) {
	// Enumerate all known EventType constants (M0–M3 + M4 additions).
	// If any two constants share the same string value the table will have a
	// duplicate that causes the uniqueness check below to fail, catching
	// accidental collisions before they reach clients.
	all := []struct {
		name string
		val  ws.EventType
	}{
		// M0–M1 constants.
		{"EventHello", ws.EventHello},
		{"EventBeadCreated", ws.EventBeadCreated},
		{"EventBeadUpdated", ws.EventBeadUpdated},
		{"EventBeadMoved", ws.EventBeadMoved},
		{"EventBeadDeleted", ws.EventBeadDeleted},
		{"EventCommentAdded", ws.EventCommentAdded},
		{"EventPong", ws.EventPong},
		// M2 constants.
		{"EventRunlogLine", ws.EventRunlogLine},
		{"EventTmuxOpened", ws.EventTmuxOpened},
		{"EventTmuxClosed", ws.EventTmuxClosed},
		// M4 constants (additive).
		{"EventDispatchQueued", ws.EventDispatchQueued},
		{"EventDispatchAdmitted", ws.EventDispatchAdmitted},
		{"EventStepAdvanced", ws.EventStepAdvanced},
		{"EventStepLoopedBack", ws.EventStepLoopedBack},
		{"EventWorktreeFinalized", ws.EventWorktreeFinalized},
		{"EventWorktreePushed", ws.EventWorktreePushed},
		{"EventWorktreeRemoved", ws.EventWorktreeRemoved},
		{"EventRunQuota", ws.EventRunQuota},
		{"EventRunFailed", ws.EventRunFailed},
	}

	seen := make(map[ws.EventType]string, len(all))
	for _, entry := range all {
		if prev, ok := seen[entry.val]; ok {
			t.Errorf("EventType collision: %s and %s both equal %q", prev, entry.name, entry.val)
		}
		seen[entry.val] = entry.name
	}
}

// TestEventTypeConstants_Values asserts the M4 string values match the
// documented wire format in ws-events.md.
func TestEventTypeConstants_Values(t *testing.T) {
	tests := []struct {
		name string
		got  ws.EventType
		want ws.EventType
	}{
		{"dispatch.queued", ws.EventDispatchQueued, "dispatch.queued"},
		{"dispatch.admitted", ws.EventDispatchAdmitted, "dispatch.admitted"},
		{"step.advanced", ws.EventStepAdvanced, "step.advanced"},
		{"step.loopedback", ws.EventStepLoopedBack, "step.loopedback"},
		{"worktree.finalized", ws.EventWorktreeFinalized, "worktree.finalized"},
		{"worktree.pushed", ws.EventWorktreePushed, "worktree.pushed"},
		{"worktree.removed", ws.EventWorktreeRemoved, "worktree.removed"},
		{"run.quota", ws.EventRunQuota, "run.quota"},
		{"run.failed", ws.EventRunFailed, "run.failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("want %q got %q", tc.want, tc.got)
			}
		})
	}
}
