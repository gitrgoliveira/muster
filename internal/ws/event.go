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
}

// ClientFrame is the only accepted client-to-server frame shape.
type ClientFrame struct {
	Type string `json:"type"` // only "ping" handled in M0
}
