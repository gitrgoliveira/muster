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
	EventDispatchQueued    EventType = "dispatch.queued"    // bead enqueued waiting for scheduler capacity
	EventDispatchAdmitted  EventType = "dispatch.admitted"  // bead admitted from queue and run started
	EventStepAdvanced      EventType = "step.advanced"      // operator advanced the chain to the next step
	EventStepLoopedBack    EventType = "step.loopedback"    // operator looped the chain back to a prior step
	EventWorktreeFinalized EventType = "worktree.finalized" // agent worktree committed (finalize complete)
	EventWorktreePushed    EventType = "worktree.pushed"    // agent worktree branch pushed to remote
	EventWorktreeRemoved   EventType = "worktree.removed"   // agent worktree directory removed
	EventRunQuota          EventType = "run.quota"          // token/cost quota captured at run end
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
}

// ClientFrame is the only accepted client-to-server frame shape.
type ClientFrame struct {
	Type string `json:"type"` // only "ping" handled in M0
}
