package core

type HistoryEvent struct {
	At    string    `json:"at"`
	Kind  EventKind `json:"kind"`
	Actor string    `json:"actor"`
	Agent AgentID   `json:"agent,omitempty"`
	Note  string    `json:"note,omitempty"`
}
