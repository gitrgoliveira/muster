package core

type Provider struct {
	ID       string `json:"id"` // "claude"|"gemini"|"opencode"|"codex"
	Name     string `json:"name"`
	Mono     string `json:"mono"`     // 2-letter mono code (CC/GM/OC/CX)
	Color    string `json:"color"`    // hex
	Parallel int    `json:"parallel"` // max parallel sessions
	Kind     string `json:"kind"`     // "cli"|"sdk"
	Plan     string `json:"plan,omitempty"`
}

type Capacity struct {
	Agent   AgentID `json:"agent"`
	Running int     `json:"running"`
	Queued  int     `json:"queued"`
	Limit   int     `json:"limit"`
}

// DoltStatus is the canonical shape; api/health's OrchestratorStatusResponse references it.
type DoltStatus struct {
	Branch   string `json:"branch"`
	Remote   string `json:"remote"`
	Ahead    int    `json:"ahead"`
	Behind   int    `json:"behind"`
	LastSync string `json:"lastSync"`
	Status   string `json:"status"`
	Server   string `json:"server"`
	Port     int    `json:"port"`
	Writers  int    `json:"writers"`
}
