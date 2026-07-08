package orchestrator

import "github.com/gitrgoliveira/muster/internal/core"

// This file is the single source of truth for the per-mode default step prompts
// and synthesized-stage framing. Per handoff §9 the backend and UI are meant to
// "share the table"; there is no shared Go table today (the UI is an embedded
// static asset), so these strings are transcribed here from handoff §6.1/§9 and
// this table is authoritative. `resolvePrompt` uses it when a step supplies no
// explicit prompt override.

// defaultStepPrompts maps a workflow Mode to the default user-turn step prompt
// used when StepProfile.PromptRef resolves to no stored override.
var defaultStepPrompts = map[core.Mode]string{
	core.ModePlan:   "Analyze the task above and produce a concise implementation plan: the approach, the files you will change, and the tests you will write. Do not modify files yet.",
	core.ModeBuild:  "Implement the task above. Make the code changes and add or update tests that cover them, keeping the change focused on the acceptance criteria.",
	core.ModeAgent:  "Work the task above end to end: make the necessary code changes and tests to satisfy the acceptance criteria.",
	core.ModeReview: "Review the changes for this bead. Read-only: produce inline comments and a short summary of what is correct, risky, or missing. Do not modify files.",
	core.ModeApply:  "Apply the reviewed changes and requested fixes for the task above, then re-run the relevant tests.",
	core.ModeYolo:   "Complete the task above autonomously, making whatever code and test changes are required to satisfy the acceptance criteria.",
}

// defaultAgentPrompt is the fallback for any mode without an explicit entry.
const defaultAgentPrompt = "Work the task above to satisfy the acceptance criteria."

// defaultPromptFor returns the default step prompt for a mode. It never returns
// empty — an unknown mode falls back to the agent-style default so assembly
// always has a well-formed user turn (FR-002/FR-005).
func defaultPromptFor(mode core.Mode) string {
	if p, ok := defaultStepPrompts[mode]; ok {
		return p
	}
	return defaultAgentPrompt
}

// synthesizedStages are workflow-stage modes that have no native claude
// permission flag; for these the dispatcher prepends a <system role="muster-stage">
// framing block (handoff §6.1). Native modes (plan, agent/default) return "".
var synthesizedStages = map[core.Mode]string{
	core.ModeReview: "This step is a code review. Read-only. Produce inline comments and a summary. Do not write files.",
	core.ModeBuild:  "This step is a build/implementation stage. Make the changes required by the step prompt below.",
}

// synthesizedStagePrefix returns the muster-stage framing text for a synthesized
// mode, or "" for a native mode. Assembly includes it inside the step-prompt
// section (additive layering, not a replacement of the constitution header).
func synthesizedStagePrefix(mode core.Mode) string {
	return synthesizedStages[mode]
}
