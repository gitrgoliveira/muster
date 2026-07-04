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

// VCSAvailability describes one VCS backend's availability at runtime.
type VCSAvailability struct {
	// Available is true when the VCS binary responds to `--version`.
	Available bool `json:"available"`
	// Version is the binary's version string (best-effort; empty when unavailable).
	Version string `json:"version,omitempty"`
}

// VCSStatus describes the availability of VCS backends at runtime (M3 addition).
type VCSStatus struct {
	// DefaultVCS is the configured default VCS ("git" or "jj").
	DefaultVCS string `json:"defaultVCS"`
	// Git is the git backend's availability.
	Git VCSAvailability `json:"git"`
	// JJ is the jj backend's availability.
	JJ VCSAvailability `json:"jj"`
}

// SchedulerSnapshotDTO is the JSON shape for the scheduler's current state.
// Used by both GET /orchestrator/status (embedded) and PUT /orchestrator/capacity (body).
type SchedulerSnapshotDTO struct {
	Capacity    int      `json:"capacity"`
	ActiveCount int      `json:"activeCount"`
	Waiting     []string `json:"waiting"`
}

// RunSummaryDTO is a per-run summary included in OrchestratorStatusResponse.
// It carries per-step chain progress (T047).
type RunSummaryDTO struct {
	// BeadID is the bead this run belongs to.
	BeadID string `json:"beadID"`
	// StepIdx is the current (or last completed) step index.
	StepIdx int `json:"stepIdx"`
	// ChainLen is the total number of steps in the chain. 0 means single-step (M2).
	ChainLen int `json:"chainLen"`
	// State is the run's current state: "pending", "active", "done", "failed".
	State string `json:"state"`
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

	// M4 additions (additive — all M0–M3 fields unchanged).
	// Capacity is the scheduler's maximum concurrency.
	Capacity int `json:"capacity"`
	// ActiveCount is the number of currently-running agent sessions.
	ActiveCount int `json:"activeCount"`
	// Waiting is the bead IDs in FIFO order waiting for a capacity slot.
	Waiting []string `json:"waiting"`
	// Runs is a per-run summary list including stepIdx and chainLen (T047).
	// Empty (not null) when no runs are tracked.
	Runs []RunSummaryDTO `json:"runs"`
}
