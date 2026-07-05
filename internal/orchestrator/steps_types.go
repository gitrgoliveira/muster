package orchestrator

import "github.com/gitrgoliveira/muster/internal/core"

// StepProfile describes a single step in a multi-step chain.
// PromptRef is stored but resolves to the M2 bead prompt in M4 (real assembly
// is M6); it is intentionally left unused by the current dispatch path.
type StepProfile struct {
	Name           string
	PermissionMode core.PermissionMode
	PromptRef      string // logical prompt identifier; resolved in M6
}

// StepChain is an ordered slice of step profiles that describes the full
// pipeline a bead runs through. A nil or empty chain means the bead uses the
// single default step (M2 behaviour preserved).
type StepChain []StepProfile
