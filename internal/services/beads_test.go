package services

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"testing"

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
type fakeOrchestrator struct{ dispatchCalled bool }

func (f *fakeOrchestrator) Dispatch(context.Context, OrchestratorDispatchRequest) (*core.Bead, error) {
	f.dispatchCalled = true
	return &core.Bead{ID: "mp-aaa", Column: core.ColRunning}, nil
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
	if got.Column != core.ColRunning {
		t.Errorf("returned bead Column want running got %q", got.Column)
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
	if got.Column != core.ColRunning {
		t.Errorf("returned bead Column want running got %q", got.Column)
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
	if got.Column != core.ColRunning {
		t.Errorf("returned bead Column want running got %q", got.Column)
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
	if got.Column != core.ColRunning {
		t.Errorf("returned bead Column want running got %q", got.Column)
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
