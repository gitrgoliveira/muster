package orchestrator

import (
	"strings"
	"sync"
	"testing"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/skills"
	"github.com/gitrgoliveira/muster/internal/ws"
)

func captureWarnings(o *Orchestrator) *[]ws.Frame {
	var mu sync.Mutex
	frames := &[]ws.Frame{}
	o.publish = func(f ws.Frame) {
		mu.Lock()
		*frames = append(*frames, f)
		mu.Unlock()
	}
	return frames
}

func TestAssembly_UnresolvableSkillWarnsNotBlocks(t *testing.T) {
	o := &Orchestrator{skills: fakeSkillProvider{m: map[string]skills.Skill{
		"repo-grep": {Name: "Repo Grep", PromptStub: "search"},
	}}}
	frames := captureWarnings(o)
	req := DispatchRequest{BeadID: "b-1", BeadTitle: "T", BeadDesc: "D", Agent: core.AgentID("claude")}

	// "does-not-exist" is unresolvable; assembly must still return a prompt.
	got := o.assemblePrompt(nil, req, core.ModeAgent, 0, []string{"repo-grep", "does-not-exist"})
	if got == "" {
		t.Fatal("assembly must not be blocked by an unresolvable skill")
	}
	// The resolved skill still appears.
	if !strings.Contains(got, "Repo Grep") {
		t.Errorf("resolved skill missing:\n%s", got)
	}
	// A runlog.warning naming the unresolved id was emitted (never silent).
	if !hasWarning(*frames, "does-not-exist") {
		t.Fatalf("expected a runlog.warning for the unresolved skill, got %+v", *frames)
	}
}

func TestAssembly_UnresolvedWarnDistinguishesInvalidFromNotFound(t *testing.T) {
	o := &Orchestrator{skills: fakeSkillProvider{m: map[string]skills.Skill{}}}
	frames := captureWarnings(o)
	req := DispatchRequest{BeadID: "b-1", BeadTitle: "T", BeadDesc: "D", Agent: core.AgentID("claude")}

	// "../evil" fails ValidateID (invalid); "ghost" is a well-formed id that
	// simply doesn't resolve (not found). The warnings must name the right reason.
	_ = o.assemblePrompt(nil, req, core.ModeAgent, 0, []string{"../evil", "ghost"})

	if !hasWarning(*frames, `"../evil" invalid`) {
		t.Errorf("invalid id should warn 'invalid', got %+v", *frames)
	}
	if !hasWarning(*frames, `"ghost" not found`) {
		t.Errorf("unknown id should warn 'not found', got %+v", *frames)
	}
}

func hasWarning(frames []ws.Frame, substr string) bool {
	for _, f := range frames {
		if f.Type == ws.EventRunlogWarning && strings.Contains(f.Reason, substr) {
			return true
		}
	}
	return false
}
