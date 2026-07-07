package orchestrator

import (
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/skills"
)

func TestBuildAssembledPrompt_Full_ByteVerifiable(t *testing.T) {
	in := assemblyInput{
		ConstMarkdown: "# Rules\n- Run tests with -race.\n",
		ConstVersion:  2,
		StepIdx:       0,
		StepCount:     2,
		Mode:          core.ModePlan,
		Provider:      "claude",
		Skills:        []skills.Skill{{Name: "Repo Grep", PromptStub: "Search fast.\nmore detail"}},
		BeadID:        "muster-ep0",
		Title:         "M6",
		Desc:          "Do the thing.",
		StepPrompt:    "Plan it.",
	}
	want := "<system role=\"muster\">\n" +
		"Constitution (v2):\n" +
		"# Rules\n- Run tests with -race.\n" +
		"\n" +
		"# Step 1 of 2: plan mode\n" +
		"Provider: claude\n" +
		"Skills loaded:\n" +
		"  Repo Grep — Search fast.\n" +
		"Bead muster-ep0: M6\n" +
		"Acceptance criteria:\n" +
		"Do the thing.\n" +
		"</system>\n" +
		"<user>\n" +
		"Plan it.\n" +
		"</user>\n"
	if got := buildAssembledPrompt(in); got != want {
		t.Fatalf("assembled prompt mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestBuildAssembledPrompt_Degenerate_WellFormed(t *testing.T) {
	// Empty constitution, no skills, no prior steps, single step (FR-005).
	in := assemblyInput{
		StepIdx:    0,
		StepCount:  1,
		Mode:       core.ModeAgent,
		Provider:   "claude",
		BeadID:     "b-1",
		Title:      "T",
		Desc:       "D",
		StepPrompt: "go",
	}
	got := buildAssembledPrompt(in)
	for _, want := range []string{
		"<system role=\"muster\">\n",
		"Constitution (v0):\n",
		"# Step 1 of 1: agent mode\n",
		"Skills loaded:\n  (none)\n",
		"Bead b-1: T\n",
		"</system>\n<user>\ngo\n</user>\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("degenerate prompt missing %q\nfull:\n%s", want, got)
		}
	}
	// Must NOT include an earlier-step section when there are no prior steps.
	if strings.Contains(got, "Earlier-step summaries:") {
		t.Errorf("degenerate prompt should omit earlier-step section:\n%s", got)
	}
}

func TestOneLineSummary(t *testing.T) {
	if got := oneLineSummary("first\n\nlast non-blank\n\n"); got != "last non-blank" {
		t.Fatalf("oneLineSummary = %q", got)
	}
	if got := oneLineSummary("   \n  \n"); got != "" {
		t.Fatalf("all-blank should be empty, got %q", got)
	}
	long := strings.Repeat("x", summaryMaxChars+50)
	if got := oneLineSummary(long); len(got) != summaryMaxChars {
		t.Fatalf("summary should truncate to %d, got %d", summaryMaxChars, len(got))
	}
}
