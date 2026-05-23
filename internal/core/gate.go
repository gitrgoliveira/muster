package core

type GateKind string

const (
	GateHuman  GateKind = "human"
	GateTimer  GateKind = "timer"
	GateGitHub GateKind = "github"
)

type GateStatus string

const (
	GateWaiting GateStatus = "waiting"
	GatePassed  GateStatus = "passed"
	GateFailed  GateStatus = "failed"
)

type Gate struct {
	Kind   GateKind   `json:"kind"`
	Label  string     `json:"label"`
	Status GateStatus `json:"status"`
}
