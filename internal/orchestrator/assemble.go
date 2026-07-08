package orchestrator

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/skills"
)

// summaryMaxChars bounds a one-line earlier-step summary (FR-004).
const summaryMaxChars = 200

// assemblyInput is the fully-resolved set of inputs to the prompt template. It
// is deliberately explicit (no hidden reads) so buildAssembledPrompt is a pure,
// byte-verifiable function (SC-001).
type assemblyInput struct {
	ConstMarkdown string
	ConstVersion  int
	StepIdx       int // 0-based
	StepCount     int // total steps in the chain (>=1)
	Mode          core.Mode
	Provider      string
	Skills        []skills.Skill
	BeadID        string
	Title         string
	Desc          string
	Prior         []priorStepSummary // ordered by step index, ascending
	Primed        []primedKV         // primed memories, ordered by key
	StepPrompt    string             // the resolved user-turn (already includes any synthesized-stage prefix)
}

// primedKV is one primed memory in the assembled prompt.
type primedKV struct {
	Key   string
	Value string
}

// priorStepSummary is one earlier step's one-line summary for the assembled
// prompt.
type priorStepSummary struct {
	Idx    int
	Status core.StepStatus
	Line   string
}

// buildAssembledPrompt renders the handoff §9 template deterministically. Every
// branch is total (no panics) and the output is stable for stable inputs, so
// tests can assert it byte-for-byte.
func buildAssembledPrompt(in assemblyInput) string {
	var b strings.Builder
	b.WriteString("<system role=\"muster\">\n")

	// Constitution header — always carries the version (FR-007/US2 AS3), even
	// for the empty/v0 default (FR-005/FR-011).
	fmt.Fprintf(&b, "Constitution (v%d):\n", in.ConstVersion)
	if in.ConstMarkdown != "" {
		b.WriteString(in.ConstMarkdown)
		if !strings.HasSuffix(in.ConstMarkdown, "\n") {
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")

	// Step / provider framing.
	stepCount := in.StepCount
	if stepCount < 1 {
		stepCount = 1
	}
	fmt.Fprintf(&b, "# Step %d of %d: %s mode\n", in.StepIdx+1, stepCount, in.Mode)
	fmt.Fprintf(&b, "Provider: %s\n", in.Provider)

	// Skills loaded.
	b.WriteString("Skills loaded:\n")
	if len(in.Skills) == 0 {
		b.WriteString("  (none)\n")
	} else {
		for _, s := range in.Skills {
			if line := skills.PromptStubFirstLine(s); line != "" {
				fmt.Fprintf(&b, "  %s — %s\n", s.Name, line)
			} else {
				fmt.Fprintf(&b, "  %s\n", s.Name)
			}
		}
	}

	// Bead ticket.
	fmt.Fprintf(&b, "Bead %s: %s\n", in.BeadID, in.Title)
	b.WriteString("Acceptance criteria:\n")
	b.WriteString(in.Desc)
	if !strings.HasSuffix(in.Desc, "\n") {
		b.WriteString("\n")
	}

	// Earlier-step summaries (omit the section entirely when there are none).
	if len(in.Prior) > 0 {
		b.WriteString("Earlier-step summaries:\n")
		for _, p := range in.Prior {
			line := p.Line
			if line == "" {
				line = "(no output captured)"
			}
			fmt.Fprintf(&b, "  step %d (%s): %s\n", p.Idx+1, p.Status, line)
		}
	}

	// Primed memories (present only if /memories/prime ran for this bead).
	if len(in.Primed) > 0 {
		b.WriteString("Primed memories:\n")
		for _, m := range in.Primed {
			fmt.Fprintf(&b, "  %s: %s\n", m.Key, m.Value)
		}
	}

	b.WriteString("</system>\n")
	b.WriteString("<user>\n")
	b.WriteString(in.StepPrompt)
	if !strings.HasSuffix(in.StepPrompt, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("</user>\n")
	return b.String()
}

// ansiCSI matches ANSI/VT100 control sequences; the runlog tail is raw terminal
// output (escapes preserved by the streamer), so they are stripped before a line
// is injected into the assembled prompt.
var ansiCSI = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// oneLineSummary derives the FR-004 one-line summary from a step's retained
// runlog tail: the last non-blank line, ANSI-stripped, cleaned of control bytes,
// and truncated to summaryMaxChars RUNES (never splitting a multi-byte rune).
func oneLineSummary(tail string) string {
	last := ""
	for line := range strings.Lines(tail) {
		if t := strings.TrimSpace(cleanLine(line)); t != "" {
			last = t
		}
	}
	if r := []rune(last); len(r) > summaryMaxChars {
		last = string(r[:summaryMaxChars])
	}
	return last
}

// cleanLine removes ANSI escape sequences and remaining control characters (keeps
// tab as a space) so a summary line is printable text.
func cleanLine(s string) string {
	s = ansiCSI.ReplaceAllString(s, "")
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1 // drop other control chars
		}
		return r
	}, s)
}

// resolvePrompt turns a StepProfile.PromptRef + mode into the user-turn prompt.
// A non-empty PromptRef that names a stored override would be resolved here
// (US3+ store overrides); today PromptRef carries no override table, so it falls
// back to the per-mode default (FR-002). The synthesized-stage prefix (§6.1) is
// prepended for synthesized modes (additive layering).
func (o *Orchestrator) resolvePrompt(promptRef string, mode core.Mode) string {
	base := defaultPromptFor(mode)
	// A future stored-override table keyed by promptRef resolves here; until then
	// a non-empty ref that is not a known override is ignored in favour of the
	// mode default (never an empty user turn — FR-002 edge case).
	_ = promptRef
	if prefix := synthesizedStagePrefix(mode); prefix != "" {
		return prefix + "\n\n" + base
	}
	return base
}

// priorSummaries snapshots the one-line summaries of every finished step before
// stepIdx (done or failed — a failed step is included and labelled, never
// omitted; FR-004 edge case). Reads run.stepSummaries under the read lock.
func (o *Orchestrator) priorSummaries(run *Run, stepIdx int) []priorStepSummary {
	if run == nil {
		return nil
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	if len(run.stepSummaries) == 0 {
		return nil
	}
	var out []priorStepSummary
	for idx, s := range run.stepSummaries {
		if idx >= stepIdx || s == nil {
			continue
		}
		if s.Status != core.StepDone && s.Status != core.StepFailed {
			continue
		}
		out = append(out, priorStepSummary{Idx: idx, Status: s.Status, Line: oneLineSummary(s.Tail)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Idx < out[j].Idx })
	return out
}

// assemblePrompt gathers all inputs (nil-safe providers, chain framing, prior
// summaries) and renders the assembled prompt for one step launch. It replaces
// the M2 buildPrompt at doLaunch's single call site, so it covers step 0 and
// every advance/loop-back relaunch.
func (o *Orchestrator) assemblePrompt(run *Run, req DispatchRequest, mode core.Mode, stepIdx int, skillIDs []string) string {
	md, ver := o.constitutionSnapshot()
	resolved, unresolved := o.resolveSkills(skillIDs)
	// FR-020: an unresolvable skill id is skipped with a visible warning, never a
	// silent drop and never a blocked dispatch.
	for _, id := range unresolved {
		o.warn(req.BeadID, stepIdx, fmt.Sprintf("skill %q not found; skipped", id))
	}
	// FR-021: best-effort, non-blocking MCP-server verification.
	o.verifyMCPServers(req.BeadID, stepIdx, resolved)

	stepCount := 1
	promptRef := ""
	// The effective per-step mode drives both the step header and the default
	// step prompt. In a multi-step chain the run's adapter mode is constant
	// (e.g. agent), but each stage carries its own label (plan → build → …); a
	// StepProfile.Name that names a valid core.Mode is that stage, so use it
	// rather than the constant run mode. A non-mode label (or single-step run)
	// falls back to the passed-in mode.
	effectiveMode := mode
	if run != nil && run.Chain != nil {
		stepCount = len(*run.Chain)
		if stepIdx >= 0 && stepIdx < len(*run.Chain) {
			step := (*run.Chain)[stepIdx]
			promptRef = step.PromptRef
			if m := core.Mode(step.Name); m.Valid() {
				effectiveMode = m
			}
		}
	}

	return buildAssembledPrompt(assemblyInput{
		ConstMarkdown: md,
		ConstVersion:  ver,
		StepIdx:       stepIdx,
		StepCount:     stepCount,
		Mode:          effectiveMode,
		Provider:      string(req.Agent),
		Skills:        resolved,
		BeadID:        req.BeadID,
		Title:         req.BeadTitle,
		Desc:          req.BeadDesc,
		Prior:         o.priorSummaries(run, stepIdx),
		Primed:        o.primedMemories(run, req.BeadID),
		StepPrompt:    o.resolvePrompt(promptRef, effectiveMode),
	})
}

// primedMemories returns the bead's primed snapshot as a key-sorted slice for
// deterministic assembly (SC-001), nil-safely. It is ONE-SHOT per run: the
// snapshot is consumed (read + cleared) on the run's first assembly and cached
// on the run, so every step of THIS dispatch sees the same set while a later
// dispatch of the bead sees nothing unless re-primed (FR-024 "next dispatch").
// Guarded by o.mu (mutates run.primed/primedLoaded).
func (o *Orchestrator) primedMemories(run *Run, beadID string) []primedKV {
	if o.primedMemoriesProvider == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	// With a run, consume once and cache so every step of this dispatch sees the
	// same set. Without a run (degenerate/single-shot assembly), consume directly.
	if run != nil && run.primedLoaded {
		return run.primed
	}
	if run != nil {
		run.primedLoaded = true
	}
	m := o.primedMemoriesProvider.ConsumePrimedMemories(beadID)
	if len(m) == 0 {
		return nil
	}
	out := make([]primedKV, 0, len(m))
	for k, v := range m {
		out = append(out, primedKV{Key: k, Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	if run != nil {
		run.primed = out
	}
	return out
}

// recordStepTail stores a finished step's bounded runlog tail (called by the
// runlog streamer on EOF). Guarded by o.mu.
func (o *Orchestrator) recordStepTail(run *Run, idx int, tail string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	s := o.stepSummaryLocked(run, idx)
	s.Tail = tail
}

// recordStepStatus stores a finished step's terminal status (called by
// finishRun). Guarded by o.mu.
func (o *Orchestrator) recordStepStatus(run *Run, idx int, status core.StepStatus) {
	o.mu.Lock()
	defer o.mu.Unlock()
	s := o.stepSummaryLocked(run, idx)
	s.Status = status
}

// stepSummaryLocked returns (creating if needed) the stepSummary for idx. Caller
// must hold o.mu.
func (o *Orchestrator) stepSummaryLocked(run *Run, idx int) *stepSummary {
	if run.stepSummaries == nil {
		run.stepSummaries = make(map[int]*stepSummary)
	}
	s := run.stepSummaries[idx]
	if s == nil {
		s = &stepSummary{}
		run.stepSummaries[idx] = s
	}
	return s
}
