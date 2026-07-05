package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/store"
	"github.com/gitrgoliveira/muster/internal/store/bdshell"
	"github.com/gitrgoliveira/muster/internal/ws"
	"github.com/gitrgoliveira/muster/internal/wt"
)

func TestWrapCLIError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"exit 1 → validation", &bdshell.CLIError{ExitCode: 1, Stderr: "bad input"}, CodeCLIValidation},
		{"exit 2 → not found", &bdshell.CLIError{ExitCode: 2, Stderr: "missing"}, CodeNotFound},
		{"exit 3 → unavailable", &bdshell.CLIError{ExitCode: 3, Stderr: "down"}, CodeCLIUnavailable},
		{"exit 99 → internal", &bdshell.CLIError{ExitCode: 99, Stderr: "unknown"}, CodeInternal},
		{"deadline exceeded → timeout", context.DeadlineExceeded, CodeCLITimeout},
		{"generic error → internal", errors.New("oops"), CodeInternal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := wrapCLIError(tc.err)
			if got.Code != tc.wantCode {
				t.Errorf("wrapCLIError(%v) code = %q, want %q", tc.err, got.Code, tc.wantCode)
			}
		})
	}
}

func TestWrapOrchestratorError(t *testing.T) {
	// Sentinel→code mapping now lives in orchestrator.mapDispatchError (errors.Is)
	// and arrives here already typed. wrapOrchestratorError only: passes typed
	// ServiceErrors through (incl. wrapped), and maps anything else to Internal.
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"typed passthrough", &ServiceError{Code: CodeRunAlreadyActive, Message: "x"}, CodeRunAlreadyActive},
		{"wrapped typed passthrough", fmt.Errorf("ctx: %w", &ServiceError{Code: CodeUnmappedPrefix, Message: "y"}), CodeUnmappedPrefix},
		{"unknown → internal", errors.New("boom"), CodeInternal},
		{"nil → nil", nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := wrapOrchestratorError(tc.err)
			if tc.err == nil {
				if got != nil {
					t.Errorf("wrapOrchestratorError(nil) = %v, want nil", got)
				}
				return
			}
			if got.Code != tc.wantCode {
				t.Errorf("wrapOrchestratorError(%v) code = %q, want %q", tc.err, got.Code, tc.wantCode)
			}
		})
	}
}

func TestPatch_RejectsUnsupportedFields(t *testing.T) {
	backend := store.NewMemoryBackend(nil)
	svc := NewBeadService(backend, nil, func(_ ws.Frame) {})

	tests := []struct {
		name  string
		input PatchBeadInput
		want  string
	}{
		{
			"labels rejected",
			PatchBeadInput{Labels: &[]string{"foo"}},
			"labels patch not supported",
		},
		{
			"ready rejected",
			PatchBeadInput{Ready: boolPtr(true)},
			"ready patch not supported",
		},
		{
			"tokensBudget rejected",
			PatchBeadInput{TokensBudget: intPtr(100)},
			"tokensBudget patch not supported",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Patch(context.Background(), "mp-aaa", tc.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			se, ok := err.(*ServiceError)
			if !ok {
				t.Fatalf("expected ServiceError, got %T", err)
			}
			if se.Code != CodeInvalidRequest {
				t.Errorf("code = %q, want %q", se.Code, CodeInvalidRequest)
			}
			if !contains(se.Message, tc.want) {
				t.Errorf("message %q does not contain %q", se.Message, tc.want)
			}
		})
	}
}

func TestColumnToStatuses_BacklogIncludesScheduled(t *testing.T) {
	statuses := columnToStatuses("backlog")
	found := false
	for _, s := range statuses {
		if s == "scheduled" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("columnToStatuses(\"backlog\") = %v, want to include \"scheduled\"", statuses)
	}
}

func TestColumnToStatuses_RoundTrip(t *testing.T) {
	tests := []struct {
		status string
		column core.Column
	}{
		{"open", core.ColBacklog},
		{"scheduled", core.ColBacklog},
		{"blocked", core.ColBacklog},
		{"deferred", core.ColBacklog},
		{"in_progress", core.ColRunning},
		{"closed", core.ColDone},
		{"cancelled", core.ColDone},
		{"superseded", core.ColDone},
	}
	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			col := statusToColumn(tc.status)
			if col != tc.column {
				t.Errorf("statusToColumn(%q) = %q, want %q", tc.status, col, tc.column)
			}
			statuses := columnToStatuses(string(tc.column))
			found := false
			for _, s := range statuses {
				if s == tc.status {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("columnToStatuses(%q) = %v, does not include %q", tc.column, statuses, tc.status)
			}
		})
	}
}

// fakeCLI is a CLIRunner that returns a fixed issue for every write.
// dispatchCalls, if non-nil, is incremented on every Dispatch call — lets
// tests assert the bd CLI claim actually ran (not just an in-memory overlay).
// dispatchErr, if set, is returned by Dispatch instead of iss.
type fakeCLI struct {
	iss           store.Issue
	dispatchCalls *int
	dispatchErr   error
}

func (f fakeCLI) Create(context.Context, bdshell.CreateInput) (store.Issue, error) {
	return f.iss, nil
}
func (f fakeCLI) Update(context.Context, string, bdshell.UpdatePatch) (store.Issue, error) {
	return f.iss, nil
}
func (f fakeCLI) Close(context.Context, string) (store.Issue, error) { return f.iss, nil }
func (f fakeCLI) Dispatch(context.Context, string) (store.Issue, error) {
	if f.dispatchCalls != nil {
		(*f.dispatchCalls)++
	}
	if f.dispatchErr != nil {
		return store.Issue{}, f.dispatchErr
	}
	return f.iss, nil
}
func (f fakeCLI) AppendNote(context.Context, string, string) (store.Issue, error) {
	return f.iss, nil
}

func TestPublishOnWrite_RemoteMode(t *testing.T) {
	backend := store.NewMemoryBackend(nil)
	cli := fakeCLI{iss: store.Issue{ID: "mp-aaa", Title: "T", Status: "open", IssueType: "task"}}

	var frames []ws.Frame
	pub := func(f ws.Frame) { frames = append(frames, f) }
	svc := NewBeadServiceWithRepo(backend, cli, pub, "muster", true)

	if _, err := svc.Create(context.Background(), CreateBeadInput{Title: "T", Type: core.TypeTask, Priority: 2}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Patch(context.Background(), "mp-aaa", PatchBeadInput{Title: strPtr("T2")}); err != nil {
		t.Fatalf("Patch: %v", err)
	}

	if len(frames) != 2 {
		t.Fatalf("want 2 frames, got %d: %+v", len(frames), frames)
	}
	if frames[0].Type != ws.EventBeadCreated || frames[0].Bead == nil {
		t.Errorf("frame[0] = %+v, want bead.created with Bead", frames[0])
	}
	if frames[1].Type != ws.EventBeadUpdated || frames[1].Bead == nil {
		t.Errorf("frame[1] = %+v, want bead.updated with Bead", frames[1])
	}
}

func TestPublishOnWrite_EmbeddedModeSilent(t *testing.T) {
	backend := store.NewMemoryBackend(nil)
	cli := fakeCLI{iss: store.Issue{ID: "mp-aaa", Title: "T", Status: "open", IssueType: "task"}}

	var frames []ws.Frame
	pub := func(f ws.Frame) { frames = append(frames, f) }
	// publishOnWrite=false → watcher is the WS source, service must not publish.
	svc := NewBeadServiceWithRepo(backend, cli, pub, "muster", false)

	if _, err := svc.Create(context.Background(), CreateBeadInput{Title: "T", Type: core.TypeTask, Priority: 2}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(frames) != 0 {
		t.Errorf("embedded mode must not publish on write, got %d frames", len(frames))
	}
}

// fakeOrchestrator is an OrchestratorDispatcher that returns a fixed stub
// bead, mirroring what orchestrator.Dispatch actually returns (an in-memory
// stub, not a store write).
type fakeOrchestrator struct {
	dispatchCalled bool
	joined         bool // when true, Dispatch returns Joined:true (idempotent join)
	queued         bool // when true, Dispatch returns Queued:true (capacity-waiting)
	lastReq        OrchestratorDispatchRequest
}

func (f *fakeOrchestrator) Dispatch(_ context.Context, req OrchestratorDispatchRequest) (OrchestratorDispatchResult, error) {
	f.dispatchCalled = true
	f.lastReq = req
	return OrchestratorDispatchResult{
		Bead:   &core.Bead{ID: "mp-aaa", Column: core.ColRunning},
		Joined: f.joined,
		Queued: f.queued,
	}, nil
}

// ── Dispatch chain (per-dispatch step-chain override) ──────────────────

// TestDispatch_Chain_MissingPermissionMode verifies that an explicit chain
// with a step missing PermissionMode is rejected with CodeInvalidRequest
// before the orchestrator is ever called. FR-012a is explicit that
// per-step permission mode is never silently defaulted, so this must be a
// hard 400, not a fallback to DefaultPermissionMode.
func TestDispatch_Chain_MissingPermissionMode(t *testing.T) {
	backend := store.NewMemoryBackend(store.SeedIssues())
	orch := &fakeOrchestrator{}
	svc := NewBeadService(backend, nil, nil).WithOrchestrator(orch)

	_, err := svc.Dispatch(context.Background(), "mp-aaa", DispatchInput{
		Agent: core.AgentClaude,
		Mode:  core.ModeAgent,
		Chain: []ChainStepInput{
			{Name: "plan", PermissionMode: core.PermPlan},
			{Name: "build"}, // missing PermissionMode
		},
	})
	if err == nil {
		t.Fatal("Dispatch should reject a chain step with no PermissionMode")
	}
	var se *ServiceError
	if !errors.As(err, &se) {
		t.Fatalf("want *ServiceError, got %T: %v", err, err)
	}
	if se.Code != CodeInvalidRequest {
		t.Errorf("code = %q, want %q", se.Code, CodeInvalidRequest)
	}
	if orch.dispatchCalled {
		t.Error("orchestrator.Dispatch must not be called when chain validation fails")
	}
}

// TestDispatch_Chain_EmptyName verifies that an explicit chain with a step
// whose Name is empty is rejected with CodeInvalidRequest before the
// orchestrator is called.
func TestDispatch_Chain_EmptyName(t *testing.T) {
	backend := store.NewMemoryBackend(store.SeedIssues())
	orch := &fakeOrchestrator{}
	svc := NewBeadService(backend, nil, nil).WithOrchestrator(orch)

	_, err := svc.Dispatch(context.Background(), "mp-aaa", DispatchInput{
		Agent: core.AgentClaude,
		Mode:  core.ModeAgent,
		Chain: []ChainStepInput{
			{Name: "", PermissionMode: core.PermPlan},
		},
	})
	if err == nil {
		t.Fatal("Dispatch should reject a chain step with an empty Name")
	}
	var se *ServiceError
	if !errors.As(err, &se) {
		t.Fatalf("want *ServiceError, got %T: %v", err, err)
	}
	if se.Code != CodeInvalidRequest {
		t.Errorf("code = %q, want %q", se.Code, CodeInvalidRequest)
	}
	if orch.dispatchCalled {
		t.Error("orchestrator.Dispatch must not be called when chain validation fails")
	}
}

// TestDispatch_Chain_ForwardedVerbatim verifies that a valid chain is
// forwarded, in order and unmodified, into OrchestratorDispatchRequest.Chain
// — this is the wiring the rest of M4's step-chain feature (advance/loopback)
// depends on; without it resolveChain always sees nil and every dispatch is
// forced into the M2 single-step path.
func TestDispatch_Chain_ForwardedVerbatim(t *testing.T) {
	backend := store.NewMemoryBackend(store.SeedIssues())
	orch := &fakeOrchestrator{}
	svc := NewBeadService(backend, nil, nil).WithOrchestrator(orch)

	chain := []ChainStepInput{
		{Name: "plan", PermissionMode: core.PermPlan, PromptRef: "plan-ref"},
		{Name: "build", PermissionMode: core.PermAcceptEdits},
	}
	_, err := svc.Dispatch(context.Background(), "mp-aaa", DispatchInput{
		Agent: core.AgentClaude,
		Mode:  core.ModeAgent,
		Chain: chain,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !orch.dispatchCalled {
		t.Fatal("orchestrator.Dispatch was not called")
	}
	if len(orch.lastReq.Chain) != 2 {
		t.Fatalf("forwarded Chain length = %d, want 2", len(orch.lastReq.Chain))
	}
	if orch.lastReq.Chain[0] != chain[0] || orch.lastReq.Chain[1] != chain[1] {
		t.Errorf("forwarded Chain = %+v, want %+v", orch.lastReq.Chain, chain)
	}
}

// TestDispatch_OrchestratorPath_PersistsRunningState verifies that
// BeadService.Dispatch, when an orchestrator is wired in, ALSO persists the
// running transition via the bd CLI (bd update --claim) — not just an
// in-memory overlay on the orchestrator's own stub bead. Beads is the source
// of truth for issue state (constitution II; FR-002); without this, a
// subsequent GET would show the pre-dispatch column even though the agent is
// genuinely running.
func TestDispatch_OrchestratorPath_PersistsRunningState(t *testing.T) {
	backend := store.NewMemoryBackend(store.SeedIssues()) // mp-aaa starts "open"
	dispatchCalls := 0
	cli := fakeCLI{
		iss:           store.Issue{ID: "mp-aaa", Title: "Implement feature alpha", Status: "in_progress", IssueType: "feature"},
		dispatchCalls: &dispatchCalls,
	}
	orch := &fakeOrchestrator{}

	var frames []ws.Frame
	pub := func(f ws.Frame) { frames = append(frames, f) }
	svc := NewBeadServiceWithRepo(backend, cli, pub, "muster", true).WithOrchestrator(orch)

	got, err := svc.Dispatch(context.Background(), "mp-aaa", DispatchInput{Agent: core.AgentClaude, Mode: core.ModeAgent})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if !orch.dispatchCalled {
		t.Fatal("orchestrator.Dispatch was not called")
	}
	// The bd CLI claim must have actually run (persisting the transition to
	// the beads store) — not just relied on the orchestrator's in-memory
	// stub overlay.
	if dispatchCalls != 1 {
		t.Errorf("bd CLI Dispatch (claim) call count want 1 got %d", dispatchCalls)
	}
	if got.Bead.Column != core.ColRunning {
		t.Errorf("returned bead Column want running got %q", got.Bead.Column)
	}

	if len(frames) == 0 {
		t.Fatal("expected a bead.updated frame to be published")
	}
	last := frames[len(frames)-1]
	if last.Type != ws.EventBeadUpdated || last.Bead == nil {
		t.Fatalf("last frame = %+v, want bead.updated with Bead", last)
	}
	if last.Bead.Column != core.ColRunning {
		t.Errorf("published bead Column want running got %q", last.Bead.Column)
	}
}

// TestDispatch_OrchestratorPath_CLIUnavailable verifies the orchestrator
// launch still succeeds when the bd CLI isn't configured — persistence is
// best-effort (logged, not fatal), since the run is already genuinely active
// by the time the claim would run.
func TestDispatch_OrchestratorPath_CLIUnavailable(t *testing.T) {
	backend := store.NewMemoryBackend(store.SeedIssues())
	orch := &fakeOrchestrator{}
	svc := NewBeadService(backend, nil, nil).WithOrchestrator(orch)

	got, err := svc.Dispatch(context.Background(), "mp-aaa", DispatchInput{Agent: core.AgentClaude, Mode: core.ModeAgent})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !orch.dispatchCalled {
		t.Fatal("orchestrator.Dispatch was not called")
	}
	if got.Bead.Column != core.ColRunning {
		t.Errorf("returned bead Column want running got %q", got.Bead.Column)
	}
}

// TestDispatch_OrchestratorPath_CLIClaimFails verifies that a failed bd CLI
// claim does not fail the whole Dispatch call: the agent is already genuinely
// running by the time the claim would run (orchestrator.Dispatch already
// succeeded), so failing the request here would be misleading and would
// prompt a pointless retry that just hits ErrRunAlreadyActive.
func TestDispatch_OrchestratorPath_CLIClaimFails(t *testing.T) {
	backend := store.NewMemoryBackend(store.SeedIssues())
	cli := fakeCLI{dispatchErr: errors.New("bd: connection refused")}
	orch := &fakeOrchestrator{}
	svc := NewBeadServiceWithRepo(backend, cli, nil, "muster", true).WithOrchestrator(orch)

	got, err := svc.Dispatch(context.Background(), "mp-aaa", DispatchInput{Agent: core.AgentClaude, Mode: core.ModeAgent})
	if err != nil {
		t.Fatalf("Dispatch should still succeed when only the bd claim fails: %v", err)
	}
	if !orch.dispatchCalled {
		t.Fatal("orchestrator.Dispatch was not called")
	}
	if got.Bead.Column != core.ColRunning {
		t.Errorf("returned bead Column want running got %q", got.Bead.Column)
	}
}

// TestDispatch_OrchestratorPath_EmbeddedMode_ForcesPublishWhenNotPersisted
// verifies that in embedded mode (publishOnWrite=false), if the bd CLI claim
// did not happen (no CLI configured here), Dispatch still broadcasts a
// bead.updated frame directly. Without this, publishWrite's normal no-op gate
// (embedded mode relies on the file watcher instead) would leave every other
// connected client without any running-transition signal at all, since no
// real bd write occurred to trigger the watcher.
func TestDispatch_OrchestratorPath_EmbeddedMode_ForcesPublishWhenNotPersisted(t *testing.T) {
	backend := store.NewMemoryBackend(store.SeedIssues())
	orch := &fakeOrchestrator{}
	var frames []ws.Frame
	pub := func(f ws.Frame) { frames = append(frames, f) }
	// publishOnWrite=false mirrors embedded mode; nil cli means the claim can't run.
	svc := NewBeadServiceWithRepo(backend, nil, pub, "muster", false).WithOrchestrator(orch)

	got, err := svc.Dispatch(context.Background(), "mp-aaa", DispatchInput{Agent: core.AgentClaude, Mode: core.ModeAgent})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got.Bead.Column != core.ColRunning {
		t.Errorf("returned bead Column want running got %q", got.Bead.Column)
	}
	if len(frames) == 0 {
		t.Fatal("expected a forced bead.updated frame despite publishOnWrite=false, since no real bd write occurred")
	}
	last := frames[len(frames)-1]
	if last.Type != ws.EventBeadUpdated || last.Bead == nil || last.Bead.Column != core.ColRunning {
		t.Fatalf("last frame = %+v, want bead.updated with Column=running", last)
	}
}

// TestDispatch_OrchestratorPath_EmbeddedMode_SkipsForcedPublishWhenPersisted
// verifies the opposite: when the bd claim DOES succeed in embedded mode, the
// normal publishWrite no-op gate is left alone (the file watcher is the sole
// announcer of that real write) — Dispatch must not double-announce.
func TestDispatch_OrchestratorPath_EmbeddedMode_SkipsForcedPublishWhenPersisted(t *testing.T) {
	backend := store.NewMemoryBackend(store.SeedIssues())
	cli := fakeCLI{iss: store.Issue{ID: "mp-aaa", Title: "Implement feature alpha", Status: "in_progress", IssueType: "feature"}}
	orch := &fakeOrchestrator{}
	var frames []ws.Frame
	pub := func(f ws.Frame) { frames = append(frames, f) }
	svc := NewBeadServiceWithRepo(backend, cli, pub, "muster", false).WithOrchestrator(orch)

	if _, err := svc.Dispatch(context.Background(), "mp-aaa", DispatchInput{Agent: core.AgentClaude, Mode: core.ModeAgent}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(frames) != 0 {
		t.Fatalf("expected no forced publish when embedded-mode claim succeeded (file watcher owns the signal), got %d frames", len(frames))
	}
}

// TestSendKeys_NoAttacher verifies that SendKeys reports CodeAttachUnavailable
// (→ 501) when no SessionAttacher is wired in: the attach/send feature simply
// isn't available in this configuration — not the bd CLI being absent
// (BD_CLI_MISSING) nor a transient outage (BD_CLI_UNAVAILABLE/503).
func TestSendKeys_NoAttacher(t *testing.T) {
	backend := store.NewMemoryBackend(store.SeedIssues())
	svc := NewBeadService(backend, nil, nil)

	err := svc.SendKeys(context.Background(), "mp-aaa", 0, "y\n")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	se, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if se.Code != CodeAttachUnavailable {
		t.Errorf("code = %q, want %q", se.Code, CodeAttachUnavailable)
	}
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }
func intPtr(i int) *int       { return &i }

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || indexOf(s, substr) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// ── T009: OrchestratorDispatchResult service-layer mirror ────────────────────

// TestOrchestratorDispatchResult_ZeroValueSane verifies that the service-layer
// DispatchResult mirror has sane zero values and the expected fields, mirroring
// the OrchestratorDispatchRequest pattern.
func TestOrchestratorDispatchResult_ZeroValueSane(t *testing.T) {
	var r OrchestratorDispatchResult

	if r.Bead != nil {
		t.Errorf("OrchestratorDispatchResult.Bead zero value: want nil, got %v", r.Bead)
	}
	if r.Joined {
		t.Error("OrchestratorDispatchResult.Joined zero value: want false")
	}
	if r.Queued {
		t.Error("OrchestratorDispatchResult.Queued zero value: want false")
	}

	// Exercise the struct with non-zero values to confirm field names compile.
	b := &core.Bead{ID: "mp-001"}
	r2 := OrchestratorDispatchResult{Bead: b, Joined: true, Queued: false}
	if r2.Bead != b {
		t.Error("Bead field assignment failed")
	}
	if !r2.Joined {
		t.Error("Joined field assignment failed")
	}
}

// ── T003: M4 additive service error codes ────────────────────────────────────

// TestM4ErrorCodes verifies that CodeStepOutOfRange and CodeInvalidCapacity are
// distinct non-empty strings that do not collide with any existing code.
func TestM4ErrorCodes(t *testing.T) {
	existing := []string{
		CodeInvalidRequest,
		CodeInvalidState,
		CodeNotFound,
		CodeInternal,
		CodeCLIMissing,
		CodeCLIValidation,
		CodeCLIUnavailable,
		CodeCLITimeout,
		CodeRunAlreadyActive,
		CodeUnmappedPrefix,
		CodeAdapterNotFound,
		CodeAdapterNotInstalled,
		CodeAdapterNotLoggedIn,
		CodeAttachUnavailable,
		CodeWorktreeNotFound,
		CodeVCSUnavailable,
	}
	newCodes := []string{CodeStepOutOfRange, CodeInvalidCapacity}

	for _, code := range newCodes {
		if code == "" {
			t.Errorf("new code must not be empty")
		}
	}

	// Build a set of existing codes and check no collision.
	set := make(map[string]struct{}, len(existing))
	for _, c := range existing {
		set[c] = struct{}{}
	}
	for _, nc := range newCodes {
		if _, ok := set[nc]; ok {
			t.Errorf("new code %q collides with an existing code", nc)
		}
	}

	// The two new codes must themselves be distinct.
	if CodeStepOutOfRange == CodeInvalidCapacity {
		t.Errorf("CodeStepOutOfRange and CodeInvalidCapacity must be distinct, both are %q", CodeStepOutOfRange)
	}

	// Verify they form valid ServiceErrors.
	for _, code := range newCodes {
		se := &ServiceError{Code: code, Message: "test"}
		if se.Error() == "" {
			t.Errorf("ServiceError with code %q must have non-empty Error()", code)
		}
	}
}

// T018: SetCapacity and SchedulerSnapshot service-layer wiring tests.

// fakeSchedulerManager is a test double for SchedulerManager.
type fakeSchedulerManager struct {
	capacity    int
	activeCount int
	waiting     []string
	setErr      error // non-nil causes SetCapacity to fail
}

func (f *fakeSchedulerManager) SetCapacity(n int) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.capacity = n
	return nil
}

func (f *fakeSchedulerManager) SchedulerSnapshot() SchedulerSnapshot {
	return SchedulerSnapshot{
		Capacity:    f.capacity,
		ActiveCount: f.activeCount,
		Waiting:     f.waiting,
	}
}

func TestT018_SetCapacity_NoScheduler(t *testing.T) {
	svc := NewBeadService(nil, nil, nil)
	err := svc.SetCapacity(4)
	if err == nil {
		t.Fatal("expected error when scheduler not configured")
	}
	se, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("expected *ServiceError, got %T", err)
	}
	if se.Code != CodeInvalidCapacity {
		t.Errorf("code want %q got %q", CodeInvalidCapacity, se.Code)
	}
}

func TestT018_SetCapacity_ValidCapacity(t *testing.T) {
	fake := &fakeSchedulerManager{capacity: 2}
	svc := NewBeadService(nil, nil, nil).WithScheduler(fake)
	if err := svc.SetCapacity(5); err != nil {
		t.Fatalf("SetCapacity(5): %v", err)
	}
	if fake.capacity != 5 {
		t.Errorf("capacity want 5 got %d", fake.capacity)
	}
}

func TestT018_SetCapacity_InvalidCapacity(t *testing.T) {
	fake := &fakeSchedulerManager{setErr: fmt.Errorf("capacity must be > 0")}
	svc := NewBeadService(nil, nil, nil).WithScheduler(fake)
	err := svc.SetCapacity(0)
	if err == nil {
		t.Fatal("expected error for capacity 0")
	}
	se, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("expected *ServiceError, got %T", err)
	}
	if se.Code != CodeInvalidCapacity {
		t.Errorf("code want %q got %q", CodeInvalidCapacity, se.Code)
	}
}

// TestT018_SetCapacity_NoDoubleWrap verifies that when the SchedulerManager
// adapter returns a pre-wrapped *ServiceError (mimicking the orchestrator adapter
// returning a raw error that BeadService then wraps once), the final error
// message does not contain the code string twice. This guards against the
// double-wrapping anti-pattern where both the adapter and BeadService each wrap
// the same error in ServiceError{CodeInvalidCapacity}.
func TestT018_SetCapacity_NoDoubleWrap(t *testing.T) {
	// Simulate the adapter returning a pre-wrapped *ServiceError — this is what
	// the old (buggy) schedulerManagerAdapter did. BeadService.SetCapacity must
	// not wrap it a second time.
	preWrapped := &ServiceError{Code: CodeInvalidCapacity, Message: "capacity must be > 0"}
	fake := &fakeSchedulerManager{setErr: preWrapped}
	svc := NewBeadService(nil, nil, nil).WithScheduler(fake)
	err := svc.SetCapacity(0)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	// The code must not appear more than once in the message.
	if count := strings.Count(msg, CodeInvalidCapacity); count > 1 {
		t.Errorf("error message contains %q %d times (double-wrap): %s", CodeInvalidCapacity, count, msg)
	}
}

func TestT018_SchedulerSnapshot_NoScheduler(t *testing.T) {
	svc := NewBeadService(nil, nil, nil)
	snap := svc.SchedulerSnapshot()
	if snap.Capacity != 0 || snap.ActiveCount != 0 || len(snap.Waiting) != 0 {
		t.Errorf("zero-value snapshot expected when no scheduler; got %+v", snap)
	}
}

func TestT018_SchedulerSnapshot_WithScheduler(t *testing.T) {
	fake := &fakeSchedulerManager{
		capacity:    3,
		activeCount: 2,
		waiting:     []string{"bd-1", "bd-2"},
	}
	svc := NewBeadService(nil, nil, nil).WithScheduler(fake)
	snap := svc.SchedulerSnapshot()
	if snap.Capacity != 3 {
		t.Errorf("Capacity want 3 got %d", snap.Capacity)
	}
	if snap.ActiveCount != 2 {
		t.Errorf("ActiveCount want 2 got %d", snap.ActiveCount)
	}
	if len(snap.Waiting) != 2 || snap.Waiting[0] != "bd-1" {
		t.Errorf("Waiting want [bd-1 bd-2] got %v", snap.Waiting)
	}
}

func TestMapWorktreeReadError(t *testing.T) {
	// A REAL missing-binary error, wrapped the way the wt backend wraps exec
	// failures. In modern Go *exec.Error unwraps to exec.ErrNotFound, so this
	// must map to VCS_UNAVAILABLE (guards the round-2 exec.ErrNotFound branch).
	realMissing := fmt.Errorf("wt: start: %w", exec.Command("muster-nonexistent-binary-xyz-abc").Run())

	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"worktree not found", wt.ErrWorktreeNotFound, CodeWorktreeNotFound},
		{"vcs unavailable", wt.ErrVCSUnavailable, CodeVCSUnavailable},
		{"vcs unavailable wrapped", fmt.Errorf("wt jj: %w", wt.ErrVCSUnavailable), CodeVCSUnavailable},
		{"binary missing (exec.ErrNotFound)", fmt.Errorf("wt: start [git]: %w", exec.ErrNotFound), CodeVCSUnavailable},
		{"real missing binary (*exec.Error)", realMissing, CodeVCSUnavailable},
		{"generic error is internal", errors.New("boom"), CodeInternal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			se := mapWorktreeReadError(tc.err, "bd-1", "diff")
			if se == nil {
				t.Fatal("mapWorktreeReadError returned nil")
			}
			if se.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", se.Code, tc.wantCode)
			}
		})
	}
}

// ── T036a/T037: FinalizeWorktree, PushWorktree, RemoveWorktree service methods

// fakeWriteableWorktreeAccessor is a test double implementing WorktreeAccessor
// with the write-side methods added in M4 T036a.
type fakeWriteableWorktreeAccessor struct {
	// read-side state
	existingBeadID string
	runState       core.StepStatus // BeadRunState result

	// write-side capture
	finalizeBeadID string
	finalizeMsg    string
	finalizeErr    error

	pushBeadID string
	pushErr    error

	removeBeadID string
	removeErr    error
}

func (f *fakeWriteableWorktreeAccessor) WorktreeStatus(_ context.Context, beadID string) (wt.WorktreeStatus, error) {
	if beadID == f.existingBeadID {
		return wt.WorktreeStatus{Exists: true, Clean: true}, nil
	}
	return wt.WorktreeStatus{}, wt.ErrWorktreeNotFound
}
func (f *fakeWriteableWorktreeAccessor) DiffSummary(_ context.Context, _ string) ([]wt.FileChange, error) {
	return nil, nil
}
func (f *fakeWriteableWorktreeAccessor) Diff(_ context.Context, _, _ string) (io.ReadCloser, error) {
	return nil, nil
}
func (f *fakeWriteableWorktreeAccessor) DefaultVCS() string { return "git" }

func (f *fakeWriteableWorktreeAccessor) BeadRunState(beadID string) core.StepStatus {
	if beadID == f.existingBeadID {
		return f.runState
	}
	return ""
}

func (f *fakeWriteableWorktreeAccessor) Finalize(_ context.Context, beadID, message string) (bool, error) {
	f.finalizeBeadID = beadID
	f.finalizeMsg = message
	if f.finalizeErr != nil {
		return false, f.finalizeErr
	}
	return true, nil
}

func (f *fakeWriteableWorktreeAccessor) Push(_ context.Context, beadID, _ string) error {
	f.pushBeadID = beadID
	return f.pushErr
}

func (f *fakeWriteableWorktreeAccessor) Remove(_ context.Context, beadID string) error {
	f.removeBeadID = beadID
	return f.removeErr
}

// Compile assertion: fakeWriteableWorktreeAccessor must satisfy WorktreeAccessor.
var _ WorktreeAccessor = (*fakeWriteableWorktreeAccessor)(nil)

// newSvcWithFakeWT returns a BeadService with a MemoryBackend, the given
// worktreeAccessor, a pre-seeded bead, and a no-op publisher.
func newSvcWithFakeWT(beadID string, wa WorktreeAccessor) *BeadService {
	backend := store.NewMemoryBackend([]store.Issue{
		{ID: beadID, Title: "Test bead", Status: "open", IssueType: "task"},
	})
	svc := NewBeadService(backend, nil, func(ws.Frame) {})
	return svc.WithWorktreeAccessor(wa)
}

// TestT037_FinalizeWorktree_NoAccessor asserts VCS_UNAVAILABLE when no accessor.
func TestT037_FinalizeWorktree_NoAccessor(t *testing.T) {
	svc := NewBeadService(store.NewMemoryBackend(nil), nil, nil)
	_, err := svc.FinalizeWorktree(context.Background(), "bd-1", "msg")
	se := mustServiceError(t, err)
	if se.Code != CodeVCSUnavailable {
		t.Errorf("code want %q got %q", CodeVCSUnavailable, se.Code)
	}
}

// TestT037_FinalizeWorktree_RunActive asserts CodeRunAlreadyActive when run is active.
func TestT037_FinalizeWorktree_RunActive(t *testing.T) {
	wa := &fakeWriteableWorktreeAccessor{
		existingBeadID: "bd-1",
		runState:       core.StepActive,
	}
	svc := newSvcWithFakeWT("bd-1", wa)

	_, err := svc.FinalizeWorktree(context.Background(), "bd-1", "msg")
	se := mustServiceError(t, err)
	if se.Code != CodeRunAlreadyActive {
		t.Errorf("code want %q got %q", CodeRunAlreadyActive, se.Code)
	}
}

// TestT037_FinalizeWorktree_RunPending asserts CodeRunAlreadyActive when run is pending.
func TestT037_FinalizeWorktree_RunPending(t *testing.T) {
	wa := &fakeWriteableWorktreeAccessor{
		existingBeadID: "bd-1",
		runState:       core.StepPending,
	}
	svc := newSvcWithFakeWT("bd-1", wa)

	_, err := svc.FinalizeWorktree(context.Background(), "bd-1", "msg")
	se := mustServiceError(t, err)
	if se.Code != CodeRunAlreadyActive {
		t.Errorf("code want %q got %q", CodeRunAlreadyActive, se.Code)
	}
}

// TestT037_FinalizeWorktree_TerminalState succeeds when run is in terminal state.
func TestT037_FinalizeWorktree_TerminalState(t *testing.T) {
	for _, state := range []core.StepStatus{core.StepDone, core.StepFailed, ""} {
		t.Run(string(state)+"_or_none", func(t *testing.T) {
			wa := &fakeWriteableWorktreeAccessor{
				existingBeadID: "bd-1",
				runState:       state,
			}
			svc := newSvcWithFakeWT("bd-1", wa)

			if _, err := svc.FinalizeWorktree(context.Background(), "bd-1", "seal it"); err != nil {
				t.Fatalf("expected success for state=%q, got %v", state, err)
			}
			if wa.finalizeBeadID != "bd-1" {
				t.Errorf("Finalize not called with correct beadID: got %q", wa.finalizeBeadID)
			}
			if wa.finalizeMsg != "seal it" {
				t.Errorf("Finalize not called with correct message: got %q", wa.finalizeMsg)
			}
		})
	}
}

// TestT037_FinalizeWorktree_BackendError maps a backend error to CodeInternal.
func TestT037_FinalizeWorktree_BackendError(t *testing.T) {
	wa := &fakeWriteableWorktreeAccessor{
		existingBeadID: "bd-1",
		runState:       core.StepDone,
		finalizeErr:    errors.New("git commit failed"),
	}
	svc := newSvcWithFakeWT("bd-1", wa)

	_, err := svc.FinalizeWorktree(context.Background(), "bd-1", "msg")
	se := mustServiceError(t, err)
	if se.Code != CodeInternal {
		t.Errorf("code want %q got %q", CodeInternal, se.Code)
	}
}

// TestT037_FinalizeWorktree_VCSUnavailable maps ErrVCSUnavailable to CodeVCSUnavailable.
func TestT037_FinalizeWorktree_VCSUnavailable(t *testing.T) {
	wa := &fakeWriteableWorktreeAccessor{
		existingBeadID: "bd-1",
		runState:       core.StepDone,
		finalizeErr:    wt.ErrVCSUnavailable,
	}
	svc := newSvcWithFakeWT("bd-1", wa)

	_, err := svc.FinalizeWorktree(context.Background(), "bd-1", "msg")
	se := mustServiceError(t, err)
	if se.Code != CodeVCSUnavailable {
		t.Errorf("code want %q got %q", CodeVCSUnavailable, se.Code)
	}
}

// TestT037_FinalizeWorktree_NotFound when bead does not exist.
func TestT037_FinalizeWorktree_NotFound(t *testing.T) {
	wa := &fakeWriteableWorktreeAccessor{existingBeadID: "bd-other"}
	// seed bead "bd-other", try finalize "bd-missing"
	backend := store.NewMemoryBackend([]store.Issue{
		{ID: "bd-other", Title: "T", Status: "open", IssueType: "task"},
	})
	svc := NewBeadService(backend, nil, func(ws.Frame) {}).WithWorktreeAccessor(wa)

	_, err := svc.FinalizeWorktree(context.Background(), "bd-missing", "msg")
	se := mustServiceError(t, err)
	if se.Code != CodeNotFound {
		t.Errorf("code want %q got %q", CodeNotFound, se.Code)
	}
}

// ── FIX D: NUL byte in commit message → CodeInvalidRequest (not CodeInternal) ──

// TestT037_FinalizeWorktree_NULMessage verifies that a message containing a NUL
// byte returns CodeInvalidRequest before the accessor is called.
func TestT037_FinalizeWorktree_NULMessage(t *testing.T) {
	wa := &fakeWriteableWorktreeAccessor{
		existingBeadID: "bd-1",
		runState:       core.StepDone,
	}
	svc := newSvcWithFakeWT("bd-1", wa)

	_, err := svc.FinalizeWorktree(context.Background(), "bd-1", "bad\x00msg")
	se := mustServiceError(t, err)
	if se.Code != CodeInvalidRequest {
		t.Errorf("code want %q got %q", CodeInvalidRequest, se.Code)
	}
	// The accessor must NOT have been called.
	if wa.finalizeBeadID != "" {
		t.Errorf("accessor Finalize was called despite NUL in message (beadID=%q)", wa.finalizeBeadID)
	}
}

// TestT037_FinalizeWorktree_MultilineMessage verifies that a multiline commit
// message (legitimate) reaches the accessor without being rejected.
func TestT037_FinalizeWorktree_MultilineMessage(t *testing.T) {
	wa := &fakeWriteableWorktreeAccessor{
		existingBeadID: "bd-1",
		runState:       core.StepDone,
	}
	svc := newSvcWithFakeWT("bd-1", wa)

	msg := "subject line\n\nbody paragraph"
	if _, err := svc.FinalizeWorktree(context.Background(), "bd-1", msg); err != nil {
		t.Fatalf("FinalizeWorktree(multiline): unexpected error: %v", err)
	}
	if wa.finalizeMsg != msg {
		t.Errorf("accessor received message %q, want %q", wa.finalizeMsg, msg)
	}
}

// TestT037_PushWorktree_NoAccessor asserts VCS_UNAVAILABLE when no accessor.
func TestT037_PushWorktree_NoAccessor(t *testing.T) {
	svc := NewBeadService(store.NewMemoryBackend(nil), nil, nil)
	err := svc.PushWorktree(context.Background(), "bd-1", "")
	se := mustServiceError(t, err)
	if se.Code != CodeVCSUnavailable {
		t.Errorf("code want %q got %q", CodeVCSUnavailable, se.Code)
	}
}

// TestT037_PushWorktree_Success verifies Push delegates to backend.
func TestT037_PushWorktree_Success(t *testing.T) {
	wa := &fakeWriteableWorktreeAccessor{
		existingBeadID: "bd-1",
		runState:       core.StepDone,
	}
	svc := newSvcWithFakeWT("bd-1", wa)

	if err := svc.PushWorktree(context.Background(), "bd-1", ""); err != nil {
		t.Fatalf("PushWorktree: %v", err)
	}
	if wa.pushBeadID != "bd-1" {
		t.Errorf("Push not called with correct beadID: got %q", wa.pushBeadID)
	}
}

// TestT037_PushWorktree_BackendError maps a backend push error to CodeInternal.
func TestT037_PushWorktree_BackendError(t *testing.T) {
	wa := &fakeWriteableWorktreeAccessor{
		existingBeadID: "bd-1",
		runState:       core.StepDone,
		pushErr:        errors.New("authentication failed"),
	}
	svc := newSvcWithFakeWT("bd-1", wa)

	err := svc.PushWorktree(context.Background(), "bd-1", "")
	se := mustServiceError(t, err)
	if se.Code != CodeInternal {
		t.Errorf("code want %q got %q", CodeInternal, se.Code)
	}
}

// TestT037_RemoveWorktree_NoAccessor asserts VCS_UNAVAILABLE when no accessor.
func TestT037_RemoveWorktree_NoAccessor(t *testing.T) {
	svc := NewBeadService(store.NewMemoryBackend(nil), nil, nil)
	err := svc.RemoveWorktree(context.Background(), "bd-1")
	se := mustServiceError(t, err)
	if se.Code != CodeVCSUnavailable {
		t.Errorf("code want %q got %q", CodeVCSUnavailable, se.Code)
	}
}

// TestT037_RemoveWorktree_RunActive asserts CodeRunAlreadyActive when run is active.
func TestT037_RemoveWorktree_RunActive(t *testing.T) {
	wa := &fakeWriteableWorktreeAccessor{
		existingBeadID: "bd-1",
		runState:       core.StepActive,
	}
	svc := newSvcWithFakeWT("bd-1", wa)

	err := svc.RemoveWorktree(context.Background(), "bd-1")
	se := mustServiceError(t, err)
	if se.Code != CodeRunAlreadyActive {
		t.Errorf("code want %q got %q", CodeRunAlreadyActive, se.Code)
	}
}

// TestT037_RemoveWorktree_RunPending asserts CodeRunAlreadyActive when run is pending.
func TestT037_RemoveWorktree_RunPending(t *testing.T) {
	wa := &fakeWriteableWorktreeAccessor{
		existingBeadID: "bd-1",
		runState:       core.StepPending,
	}
	svc := newSvcWithFakeWT("bd-1", wa)

	err := svc.RemoveWorktree(context.Background(), "bd-1")
	se := mustServiceError(t, err)
	if se.Code != CodeRunAlreadyActive {
		t.Errorf("code want %q got %q", CodeRunAlreadyActive, se.Code)
	}
}

// TestT037_RemoveWorktree_TerminalState succeeds when run is in terminal state.
func TestT037_RemoveWorktree_TerminalState(t *testing.T) {
	for _, state := range []core.StepStatus{core.StepDone, core.StepFailed, ""} {
		t.Run(string(state)+"_or_none", func(t *testing.T) {
			wa := &fakeWriteableWorktreeAccessor{
				existingBeadID: "bd-1",
				runState:       state,
			}
			svc := newSvcWithFakeWT("bd-1", wa)

			if err := svc.RemoveWorktree(context.Background(), "bd-1"); err != nil {
				t.Fatalf("expected success for state=%q, got %v", state, err)
			}
			if wa.removeBeadID != "bd-1" {
				t.Errorf("Remove not called with correct beadID: got %q", wa.removeBeadID)
			}
		})
	}
}

// TestT037_RemoveWorktree_BackendVCSUnavailable maps ErrVCSUnavailable properly.
func TestT037_RemoveWorktree_BackendVCSUnavailable(t *testing.T) {
	wa := &fakeWriteableWorktreeAccessor{
		existingBeadID: "bd-1",
		runState:       core.StepDone,
		removeErr:      wt.ErrVCSUnavailable,
	}
	svc := newSvcWithFakeWT("bd-1", wa)

	err := svc.RemoveWorktree(context.Background(), "bd-1")
	se := mustServiceError(t, err)
	if se.Code != CodeVCSUnavailable {
		t.Errorf("code want %q got %q", CodeVCSUnavailable, se.Code)
	}
}

// mustServiceError asserts err is a *ServiceError and returns it.
func mustServiceError(t *testing.T, err error) *ServiceError {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	se := &ServiceError{}
	if !errors.As(err, &se) {
		t.Fatalf("expected *ServiceError, got %T: %v", err, err)
	}
	return se
}

// ── T040: WS event emission for worktree write-side ──────────────────────────

// newSvcWithFakeWTAndPub returns a BeadService with the given WorktreeAccessor
// and a pub function that captures emitted frames.
func newSvcWithFakeWTAndPub(beadID string, wa WorktreeAccessor) (*BeadService, *[]ws.Frame) {
	backend := store.NewMemoryBackend([]store.Issue{
		{ID: beadID, Title: "Test bead", Status: "open", IssueType: "task"},
	})
	var frames []ws.Frame
	pub := func(f ws.Frame) { frames = append(frames, f) }
	svc := NewBeadServiceWithRepo(backend, nil, pub, "muster", true).
		WithWorktreeAccessor(wa)
	return svc, &frames
}

// TestT040_FinalizeWorktree_EmitsEvent verifies worktree.finalized is published
// on successful Finalize, with the committed field set.
func TestT040_FinalizeWorktree_EmitsEvent(t *testing.T) {
	wa := &fakeWriteableWorktreeAccessor{
		existingBeadID: "bd-1",
		runState:       core.StepDone,
	}
	svc, frames := newSvcWithFakeWTAndPub("bd-1", wa)

	if _, err := svc.FinalizeWorktree(context.Background(), "bd-1", "seal it"); err != nil {
		t.Fatalf("FinalizeWorktree: %v", err)
	}

	if len(*frames) != 1 {
		t.Fatalf("expected 1 WS frame, got %d", len(*frames))
	}
	f := (*frames)[0]
	if f.Type != ws.EventWorktreeFinalized {
		t.Errorf("frame type = %q, want %q", f.Type, ws.EventWorktreeFinalized)
	}
	if f.BeadID != "bd-1" {
		t.Errorf("frame beadID = %q, want bd-1", f.BeadID)
	}
	// Committed field must be set (not nil) and reflect the backend's return value.
	if f.Committed == nil {
		t.Error("frame Committed is nil, want non-nil *bool")
	} else if !*f.Committed {
		t.Errorf("frame Committed = false, want true (fake returns committed=true)")
	}
}

// TestT040_PushWorktree_EmitsEvent verifies worktree.pushed is published on
// successful Push, with branch and remote populated.
func TestT040_PushWorktree_EmitsEvent(t *testing.T) {
	wa := &fakeWriteableWorktreeAccessor{
		existingBeadID: "bd-1",
		runState:       core.StepDone,
	}
	svc, frames := newSvcWithFakeWTAndPub("bd-1", wa)

	if err := svc.PushWorktree(context.Background(), "bd-1", ""); err != nil {
		t.Fatalf("PushWorktree: %v", err)
	}

	if len(*frames) != 1 {
		t.Fatalf("expected 1 WS frame, got %d", len(*frames))
	}
	f := (*frames)[0]
	if f.Type != ws.EventWorktreePushed {
		t.Errorf("frame type = %q, want %q", f.Type, ws.EventWorktreePushed)
	}
	if f.BeadID != "bd-1" {
		t.Errorf("frame beadID = %q, want bd-1", f.BeadID)
	}
	if f.Branch != wt.BranchName("bd-1") {
		t.Errorf("frame branch = %q, want %q", f.Branch, wt.BranchName("bd-1"))
	}
	if f.Remote != "origin" {
		t.Errorf("frame remote = %q, want origin", f.Remote)
	}
}

// TestT040_PushWorktree_EmitsEvent_CustomRemote verifies the pushed event carries
// the caller-supplied remote name (not always "origin").
func TestT040_PushWorktree_EmitsEvent_CustomRemote(t *testing.T) {
	wa := &fakeWriteableWorktreeAccessor{
		existingBeadID: "bd-1",
		runState:       core.StepDone,
	}
	svc, frames := newSvcWithFakeWTAndPub("bd-1", wa)

	if err := svc.PushWorktree(context.Background(), "bd-1", "upstream"); err != nil {
		t.Fatalf("PushWorktree: %v", err)
	}

	if len(*frames) != 1 {
		t.Fatalf("expected 1 WS frame, got %d", len(*frames))
	}
	f := (*frames)[0]
	if f.Remote != "upstream" {
		t.Errorf("frame remote = %q, want upstream", f.Remote)
	}
}

// TestT040_RemoveWorktree_EmitsEvent verifies worktree.removed is published on
// successful Remove.
func TestT040_RemoveWorktree_EmitsEvent(t *testing.T) {
	wa := &fakeWriteableWorktreeAccessor{
		existingBeadID: "bd-1",
		runState:       core.StepDone,
	}
	svc, frames := newSvcWithFakeWTAndPub("bd-1", wa)

	if err := svc.RemoveWorktree(context.Background(), "bd-1"); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}

	if len(*frames) != 1 {
		t.Fatalf("expected 1 WS frame, got %d", len(*frames))
	}
	f := (*frames)[0]
	if f.Type != ws.EventWorktreeRemoved {
		t.Errorf("frame type = %q, want %q", f.Type, ws.EventWorktreeRemoved)
	}
	if f.BeadID != "bd-1" {
		t.Errorf("frame beadID = %q, want bd-1", f.BeadID)
	}
}

// TestT040_FinalizeWorktree_NoEventOnError verifies no WS event on failure.
func TestT040_FinalizeWorktree_NoEventOnError(t *testing.T) {
	wa := &fakeWriteableWorktreeAccessor{
		existingBeadID: "bd-1",
		runState:       core.StepDone,
		finalizeErr:    errors.New("git commit failed"),
	}
	svc, frames := newSvcWithFakeWTAndPub("bd-1", wa)

	if _, err := svc.FinalizeWorktree(context.Background(), "bd-1", "msg"); err == nil {
		t.Fatal("expected error, got nil")
	}
	if len(*frames) != 0 {
		t.Errorf("expected no WS frames on error, got %d", len(*frames))
	}
}

// ── T041: per-bead operation mutex (tri-review 5 — TOCTOU) ─────────────────────────

// slowWorktreeAccessor wraps fakeWriteableWorktreeAccessor and inserts a
// configurable delay inside Finalize and Remove so concurrent callers have a
// real opportunity to interleave. It also maintains an atomic in-flight counter
// so the test can assert mutual exclusion.
type slowWorktreeAccessor struct {
	fakeWriteableWorktreeAccessor
	delay    time.Duration
	inFlight atomic.Int32 // incremented during Finalize/Remove; must never exceed 1
	maxSeen  atomic.Int32 // peak in-flight value observed
}

func (s *slowWorktreeAccessor) Finalize(ctx context.Context, beadID, message string) (bool, error) {
	cur := s.inFlight.Add(1)
	defer s.inFlight.Add(-1)
	// Record peak.
	for {
		prev := s.maxSeen.Load()
		if cur <= prev || s.maxSeen.CompareAndSwap(prev, cur) {
			break
		}
	}
	time.Sleep(s.delay)
	return s.fakeWriteableWorktreeAccessor.Finalize(ctx, beadID, message)
}

func (s *slowWorktreeAccessor) Remove(ctx context.Context, beadID string) error {
	cur := s.inFlight.Add(1)
	defer s.inFlight.Add(-1)
	for {
		prev := s.maxSeen.Load()
		if cur <= prev || s.maxSeen.CompareAndSwap(prev, cur) {
			break
		}
	}
	time.Sleep(s.delay)
	return s.fakeWriteableWorktreeAccessor.Remove(ctx, beadID)
}

// TestT041_WtOp_Serialized verifies that concurrent FinalizeWorktree and
// RemoveWorktree calls for the same bead are serialized by the per-bead
// operation mutex: the in-flight counter inside the slow accessor must never
// exceed 1, even though both goroutines start simultaneously.
func TestT041_WtOp_Serialized(t *testing.T) {
	t.Parallel()

	slow := &slowWorktreeAccessor{
		fakeWriteableWorktreeAccessor: fakeWriteableWorktreeAccessor{
			existingBeadID: "bd-1",
			runState:       core.StepDone,
		},
		delay: 20 * time.Millisecond,
	}
	backend := store.NewMemoryBackend([]store.Issue{
		{ID: "bd-1", Title: "Test bead", Status: "open", IssueType: "task"},
	})
	svc := NewBeadService(backend, nil, func(ws.Frame) {}).WithWorktreeAccessor(slow)

	ctx := context.Background()

	// Launch both operations simultaneously.
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	var finalizeErr, removeErr error
	go func() {
		defer wg.Done()
		<-start
		_, finalizeErr = svc.FinalizeWorktree(ctx, "bd-1", "msg")
	}()
	go func() {
		defer wg.Done()
		<-start
		// Give Finalize a tiny head start so it enters the slow section first.
		time.Sleep(2 * time.Millisecond)
		removeErr = svc.RemoveWorktree(ctx, "bd-1")
	}()

	close(start)
	wg.Wait()

	// Both operations must succeed (the fake never returns an error here).
	if finalizeErr != nil {
		t.Errorf("FinalizeWorktree: %v", finalizeErr)
	}
	if removeErr != nil {
		t.Errorf("RemoveWorktree: %v", removeErr)
	}

	// The critical assertion: the peak in-flight count must be exactly 1.
	// If the mutex is absent, both goroutines enter the slow section together
	// and maxSeen reaches 2.
	if peak := slow.maxSeen.Load(); peak > 1 {
		t.Errorf("concurrent operations detected: peak in-flight = %d, want ≤ 1", peak)
	}
}
