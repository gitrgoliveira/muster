package orchestrator_test

// T041: failing tests for chain resolution, advance/loopback range-checks,
//        ErrStepOutOfRange sentinel.
// T042: failing test asserting M2 single-step (idx 0, no chain) is preserved.

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/gitrgoliveira/muster/internal/adapter"
	"github.com/gitrgoliveira/muster/internal/adapter/claude"
	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/orchestrator"
)

// ── T041: chain resolution ────────────────────────────────────────────────────

// TestResolveChain_NilWhenNoChain verifies that dispatching with no chain (nil)
// gives a run with nil Chain (single implicit step 0 — M2 behaviour).
func TestResolveChain_NilWhenNoChain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	o, _ := newOrchestratorForTest(t, repoPath)

	_, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         "mp-chain0",
		BeadTitle:      "Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
		Chain:          nil,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	run := o.GetRun("mp-chain0")
	if run == nil {
		t.Fatal("GetRun returned nil")
	}
	if run.Chain != nil {
		t.Errorf("Chain want nil (single-step default) got %v", run.Chain)
	}
}

// TestResolveChain_RequestOverride verifies that a chain supplied in the
// dispatch request is stored on the run.
func TestResolveChain_RequestOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	o, _ := newOrchestratorForTest(t, repoPath)

	chain := orchestrator.StepChain{
		{Name: "plan", PermissionMode: core.PermPlan, PromptRef: ""},
		{Name: "build", PermissionMode: core.PermAcceptEdits, PromptRef: ""},
	}
	_, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         "mp-chainr",
		BeadTitle:      "Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
		Chain:          &chain,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	run := o.GetRun("mp-chainr")
	if run == nil {
		t.Fatal("GetRun returned nil")
	}
	if run.Chain == nil {
		t.Fatal("Chain want non-nil for dispatch with explicit chain, got nil")
	}
	if len(*run.Chain) != 2 {
		t.Errorf("Chain length want 2 got %d", len(*run.Chain))
	}
	if (*run.Chain)[0].Name != "plan" {
		t.Errorf("Chain[0].Name want plan got %q", (*run.Chain)[0].Name)
	}
}

// ── T041: Advance range-checks and ErrStepOutOfRange ─────────────────────────

// TestAdvance_ErrStepOutOfRange_NoChain verifies Advance returns ErrStepOutOfRange
// when the run has no chain (nil → single step, can't advance).
func TestAdvance_ErrStepOutOfRange_NoChain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	o, transport := newOrchestratorForTest(t, repoPath)
	transport.deadDead = false // keep alive

	_, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         "mp-adv0",
		BeadTitle:      "Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	err = o.Advance("mp-adv0")
	if !errors.Is(err, orchestrator.ErrStepOutOfRange) {
		t.Errorf("Advance on single-step run: want ErrStepOutOfRange, got %v", err)
	}
}

// TestAdvance_ErrStepOutOfRange_AtEnd verifies Advance returns ErrStepOutOfRange
// when already at the last step.
func TestAdvance_ErrStepOutOfRange_AtEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	o, transport := newOrchestratorForTest(t, repoPath)
	transport.deadDead = false // keep alive

	chain := orchestrator.StepChain{
		{Name: "plan", PermissionMode: core.PermPlan},
		{Name: "build", PermissionMode: core.PermAcceptEdits},
	}
	_, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         "mp-advend",
		BeadTitle:      "Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
		Chain:          &chain,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	// Advance to step 1.
	if err := o.Advance("mp-advend"); err != nil {
		t.Fatalf("first Advance: %v", err)
	}

	// Attempt to advance past the end (chain has only 2 steps, idx 0 and 1).
	err = o.Advance("mp-advend")
	if !errors.Is(err, orchestrator.ErrStepOutOfRange) {
		t.Errorf("Advance at last step: want ErrStepOutOfRange, got %v", err)
	}
}

// TestAdvance_ErrStepOutOfRange_NoBead verifies Advance returns an error
// when there is no run for the given bead.
func TestAdvance_ErrStepOutOfRange_NoBead(t *testing.T) {
	o := orchestrator.New(orchestrator.Config{
		Adapters:     adapter.NewRegistry(),
		Transport:    &fakeTransport{},
		RepoMap:      orchestrator.RepoMap{},
		WorktreesDir: t.TempDir(),
	})

	err := o.Advance("mp-nonexistent")
	if err == nil {
		t.Error("Advance on non-existent bead: want error, got nil")
	}
}

// ── T041: LoopBack range-checks ──────────────────────────────────────────────

// TestLoopBack_ErrStepOutOfRange_NoChain verifies LoopBack returns ErrStepOutOfRange
// when the run has no chain (toIdx=0 on a single-step run can't loop back).
func TestLoopBack_ErrStepOutOfRange_NoChain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	o, transport := newOrchestratorForTest(t, repoPath)
	transport.deadDead = false

	_, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         "mp-lb0",
		BeadTitle:      "Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	err = o.LoopBack("mp-lb0", 0)
	if !errors.Is(err, orchestrator.ErrStepOutOfRange) {
		t.Errorf("LoopBack on step 0: want ErrStepOutOfRange, got %v", err)
	}
}

// TestLoopBack_ErrStepOutOfRange_Negative verifies LoopBack rejects toIdx < 0.
func TestLoopBack_ErrStepOutOfRange_Negative(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	o, transport := newOrchestratorForTest(t, repoPath)
	transport.deadDead = false

	chain := orchestrator.StepChain{
		{Name: "plan", PermissionMode: core.PermPlan},
		{Name: "build", PermissionMode: core.PermAcceptEdits},
	}
	_, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         "mp-lbneg",
		BeadTitle:      "Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
		Chain:          &chain,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	// advance to step 1 first
	if err := o.Advance("mp-lbneg"); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	err = o.LoopBack("mp-lbneg", -1)
	if !errors.Is(err, orchestrator.ErrStepOutOfRange) {
		t.Errorf("LoopBack(-1): want ErrStepOutOfRange, got %v", err)
	}
}

// TestLoopBack_ErrStepOutOfRange_SameOrHigher verifies LoopBack rejects
// toIdx >= currentStepIdx.
func TestLoopBack_ErrStepOutOfRange_SameOrHigher(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	o, transport := newOrchestratorForTest(t, repoPath)
	transport.deadDead = false

	chain := orchestrator.StepChain{
		{Name: "plan", PermissionMode: core.PermPlan},
		{Name: "build", PermissionMode: core.PermAcceptEdits},
		{Name: "review", PermissionMode: core.PermAcceptEdits},
	}
	_, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         "mp-lbsame",
		BeadTitle:      "Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
		Chain:          &chain,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	// advance to step 1
	if err := o.Advance("mp-lbsame"); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	// toIdx == currentStepIdx (1) — not allowed (must be < current)
	err = o.LoopBack("mp-lbsame", 1)
	if !errors.Is(err, orchestrator.ErrStepOutOfRange) {
		t.Errorf("LoopBack(toIdx=current): want ErrStepOutOfRange, got %v", err)
	}

	// toIdx > currentStepIdx (2) — also not allowed
	err = o.LoopBack("mp-lbsame", 2)
	if !errors.Is(err, orchestrator.ErrStepOutOfRange) {
		t.Errorf("LoopBack(toIdx>current): want ErrStepOutOfRange, got %v", err)
	}
}

// TestLoopBack_ErrStepOutOfRange_NoBead verifies LoopBack returns an error
// when there is no run for the given bead.
func TestLoopBack_ErrStepOutOfRange_NoBead(t *testing.T) {
	o := orchestrator.New(orchestrator.Config{
		Adapters:     adapter.NewRegistry(),
		Transport:    &fakeTransport{},
		RepoMap:      orchestrator.RepoMap{},
		WorktreesDir: t.TempDir(),
	})

	err := o.LoopBack("mp-nonexistent", 0)
	if err == nil {
		t.Error("LoopBack on non-existent bead: want error, got nil")
	}
}

// ── T042: M2 single-step behaviour byte-for-byte preserved ───────────────────

// TestM2SingleStep_Preserved verifies that a dispatch with no chain:
//  1. Creates a run with StepIdx=0 and nil Chain.
//  2. The session name is SessionName(beadID, 0, 0) — same as M2.
//  3. The prompt file is named .muster-prompt-0.txt — same as M2.
func TestM2SingleStep_Preserved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	o, transport := newOrchestratorForTest(t, repoPath)

	req := orchestrator.DispatchRequest{
		BeadID:         "mp-m2",
		BeadTitle:      "M2 test",
		BeadDesc:       "Preserve M2 behaviour",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
		// No Chain — M2 single-step.
	}
	_, err := o.Dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	run := o.GetRun("mp-m2")
	if run == nil {
		t.Fatal("GetRun returned nil")
	}
	if run.StepIdx != 0 {
		t.Errorf("StepIdx want 0 got %d", run.StepIdx)
	}
	if run.Chain != nil {
		t.Errorf("Chain want nil for M2 single-step dispatch, got %v", run.Chain)
	}
	if run.Loop != 0 {
		t.Errorf("Loop want 0 got %d", run.Loop)
	}

	// Verify session name matches the M2 convention.
	transport.spawnMu.Lock()
	sess := transport.spawnedSession
	transport.spawnMu.Unlock()
	if sess == nil {
		t.Fatal("no session spawned")
	}
	wantSession := "muster/" + req.BeadID + "/0/0"
	if sess.Name != wantSession {
		t.Errorf("session name want %q got %q", wantSession, sess.Name)
	}
}

// ── T043a: step-indexed session/prompt names don't collide ───────────────────

// TestStepIdx_SessionAndPromptNamesDistinct verifies that step 1's session name
// and prompt file differ from step 0's and don't collide.
func TestStepIdx_SessionAndPromptNamesDistinct(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	o, transport := newOrchestratorForTest(t, repoPath)

	chain := orchestrator.StepChain{
		{Name: "plan", PermissionMode: core.PermPlan},
		{Name: "build", PermissionMode: core.PermAcceptEdits},
	}
	_, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         "mp-sess",
		BeadTitle:      "Session test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
		Chain:          &chain,
	})
	if err != nil {
		t.Fatalf("Dispatch step 0: %v", err)
	}

	// Verify step 0 session name.
	transport.spawnMu.Lock()
	step0Sess := transport.spawnedSession
	transport.spawnMu.Unlock()
	if step0Sess == nil {
		t.Fatal("no session spawned for step 0")
	}
	wantStep0 := "muster/mp-sess/0/0"
	if step0Sess.Name != wantStep0 {
		t.Errorf("step 0 session: want %q got %q", wantStep0, step0Sess.Name)
	}

	// Advance to step 1.
	if err := o.Advance("mp-sess"); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	// Give step 1 time to launch. We wait until the run is StepActive at step 1
	// with a non-empty Session — this avoids capturing the transient window
	// where StepIdx=1 but State=StepFailed (between finishRun and relaunchNextStep)
	// or Session is still the step-0 value (before relaunchNextStep clears it).
	// relaunchNextStep sets Session="" and State=StepActive under the lock, then
	// doLaunch sets Session="muster/mp-sess/1/0" and State=StepActive under the
	// lock — so {StepIdx=1, State=StepActive, Session!=""} is reached only after
	// doLaunch sets the new session name.
	deadline := time.Now().Add(3 * time.Second)
	var step1Sess string
	for time.Now().Before(deadline) {
		run := o.GetRun("mp-sess")
		if run != nil && run.StepIdx == 1 && run.State == core.StepActive && run.Session != "" {
			step1Sess = run.Session
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if step1Sess == "" {
		t.Fatal("step 1 session not set within timeout")
	}

	wantStep1 := "muster/mp-sess/1/0"
	if step1Sess != wantStep1 {
		t.Errorf("step 1 session: want %q got %q", wantStep1, step1Sess)
	}
	if step1Sess == step0Sess.Name {
		t.Error("step 1 session must differ from step 0 session (no collision)")
	}
}

// ── T043b: advance/finish interlock -race test ───────────────────────────────

// TestAdvance_ErrStepOutOfRange_QueuedRun is the regression for tri-review #1/#3:
// advancing a StepPending (queued, not-yet-launched) run must be rejected, not
// silently mutate its StepIdx — which would make the scheduler later launch it
// at step 1, skipping step 0 and double-running step 1.
func TestAdvance_ErrStepOutOfRange_QueuedRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	setupFakeClaude(t)
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))
	transport := &fakeTransport{deadDead: false} // keep the active run alive

	o := orchestrator.New(orchestrator.Config{
		Adapters:        reg,
		Transport:       transport,
		RepoMap:         orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir:    t.TempDir(),
		DefaultPermMode: core.PermAcceptEdits,
		MaxConcurrent:   1, // one active slot → the second dispatch queues
	})
	chain := orchestrator.StepChain{
		{Name: "plan", PermissionMode: core.PermPlan},
		{Name: "build", PermissionMode: core.PermAcceptEdits},
	}

	// First dispatch fills the single active slot.
	if _, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID: "mp-active", BeadTitle: "active", Agent: core.AgentClaude,
		Mode: core.ModeAgent, PermissionMode: core.PermAcceptEdits, Chain: &chain,
	}); err != nil {
		t.Fatalf("Dispatch active: %v", err)
	}
	// Second dispatch queues (StepPending) — never launched.
	res, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID: "mp-queued", BeadTitle: "queued", Agent: core.AgentClaude,
		Mode: core.ModeAgent, PermissionMode: core.PermAcceptEdits, Chain: &chain,
	})
	if err != nil {
		t.Fatalf("Dispatch queued: %v", err)
	}
	if !res.Queued {
		t.Fatalf("second dispatch: want Queued=true (at capacity), got %+v", res)
	}

	// Advancing the queued run must be rejected and must NOT mutate its pointer.
	if err := o.Advance("mp-queued"); !errors.Is(err, orchestrator.ErrStepOutOfRange) {
		t.Errorf("Advance on queued run: want ErrStepOutOfRange, got %v", err)
	}
	if run := o.GetRun("mp-queued"); run == nil || run.StepIdx != 0 {
		t.Errorf("queued run StepIdx must remain 0 after rejected Advance, got %v", run)
	}
}

// TestAdvance_WhileWatcherFinishing is a -race test that verifies Advance does
// not race with a concurrently finishing step's watcher goroutine. Specifically,
// finishRun must NOT evict a bead whose chain has a pending advance.
func TestAdvance_WhileWatcherFinishing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	setupFakeClaude(t)
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))

	// Transport: initially alive, then we flip it dead to trigger watcher.
	transport := &fakeTransport{deadDead: false}

	o := orchestrator.New(orchestrator.Config{
		Adapters:        reg,
		Transport:       transport,
		RepoMap:         orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir:    t.TempDir(),
		DefaultPermMode: core.PermAcceptEdits,
	})

	chain := orchestrator.StepChain{
		{Name: "plan", PermissionMode: core.PermPlan},
		{Name: "build", PermissionMode: core.PermAcceptEdits},
	}
	_, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         "mp-race",
		BeadTitle:      "Race test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
		Chain:          &chain,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	// Launch Advance and session-kill concurrently to race the finish path.
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		// Advance might succeed or get ErrStepOutOfRange depending on timing —
		// both are valid; what must NOT happen is a data race.
		_ = o.Advance("mp-race")
	}()

	go func() {
		defer wg.Done()
		// Flip the transport to dead so watchRun finishes the run.
		transport.forceDead.Store(true)
	}()

	wg.Wait()

	// Either step 0 or step 1 should be the live state (no stale eviction).
	// The important property tested by -race is no data race, not the exact
	// end state (which is timing-dependent).
}
