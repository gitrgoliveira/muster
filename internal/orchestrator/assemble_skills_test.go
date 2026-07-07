package orchestrator

import (
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/skills"
)

// fakeSkillProvider is a swappable SkillProvider for assembly tests.
type fakeSkillProvider struct {
	m map[string]skills.Skill
}

func (f fakeSkillProvider) Resolve(ids []string) (resolved []skills.Skill, unresolved []string) {
	for _, id := range ids {
		if s, ok := f.m[id]; ok {
			resolved = append(resolved, s)
		} else {
			unresolved = append(unresolved, id)
		}
	}
	return resolved, unresolved
}

func TestAssembly_SkillsLoadedSection(t *testing.T) {
	o := &Orchestrator{skills: fakeSkillProvider{m: map[string]skills.Skill{
		"repo-grep": {Name: "Repo Grep", PromptStub: "Search fast.\nmore detail"},
		"run-tests": {Name: "Run Tests", PromptStub: "Run go test."},
	}}}
	req := DispatchRequest{BeadID: "b-1", BeadTitle: "T", BeadDesc: "D", Agent: core.AgentID("claude")}

	got := o.assemblePrompt(nil, req, core.ModeAgent, 0, []string{"repo-grep", "run-tests"})
	want := "Skills loaded:\n  Repo Grep — Search fast.\n  Run Tests — Run go test.\n"
	if !strings.Contains(got, want) {
		t.Fatalf("Skills loaded section wrong:\n%s", got)
	}
}
