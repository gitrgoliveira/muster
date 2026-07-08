package orchestrator

import (
	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/skills"
)

// ConstitutionProvider supplies the current constitution to prompt assembly.
// Implemented by the M6 constitution service. Snapshot returns the markdown and
// its monotonic version together (an atomic snapshot — no torn read of markdown
// from one version and the number from another).
type ConstitutionProvider interface {
	Snapshot() (markdown string, version int)
}

// SkillProvider resolves a set of skill ids into the concrete skills to merge
// into the assembled prompt. Implemented by the M6 skill registry. It returns
// the resolved skills (in a stable order) and the ids it could not resolve
// (unknown/deleted) so assembly can emit a non-blocking warning per FR-020 —
// never a silent drop.
type SkillProvider interface {
	Resolve(ids []string) (resolved []skills.Skill, unresolved []string)
}

// PrimedMemoriesProvider returns the memory snapshot primed for a bead (via
// POST /memories/prime), folded into the bead's next assembled prompt. Priming
// is ONE-SHOT: Consume both reads and clears the snapshot, so it feeds exactly
// the bead's next dispatch and is not re-injected into later dispatches (FR-024
// "next dispatch"). Empty when nothing was primed. Implemented by the M6
// memories service.
type PrimedMemoriesProvider interface {
	ConsumePrimedMemories(beadID string) map[string]string
}

// constitutionSnapshot reads the constitution provider nil-safely: a nil
// provider (fresh install / not wired) yields the empty default at version 0.
func (o *Orchestrator) constitutionSnapshot() (string, int) {
	if o.constitution == nil {
		return "", 0
	}
	return o.constitution.Snapshot()
}

// resolveSkills reads the skill provider nil-safely: a nil provider or an empty
// id list yields an empty loadout.
func (o *Orchestrator) resolveSkills(ids []string) (resolved []skills.Skill, unresolved []string) {
	if o.skills == nil || len(ids) == 0 {
		return nil, nil
	}
	return o.skills.Resolve(ids)
}

// stepSummary retains one finished step's bounded runlog tail and its terminal
// status, for M6 earlier-step summaries (FR-004). Guarded by Orchestrator.mu.
type stepSummary struct {
	Tail   string          // last ~8KiB/100 lines of the step's pane output
	Status core.StepStatus // done | failed (set in finishRun)
}
