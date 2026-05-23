package core

// Bead is the primary domain entity. JSON tags follow the prototype's data.jsx
// shape verbatim — short names (`desc`, `vcs`) are deliberate to keep the
// UI's wire format unchanged from the mock.
type Bead struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	Desc         string         `json:"desc"`
	Type         BeadType       `json:"type"`
	Column       Column         `json:"column"`
	Priority     Priority       `json:"priority"`
	Labels       []string       `json:"labels"`
	VCS          VCS            `json:"vcs"`
	Branch       string         `json:"branch,omitempty"`
	Worktree     string         `json:"worktree,omitempty"`
	Ready        bool           `json:"ready"`
	Repo         string         `json:"repo"`
	Formula      string         `json:"formula,omitempty"`
	Skills       []string       `json:"skills"`
	Steps        []Step         `json:"steps"`
	SubBeads     []SubBead      `json:"subBeads"`
	Gates        []Gate         `json:"gates,omitempty"`
	History      []HistoryEvent `json:"history"`
	Acceptance   []Acceptance   `json:"acceptance"`
	TokensUsed   int            `json:"tokensUsed"`
	TokensBudget int            `json:"tokensBudget"`
	Estimate     Estimate       `json:"estimate"`
	Assignee     AgentID        `json:"assignee,omitempty"`
	Comments     int            `json:"comments"`
	Requeued     bool           `json:"requeued,omitempty"`
	NowPlaying   *NowPlaying    `json:"nowPlaying,omitempty"`
	Reviewer     *Reviewer      `json:"reviewer,omitempty"`
	CreatedAt    string         `json:"createdAt"`
	OpenedAt     string         `json:"openedAt"`
	ClosedAt     string         `json:"closedAt,omitempty"`
	LastActivity string         `json:"lastActivity"`
	Log          []LogEntry     `json:"log"`
	Files        []FileChange   `json:"files"`
	DiffPreview  string         `json:"diffPreview,omitempty"`
	Blocks       []string       `json:"blocks"`
	BlockedBy    []string       `json:"blockedBy"`
}
