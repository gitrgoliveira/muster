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
	BeadID  string  `json:"beadID,omitempty"`
	StepIdx *int    `json:"stepIdx,omitempty"` // *int so the valid M2 value 0 isn't dropped by omitempty (M1 frames leave it nil)
	Seq     *uint64 `json:"seq,omitempty"`     // *uint64 (like StepIdx): set on every runlog.line frame, nil on others. seq is 1-based (runlogStreamer uses Add(1)); the pointer makes "present vs absent" explicit rather than depending on the value never being 0, and keeps the shared Frame struct consistent with the *int fields.
	Data    string  `json:"data,omitempty"`    // base64-encoded raw pane bytes (terminal output is not guaranteed UTF-8)

	// M2: tmux.session.opened / tmux.session.closed
	Session  string `json:"session,omitempty"`
	ExitCode *int   `json:"exitCode,omitempty"` // on closed; nil on opened
}

// ClientFrame is the only accepted client-to-server frame shape.
type ClientFrame struct {
	Type string `json:"type"` // only "ping" handled in M0
}
