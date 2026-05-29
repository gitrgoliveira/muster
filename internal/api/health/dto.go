package health

// HealthzResponse is the body returned by GET /api/v1/healthz.
type HealthzResponse struct {
	OK bool `json:"ok"`
}

// AdapterInfo describes a registered agent adapter's availability state.
type AdapterInfo struct {
	ID        string `json:"id"`
	Version   string `json:"version,omitempty"`
	LoggedIn  bool   `json:"loggedIn"`
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
}
