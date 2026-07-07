package orchestrator

import (
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/core"
)

type fakePrimed struct {
	m map[string]string
}

func (f fakePrimed) PrimedMemories(beadID string) map[string]string { return f.m }

func TestAssembly_PrimedMemoriesSection(t *testing.T) {
	o := &Orchestrator{primedMemoriesProvider: fakePrimed{m: map[string]string{
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
	o := &Orchestrator{primedMemoriesProvider: fakePrimed{m: nil}}
	req := DispatchRequest{BeadID: "b-1", BeadTitle: "T", BeadDesc: "D", Agent: core.AgentID("claude")}
	if got := o.assemblePrompt(nil, req, core.ModeAgent, 0, nil); strings.Contains(got, "Primed memories:") {
		t.Fatalf("un-primed bead should omit the section:\n%s", got)
	}
}
