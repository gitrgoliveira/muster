package orchestrator

import (
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/core"
)

type fakePrimed struct {
	m     map[string]string
	calls int
}

func (f *fakePrimed) ConsumePrimedMemories(beadID string) map[string]string {
	f.calls++
	return f.m
}

func TestAssembly_PrimedMemoriesSection(t *testing.T) {
	o := &Orchestrator{primedMemoriesProvider: &fakePrimed{m: map[string]string{
		"zeta":  "last",
		"alpha": "first",
	}}}
	req := DispatchRequest{BeadID: "muster-ep0", BeadTitle: "T", BeadDesc: "D", Agent: core.AgentID("claude")}

	got := o.assemblePrompt(nil, req, core.ModeAgent, 0, nil)
	// Sorted by key for deterministic, byte-verifiable output.
	if !strings.Contains(got, "Primed memories:\n  alpha: first\n  zeta: last\n") {
		t.Fatalf("primed memories section wrong:\n%s", got)
	}
}

func TestAssembly_NoPrimedMemories_OmitsSection(t *testing.T) {
	o := &Orchestrator{primedMemoriesProvider: &fakePrimed{m: nil}}
	req := DispatchRequest{BeadID: "b-1", BeadTitle: "T", BeadDesc: "D", Agent: core.AgentID("claude")}
	if got := o.assemblePrompt(nil, req, core.ModeAgent, 0, nil); strings.Contains(got, "Primed memories:") {
		t.Fatalf("un-primed bead should omit the section:\n%s", got)
	}
}

// One-shot: within a single dispatch (Run), primed memories are consumed ONCE
// and cached, so every step assembles the same set from a single provider call —
// the snapshot is not re-consumed per step.
func TestAssembly_PrimedMemories_ConsumedOncePerRun(t *testing.T) {
	fp := &fakePrimed{m: map[string]string{"alpha": "first"}}
	o := &Orchestrator{primedMemoriesProvider: fp}
	run := &Run{}
	req := DispatchRequest{BeadID: "muster-ep0", BeadTitle: "T", BeadDesc: "D", Agent: core.AgentID("claude")}

	for step := range 3 {
		got := o.assemblePrompt(run, req, core.ModeAgent, step, nil)
		if !strings.Contains(got, "Primed memories:\n  alpha: first\n") {
			t.Fatalf("step %d missing primed section:\n%s", step, got)
		}
	}
	if fp.calls != 1 {
		t.Fatalf("provider consumed %d times across a run, want exactly 1 (one-shot cached)", fp.calls)
	}
}
