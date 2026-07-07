package ws

import "github.com/gitrgoliveira/muster/internal/core"

type EventType string

const (
	EventHello        EventType = "hello"
	EventBeadCreated  EventType = "bead.created"
	EventBeadUpdated  EventType = "bead.updated"
	EventBeadMoved    EventType = "bead.moved"
	EventBeadDeleted  EventType = "bead.deleted"
	EventCommentAdded EventType = "comment.added"
	EventPong         EventType = "pong"

	// M2 additions (additive; M1 events unchanged — FR-019).
	EventRunlogLine EventType = "runlog.line"         // agent pane output
	EventTmuxOpened EventType = "tmux.session.opened" // session spawned
	EventTmuxClosed EventType = "tmux.session.closed" // session ended

	// M4 additions (additive; M0–M3 events unchanged).
	EventDispatchQueued EventType = "dispatch.queued" // bead enqueued waiting for scheduler capacity

	// EventDispatchAdmitted signals that the bead's queued run has been ADMITTED
	// from the scheduler queue and its async launch is STARTING. It does NOT
	// guarantee the launch succeeded — a subsequent run.failed event reports any
	// async launch failure.
	EventDispatchAdmitted EventType = "dispatch.admitted"

	// EventStepAdvanced signals that the operator's Advance request was ACCEPTED
	// and the next step's async launch is STARTING. It does NOT guarantee the
	// relaunch succeeded — a subsequent run.failed event reports any failure.
	EventStepAdvanced EventType = "step.advanced"

	// EventStepLoopedBack signals that the operator's LoopBack request was ACCEPTED
	// and the targeted step's async relaunch is STARTING. It does NOT guarantee
	// the relaunch succeeded — a subsequent run.failed event reports any failure.
	EventStepLoopedBack EventType = "step.loopedback"

	EventWorktreeFinalized EventType = "worktree.finalized" // agent worktree committed (finalize complete)
	EventWorktreePushed    EventType = "worktree.pushed"    // agent worktree branch pushed to remote
	EventWorktreeRemoved   EventType = "worktree.removed"   // agent worktree directory removed
	EventRunQuota          EventType = "run.quota"          // token/cost quota captured at run end

	// EventRunFailed is emitted when an accepted run fails to LAUNCH asynchronously
	// (queued-run admission launch failure, or step-transition relaunch failure).
	// It is distinct from tmux.session.closed, which reports a session that
	// launched successfully and then exited. Clients that observed the preceding
	// dispatch.admitted / step.advanced / step.loopedback event (signalling the
	// transition/admission was ACCEPTED and launch was starting) should use this
	// event to learn that the launch ultimately failed.
	EventRunFailed EventType = "run.failed"

	// M6 additions (additive; M0–M5 events unchanged — Principle V).
	// EventConstitutionChanged is emitted after a successful PUT /api/v1/constitution;
	// it carries the new monotonic Version. The next dispatch (not any running step)
	// picks up the new constitution.
	EventConstitutionChanged EventType = "constitution.changed"

	// EventRunlogWarning is a non-blocking warning tied to a bead/step — e.g. an
	// unresolvable skill id or an MCP server named by a skill that is absent from
	// the agent's own config. Distinct from runlog.line (which is best-effort
	// dropped under backpressure) so a warning is never silently lost.
	EventRunlogWarning EventType = "runlog.warning"
)

// Frame is the server-to-client event envelope. Each event type populates a
// subset of fields; all others are zero (omitted from JSON).
type Frame struct {
	Type EventType `json:"type"`

	// hello fields
	Build         string `json:"build,omitempty"`
	SchemaVersion int    `json:"schemaVersion,omitempty"`
	ServerTime    string `json:"serverTime,omitempty"`
	BeadsVersion  string `json:"beadsVersion,omitempty"`

	// bead.created / bead.updated / bead.moved / comment.added
	Bead *core.Bead `json:"bead,omitempty"`

	// bead.moved / bead.deleted / comment.added
	ID string `json:"id,omitempty"`

	// bead.moved
	FromColumn core.Column `json:"fromColumn,omitempty"`
	ToColumn   core.Column `json:"toColumn,omitempty"`
	BeforeID   string      `json:"beforeID,omitempty"`

	// comment.added
	Event *core.HistoryEvent `json:"event,omitempty"`

	// pong
	At string `json:"at,omitempty"`

	// M2: runlog.line — agent pane output (raw bytes; ANSI preserved)
	BeadID  string `json:"beadID,omitempty"`
	StepIdx *int   `json:"stepIdx,omitempty"` // *int so the valid M2 value 0 isn't dropped by omitempty (M1 frames leave it nil)
	Seq     uint64 `json:"seq,omitempty"`     // set on every runlog.line frame, omitted elsewhere. seq is 1-based (runlogStreamer uses Add(1)), so it is never legitimately 0 — a plain value with omitempty drops it on non-runlog frames just like a pointer would, without the per-frame heap allocation a *uint64 forces on this hot path.
	Data    string `json:"data,omitempty"`    // base64-encoded raw pane bytes (terminal output is not guaranteed UTF-8)

	// M2: tmux.session.opened / tmux.session.closed
	Session  string `json:"session,omitempty"`
	ExitCode *int   `json:"exitCode,omitempty"` // on closed; nil on opened

	// M4: dispatch.queued / dispatch.admitted
	WaitingPos *int `json:"waitingPos,omitempty"` // 0-based FIFO position; dispatch.queued only

	// M4: step.advanced / step.loopedback
	ChainLen *int `json:"chainLen,omitempty"` // total number of steps in the chain

	// M4: worktree.finalized — whether a commit was actually created.
	// *bool (pointer) so false is not silently dropped by omitempty; nil on all
	// other frame types (same rationale as StepIdx/*int above).
	Committed *bool `json:"committed,omitempty"`

	// M4: worktree.pushed — the branch name and remote that were pushed to.
	Branch string `json:"branch,omitempty"`
	Remote string `json:"remote,omitempty"`

	// M4 US5: run.quota — best-effort token/cost record captured at run end.
	// Non-nil only on run.quota frames; nil on all other frame types.
	Quota *QuotaPayload `json:"quota,omitempty"`

	// run.failed — human-readable reason the launch failed (err.Error()).
	// runlog.warning also reuses Reason for the warning message.
	// Non-empty only on those frames; omitted on all other frame types.
	Reason string `json:"reason,omitempty"`

	// M6: constitution.changed — the new monotonic constitution version.
	// *int (pointer) so version 0 is not dropped by omitempty; nil on all other
	// frame types (same rationale as StepIdx/*int above).
	Version *int `json:"version,omitempty"`
}

// QuotaPayload is the run.quota event body.
// Known is false when no on-disk session record was found (best-effort advisory).
// CostUSD is 0 for interactive sessions (claude does not write costUSD to JSONL
// for non -p runs; see spike R8 in specs/005-m4-dispatcher/research.md).
type QuotaPayload struct {
	Known        bool    `json:"known"`
	InputTokens  int64   `json:"inputTokens"`
	OutputTokens int64   `json:"outputTokens"`
	CostUSD      float64 `json:"costUSD"`
}

// ClientFrame is the only accepted client-to-server frame shape.
type ClientFrame struct {
	Type string `json:"type"` // only "ping" handled in M0
}
