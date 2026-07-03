package health

// HealthzResponse is the body returned by GET /api/v1/healthz.
type HealthzResponse struct {
	OK bool `json:"ok"`
}

// AdapterInfo describes a registered agent adapter's availability state.
// Installed distinguishes "binary not on PATH" from "installed but not logged
// in" — without it, both collapse to loggedIn=false and a version-less entry.
type AdapterInfo struct {
	ID        string `json:"id"`
	Installed bool   `json:"installed"`
	Version   string `json:"version,omitempty"`
	LoggedIn  bool   `json:"loggedIn"`
}

// VCSStatus describes the availability of VCS backends at runtime (M3 addition).
type VCSStatus struct {
	// DefaultVCS is the configured default VCS ("git" or "jj").
	DefaultVCS string `json:"defaultVCS"`
	// Git is true when the git binary is available on PATH.
	Git bool `json:"git"`
	// JJ is true when the jj binary is available on PATH.
	JJ bool `json:"jj"`
	// JJVersion is the jj version string (best-effort; empty when JJ is false).
	JJVersion string `json:"jjVersion,omitempty"`
}

// OrchestratorStatusResponse is the body returned by GET /api/v1/orchestrator/status.
type OrchestratorStatusResponse struct {
	Build         string `json:"build"`
	SchemaVersion int    `json:"schemaVersion"`
	BeadsVersion  string `json:"beadsVersion"`
	Online        bool   `json:"online"`
	ServerTime    string `json:"serverTime"`
	BeadsDir      string `json:"beadsDir,omitempty"`
	DoltDatabase  string `json:"doltDatabase,omitempty"`
	DoltMode      string `json:"doltMode,omitempty"`
	ReadSource    string `json:"readSource,omitempty"`
	BdCLI         string `json:"bdCLI,omitempty"`
	ProjectID     string `json:"projectID,omitempty"`

	// M2 additions (FR-019: additive only).
	TmuxAvailable bool          `json:"tmuxAvailable"`
	TmuxVersion   string        `json:"tmuxVersion,omitempty"`
	RunningCount  int           `json:"runningCount"`
	Adapters      []AdapterInfo `json:"adapters,omitempty"`

	// M3 additions (FR-012: additive only — all M0/M1/M2 fields above are unchanged).
	// VCS describes VCS backend availability and configuration.
	VCS VCSStatus `json:"vcs"`
	// WorktreeCount is the number of per-bead worktree directories under
	// the configured --worktrees-dir.
	WorktreeCount int `json:"worktreeCount"`
}
