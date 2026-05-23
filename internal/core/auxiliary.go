package core

type Acceptance struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

type NowPlaying struct {
	Action string         `json:"action"`
	Since  int            `json:"since"` // seconds elapsed
	Kind   NowPlayingKind `json:"kind"`
}

type Reviewer struct {
	Agent    AgentID `json:"agent"`
	Comments int     `json:"comments"`
}

type LogEntry struct {
	T    string  `json:"t"` // opaque timestamp string (e.g., "09:14:02" or RFC3339)
	Kind LogKind `json:"kind"`
	Msg  string  `json:"msg"`
}

type FileChange struct {
	Path   string     `json:"path"`
	Status FileStatus `json:"status"`
	Adds   int        `json:"adds"`
	Dels   int        `json:"dels"`
}
