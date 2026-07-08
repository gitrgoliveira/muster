package services

import (
	"context"
	"testing"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/store"
	"github.com/gitrgoliveira/muster/internal/store/bdshell"
)

func TestSplitSkillLabels(t *testing.T) {
	skillIDs, plain := SplitSkillLabels([]string{"skill:repo-grep", "area:core", "skill:", "skill:a:b", "p2"})
	if len(skillIDs) != 3 || skillIDs[0] != "repo-grep" || skillIDs[1] != "" || skillIDs[2] != "a:b" {
		t.Fatalf("skillIDs = %v (empty/malformed kept for the assembly layer to warn)", skillIDs)
	}
	if len(plain) != 2 || plain[0] != "area:core" || plain[1] != "p2" {
		t.Fatalf("plain = %v", plain)
	}
}

func TestIssueToBead_SplitsLabelsIntoSkills(t *testing.T) {
	issue := store.Issue{ID: "b-1", Status: "open", Labels: []string{"skill:repo-grep", "area:core"}}
	bead := IssueToBead(&issue, "repo")
	if len(bead.Skills) != 1 || bead.Skills[0] != "repo-grep" {
		t.Fatalf("bead.Skills = %v", bead.Skills)
	}
	if len(bead.Labels) != 1 || bead.Labels[0] != "area:core" {
		t.Fatalf("bead.Labels = %v", bead.Labels)
	}
}

// fakeLabelCLI implements CLIRunner + labelReader for resolveBeadSkills tests.
type fakeLabelCLI struct {
	labels []string
}

func (f fakeLabelCLI) Create(context.Context, bdshell.CreateInput) (store.Issue, error) {
	return store.Issue{}, nil
}
func (f fakeLabelCLI) Update(context.Context, string, bdshell.UpdatePatch) (store.Issue, error) {
	return store.Issue{}, nil
}
func (f fakeLabelCLI) Close(context.Context, string) (store.Issue, error) { return store.Issue{}, nil }
func (f fakeLabelCLI) Dispatch(context.Context, string) (store.Issue, error) {
	return store.Issue{}, nil
}
func (f fakeLabelCLI) AppendNote(context.Context, string, string) (store.Issue, error) {
	return store.Issue{}, nil
}
func (f fakeLabelCLI) Labels(context.Context, string) ([]string, error) { return f.labels, nil }

func TestResolveBeadSkills_UnionNotOverride(t *testing.T) {
	svc := &BeadService{cli: fakeLabelCLI{labels: []string{"skill:a", "area:core"}}}
	bead := &core.Bead{Skills: []string{"b", "a"}} // step/bead-level already carries b (and a again)

	got := svc.resolveBeadSkills(context.Background(), "b-1", bead, nil)
	// Union of label-derived {a} and bead {b,a}, de-duplicated => [a, b].
	if len(got) != 2 {
		t.Fatalf("resolveBeadSkills = %v, want union [a b]", got)
	}
	has := map[string]bool{}
	for _, s := range got {
		has[s] = true
	}
	if !has["a"] || !has["b"] {
		t.Fatalf("resolveBeadSkills = %v, want both a and b (additive union)", got)
	}
}

func TestResolveBeadSkills_PerDispatchOverrideUnioned(t *testing.T) {
	// FR-018: the per-dispatch step-level override is unioned additively on top
	// of the bead-level (label-derived) set, de-duplicated — never subtractive.
	svc := &BeadService{cli: fakeLabelCLI{labels: []string{"skill:a"}}}
	bead := &core.Bead{Skills: []string{"b"}}

	got := svc.resolveBeadSkills(context.Background(), "b-1", bead, []string{"c", "a"})
	has := map[string]bool{}
	for _, s := range got {
		has[s] = true
	}
	// label {a} ∪ bead {b} ∪ override {c,a} = {a,b,c}, deduped.
	if len(got) != 3 || !has["a"] || !has["b"] || !has["c"] {
		t.Fatalf("resolveBeadSkills = %v, want union {a,b,c}", got)
	}
}
