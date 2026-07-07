package store

import "time"

// Issue is the M1 row type mapping 1:1 to issues.jsonl records and Dolt issues table.
type Issue struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	Status          string     `json:"status"`
	Priority        int        `json:"priority"`
	IssueType       string     `json:"issue_type"`
	Assignee        string     `json:"assignee"`
	Owner           string     `json:"owner"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	ClosedAt        *time.Time `json:"closed_at,omitempty"`
	CloseReason     string     `json:"close_reason,omitempty"`
	DependencyCount int        `json:"dependency_count"`
	DependentCount  int        `json:"dependent_count"`
	CommentCount    int        `json:"comment_count"`
	Notes           string     `json:"notes,omitempty"`

	// Labels are the bead's labels (M6). Not populated by the M1 read backends
	// (Dolt/JSONL do not read the labels table); it is filled at dispatch time
	// from `bd label list` so skill:<id> labels can be surfaced into the bead.
	Labels []string `json:"labels,omitempty"`
}
