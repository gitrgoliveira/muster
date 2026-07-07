package orchestrator

import (
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/core"
)

// fakeConstitution is a swappable ConstitutionProvider for assembly tests.
type fakeConstitution struct {
	md string
	v  int
}

func (f *fakeConstitution) Snapshot() (string, int) { return f.md, f.v }

func TestAssembly_ReflectsConstitutionVersion_NextDispatch(t *testing.T) {
	o := &Orchestrator{constitution: &fakeConstitution{md: "# v1 rules", v: 1}}
	req := DispatchRequest{BeadID: "b-1", BeadTitle: "T", BeadDesc: "D", Agent: core.AgentID("claude")}

	p1 := o.assemblePrompt(nil, req, core.ModeAgent, 0, nil)
	if !strings.Contains(p1, "Constitution (v1):\n# v1 rules\n") {
		t.Fatalf("prompt should carry v1 constitution:\n%s", p1)
	}

	// Operator PUTs a new constitution — the provider now returns v2.
	o.constitution = &fakeConstitution{md: "# v2 rules", v: 2}

	p2 := o.assemblePrompt(nil, req, core.ModeAgent, 0, nil)
	if !strings.Contains(p2, "Constitution (v2):\n# v2 rules\n") {
		t.Fatalf("next dispatch should carry v2 constitution:\n%s", p2)
	}

	// A prompt already produced for a running step is a value — it is not
	// retroactively altered by the PUT (FR-009).
	if strings.Contains(p1, "v2") {
		t.Fatal("previously-assembled prompt must not change after a PUT")
	}
}

func TestAssembly_NilConstitutionProvider_V0(t *testing.T) {
	o := &Orchestrator{} // no provider wired
	req := DispatchRequest{BeadID: "b-1", BeadTitle: "T", BeadDesc: "D", Agent: core.AgentID("claude")}
	got := o.assemblePrompt(nil, req, core.ModeAgent, 0, nil)
	if !strings.Contains(got, "Constitution (v0):\n") {
		t.Fatalf("nil provider should yield the v0 default:\n%s", got)
	}
}
