package health

// HealthzResponse is the body returned by GET /api/v1/healthz.
type HealthzResponse struct {
	OK bool `json:"ok"`
}

// OrchestratorStatusResponse is the body returned by GET /api/v1/orchestrator/status.
type OrchestratorStatusResponse struct {
	Build         string `json:"build"`
	SchemaVersion int    `json:"schemaVersion"`
	BeadsVersion  string `json:"beadsVersion"`
	Online        bool   `json:"online"`
	ServerTime    string `json:"serverTime"`
}
