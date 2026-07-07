package orchestrator

import (
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/core"
)

func TestPriorSummaries_OrderedIncludesFailed(t *testing.T) {
	o := &Orchestrator{}
	run := &Run{}

	// Step 0 completed done; step 1 completed failed; both before the current
	// step 2 that is about to assemble.
	o.recordStepTail(run, 0, "working...\nplan complete: wrote plan.md")
	o.recordStepStatus(run, 0, core.StepDone)
	o.recordStepTail(run, 1, "building...\nbuild FAILED: compile error")
	o.recordStepStatus(run, 1, core.StepFailed)

	got := o.priorSummaries(run, 2)
	if len(got) != 2 {
		t.Fatalf("want 2 prior summaries, got %d: %+v", len(got), got)
	}
	if got[0].Idx != 0 || got[0].Status != core.StepDone || got[0].Line != "plan complete: wrote plan.md" {
		t.Errorf("step0 summary wrong: %+v", got[0])
	}
	// A failed prior step is included and labelled, never omitted (FR-004 edge case).
	if got[1].Idx != 1 || got[1].Status != core.StepFailed || got[1].Line != "build FAILED: compile error" {
		t.Errorf("step1 (failed) summary wrong: %+v", got[1])
	}
}

func TestPriorSummaries_ExcludesCurrentAndLater(t *testing.T) {
	o := &Orchestrator{}
	run := &Run{}
	o.recordStepTail(run, 0, "s0")
	o.recordStepStatus(run, 0, core.StepDone)
	o.recordStepTail(run, 1, "s1")
	o.recordStepStatus(run, 1, core.StepDone)

	// Assembling step 1 must see only step 0, not step 1 (itself) or later.
	got := o.priorSummaries(run, 1)
	if len(got) != 1 || got[0].Idx != 0 {
		t.Fatalf("assembling step 1 should see only step 0, got %+v", got)
	}
}

func TestBuildAssembledPrompt_RendersEarlierSummaries(t *testing.T) {
	in := assemblyInput{
		StepIdx:    1,
		StepCount:  2,
		Mode:       core.ModeBuild,
		Provider:   "claude",
		BeadID:     "b-1",
		Title:      "T",
		Desc:       "D",
		StepPrompt: "build",
		Prior: []priorStepSummary{
			{Idx: 0, Status: core.StepDone, Line: "plan complete: wrote plan.md"},
		},
	}
	got := buildAssembledPrompt(in)
	if !strings.Contains(got, "Earlier-step summaries:\n  step 1 (done): plan complete: wrote plan.md\n") {
		t.Fatalf("earlier-step section missing/wrong:\n%s", got)
	}
}

func TestAssemblePrompt_Step1IncludesStep0Summary(t *testing.T) {
	// End-to-end through the method (nil-safe providers), with a 2-step chain.
	o := &Orchestrator{}
	chain := StepChain{{Name: "plan"}, {Name: "build"}}
	run := &Run{Chain: &chain}
	o.recordStepTail(run, 0, "...\nplan produced 3 tasks")
	o.recordStepStatus(run, 0, core.StepDone)

	req := DispatchRequest{BeadID: "b-1", BeadTitle: "T", BeadDesc: "D", Agent: core.AgentID("claude")}
	got := o.assemblePrompt(run, req, core.ModeBuild, 1, nil)

	if !strings.Contains(got, "# Step 2 of 2: build mode") {
		t.Errorf("missing step framing:\n%s", got)
	}
	if !strings.Contains(got, "step 1 (done): plan produced 3 tasks") {
		t.Errorf("step 1 prompt should include step 0 summary:\n%s", got)
	}
}
