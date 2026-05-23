package core

type Step struct {
	Agent  AgentID    `json:"agent"`
	Mode   Mode       `json:"mode"`
	Skills []string   `json:"skills"`
	Status StepStatus `json:"status"`
	Note   string     `json:"note,omitempty"`
}

type SubBead struct {
	ID        string     `json:"id"` // "<parent>.N" form, e.g. "bd-a1f2.1"
	Title     string     `json:"title"`
	Status    StepStatus `json:"status"` // reuses pending|active|done
	Agent     AgentID    `json:"agent,omitempty"`
	AutoSplit bool       `json:"autoSplit,omitempty"`
}
