package orchestrator

import (
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/core"
)

func TestDefaultPromptFor_NonEmptyPerMode(t *testing.T) {
	for _, m := range []core.Mode{
		core.ModePlan, core.ModeBuild, core.ModeReview,
		core.ModeAgent, core.ModeApply, core.ModeYolo,
	} {
		if p := defaultPromptFor(m); strings.TrimSpace(p) == "" {
			t.Errorf("defaultPromptFor(%q) is empty", m)
		}
	}
	// Unknown mode falls back to a non-empty default (never an empty user turn).
	if p := defaultPromptFor(core.Mode("nonsense")); strings.TrimSpace(p) == "" {
		t.Error("unknown mode should fall back to a non-empty default")
	}
}

func TestResolvePrompt_DefaultAndSynthesizedPrefix(t *testing.T) {
	o := &Orchestrator{}

	// A native mode (plan) resolves to the plain default, no stage prefix.
	plan := o.resolvePrompt("", core.ModePlan)
	if plan != defaultPromptFor(core.ModePlan) {
		t.Fatalf("plan resolvePrompt = %q", plan)
	}

	// A synthesized mode (review) prepends the muster-stage framing.
	review := o.resolvePrompt("", core.ModeReview)
	if !strings.HasPrefix(review, synthesizedStagePrefix(core.ModeReview)) {
		t.Fatalf("review prompt should start with the stage prefix, got %q", review)
	}
	if !strings.Contains(review, defaultPromptFor(core.ModeReview)) {
		t.Fatalf("review prompt should still contain the mode default, got %q", review)
	}

	// An unknown PromptRef is ignored in favour of the default (never empty).
	if got := o.resolvePrompt("some-unknown-ref", core.ModeAgent); strings.TrimSpace(got) == "" {
		t.Fatal("unknown PromptRef must not yield an empty user turn")
	}
}
