package orchestrator_test

// T011–T014: Failing tests for the capacity-gated FIFO scheduler (US1).
//
// These tests exercise the scheduler methods directly. They are written against
// the internal package via the _test convention, so they use exported types
// only. The scheduler lives inside the orchestrator package sharing the
// existing mu, so the tests reach it through the Orchestrator's exported
// surface (Dispatch + helper methods added in T015–T016).
//
// Test structure:
//   T011 – admission bound: active ≤ capacity; fail-fast on non-positive capacity
//   T012 – -race test: N concurrent dispatches at capacity; exactly N-k admitted; k queued
//   T013 – auto-admit next waiter when a run reaches a terminal state
//   T014 – SetCapacity: raise→admits; lower→drains (never kills); rejects ≤0

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gitrgoliveira/muster/internal/adapter"
	"github.com/gitrgoliveira/muster/internal/adapter/claude"
	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/orchestrator"
	"github.com/gitrgoliveira/muster/internal/ws"
)

// newSchedulerOrchestrator creates an Orchestrator with the given capacity and
// a git repo, wired for scheduler tests.
func newSchedulerOrchestrator(t *testing.T, repoPath string, capacity int) *orchestrator.Orchestrator {
	t.Helper()
	setupFakeClaude(t)
	transport := &fakeTransport{deadDead: false} // sessions live until forceDead
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))
	worktreesDir := t.TempDir()
	o := orchestrator.New(orchestrator.Config{
		Adapters:        reg,
		Transport:       transport,
		RepoMap:         orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir:    worktreesDir,
		MaxConcurrent:   capacity,
		DefaultPermMode: core.PermAcceptEdits,
	})
	return o
}

func dispatchReq(beadID string) orchestrator.DispatchRequest {
	return orchestrator.DispatchRequest{
		BeadID:         beadID,
		BeadTitle:      "Title " + beadID,
		BeadDesc:       "Desc",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	}
}

// T011: Admission bound — active ≤ capacity; waiters enqueued; non-positive capacity rejected.
func TestScheduler_T011_AdmissionBound(t *testing.T) {
	repoPath := initGitRepo(t)
	const cap = 2
	o := newSchedulerOrchestrator(t, repoPath, cap)

	// Dispatch cap+1 runs.
	ids := []string{"mp-001", "mp-002", "mp-003"}
	results := make([]orchestrator.DispatchResult, len(ids))
	var errs []error
	for i, id := range ids {
		res, err := o.Dispatch(context.Background(), dispatchReq(id))
		results[i] = res
		errs = append(errs, err)
	}

	// All dispatches must succeed without error.
	for i, err := range errs {
		if err != nil {
			t.Errorf("Dispatch(%q): unexpected error: %v", ids[i], err)
		}
	}

	// Count active and queued.
	var activeCount, queuedCount int
	for i, res := range results {
		if res.Queued {
			queuedCount++
		} else {
			activeCount++
		}
		_ = i
	}

	if activeCount != cap {
		t.Errorf("activeCount want %d got %d", cap, activeCount)
	}
	if queuedCount != 1 {
		t.Errorf("queuedCount want 1 got %d", queuedCount)
	}

	// SchedulerSnapshot must reflect the state.
	snap := o.SchedulerSnapshot()
	if snap.Capacity != cap {
		t.Errorf("snapshot.Capacity want %d got %d", cap, snap.Capacity)
	}
	if snap.ActiveCount != cap {
		t.Errorf("snapshot.ActiveCount want %d got %d", cap, snap.ActiveCount)
	}
	if len(snap.Waiting) != 1 {
		t.Errorf("snapshot.Waiting want 1 got %d", len(snap.Waiting))
	}
}

// T011b: Non-positive capacity must be rejected at construction.
func TestScheduler_T011b_NonPositiveCapacityRejected(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Logf("New panicked (acceptable): %v", r)
		}
	}()

	// SetCapacity(0) on an already-constructed orchestrator must return error.
	repoPath := initGitRepo(t)
	o := newSchedulerOrchestrator(t, repoPath, 2)
	err := o.SetCapacity(0)
	if err == nil {
		t.Error("SetCapacity(0) should return error")
	}
	err2 := o.SetCapacity(-1)
	if err2 == nil {
		t.Error("SetCapacity(-1) should return error")
	}
}

// T012: -race test: N concurrent dispatches at capacity N-k admit exactly N-k, enqueue k.
func TestScheduler_T012_ConcurrentDispatch_NoRace(t *testing.T) {
	repoPath := initGitRepo(t)
	const cap = 3
	const total = 5
	o := newSchedulerOrchestrator(t, repoPath, cap)

	var wg sync.WaitGroup
	type outcome struct {
		queued bool
		err    error
	}
	outcomes := make([]outcome, total)
	var mu sync.Mutex

	for i := 0; i < total; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := "mp-r" + string(rune('0'+i))
			res, err := o.Dispatch(context.Background(), dispatchReq(id))
			mu.Lock()
			outcomes[i] = outcome{queued: res.Queued, err: err}
			mu.Unlock()
		}()
	}
	wg.Wait()

	var activeCount, queuedCount int
	for _, out := range outcomes {
		if out.err != nil {
			t.Errorf("Dispatch error: %v", out.err)
			continue
		}
		if out.queued {
			queuedCount++
		} else {
			activeCount++
		}
	}

	if activeCount != cap {
		t.Errorf("activeCount want %d got %d", cap, activeCount)
	}
	if queuedCount != total-cap {
		t.Errorf("queuedCount want %d got %d", total-cap, queuedCount)
	}

	snap := o.SchedulerSnapshot()
	if snap.ActiveCount != cap {
		t.Errorf("snapshot.ActiveCount want %d got %d", cap, snap.ActiveCount)
	}
	if len(snap.Waiting) != total-cap {
		t.Errorf("snapshot.Waiting want %d got %d", total-cap, len(snap.Waiting))
	}
}

// T013: Auto-admit next FIFO waiter when a run reaches terminal state.
func TestScheduler_T013_AutoAdmitOnRunEnd(t *testing.T) {
	repoPath := initGitRepo(t)
	const cap = 1
	transport := &fakeTransport{deadDead: false}
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))
	worktreesDir := t.TempDir()
	o := orchestrator.New(orchestrator.Config{
		Adapters:        reg,
		Transport:       transport,
		RepoMap:         orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir:    worktreesDir,
		MaxConcurrent:   cap,
		DefaultPermMode: core.PermAcceptEdits,
		RunRetention:    100 * time.Millisecond,
	})

	// Dispatch the first run (admitted).
	res1, err := o.Dispatch(context.Background(), dispatchReq("mp-first"))
	if err != nil {
		t.Fatalf("Dispatch mp-first: %v", err)
	}
	if res1.Queued {
		t.Fatal("mp-first should be admitted (capacity=1)")
	}

	// Dispatch the second run (queued).
	res2, err := o.Dispatch(context.Background(), dispatchReq("mp-second"))
	if err != nil {
		t.Fatalf("Dispatch mp-second: %v", err)
	}
	if !res2.Queued {
		t.Fatal("mp-second should be queued (capacity=1, first admitted)")
	}

	snap := o.SchedulerSnapshot()
	if snap.ActiveCount != 1 || len(snap.Waiting) != 1 {
		t.Fatalf("pre-finish: active=%d waiting=%d, want 1/1", snap.ActiveCount, len(snap.Waiting))
	}

	// Simulate the first run finishing by marking it dead in the transport.
	transport.forceDead.Store(true)

	// Wait for the watcher to pick up the dead session and admit the second.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snap = o.SchedulerSnapshot()
		if snap.ActiveCount == 1 && len(snap.Waiting) == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if snap.ActiveCount != 1 || len(snap.Waiting) != 0 {
		t.Errorf("after finish: active=%d waiting=%d, want 1/0", snap.ActiveCount, len(snap.Waiting))
	}

	// Verify mp-second is now active.
	run := o.GetRun("mp-second")
	if run == nil {
		t.Fatal("GetRun(mp-second) returned nil")
	}
	if run.State != core.StepActive {
		t.Errorf("mp-second state want active got %q", run.State)
	}
}

// T014: SetCapacity raise→admits up to new limit; lower→drains (never kills); rejects ≤0.
func TestScheduler_T014_SetCapacity(t *testing.T) {
	repoPath := initGitRepo(t)
	// Start with capacity=1, dispatch 3 runs: 1 active, 2 queued.
	transport := &fakeTransport{deadDead: false}
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))
	worktreesDir := t.TempDir()
	o := orchestrator.New(orchestrator.Config{
		Adapters:        reg,
		Transport:       transport,
		RepoMap:         orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir:    worktreesDir,
		MaxConcurrent:   1,
		DefaultPermMode: core.PermAcceptEdits,
	})

	ids := []string{"mp-a01", "mp-a02", "mp-a03"}
	for _, id := range ids {
		if _, err := o.Dispatch(context.Background(), dispatchReq(id)); err != nil {
			t.Fatalf("Dispatch(%s): %v", id, err)
		}
	}

	snap := o.SchedulerSnapshot()
	if snap.ActiveCount != 1 || len(snap.Waiting) != 2 {
		t.Fatalf("pre-SetCapacity: active=%d waiting=%d, want 1/2", snap.ActiveCount, len(snap.Waiting))
	}

	// T014a: Raise capacity to 3 → both waiters should be admitted.
	if err := o.SetCapacity(3); err != nil {
		t.Fatalf("SetCapacity(3): %v", err)
	}

	// Give the background goroutines time to launch the newly admitted sessions.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		snap = o.SchedulerSnapshot()
		if snap.ActiveCount == 3 && len(snap.Waiting) == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if snap.ActiveCount != 3 || len(snap.Waiting) != 0 {
		t.Errorf("after SetCapacity(3): active=%d waiting=%d, want 3/0", snap.ActiveCount, len(snap.Waiting))
	}

	// T014b: Lower capacity to 2 → drains (never kills active runs).
	if err := o.SetCapacity(2); err != nil {
		t.Fatalf("SetCapacity(2): %v", err)
	}
	snap = o.SchedulerSnapshot()
	if snap.Capacity != 2 {
		t.Errorf("capacity after lower want 2 got %d", snap.Capacity)
	}
	// Active count stays at 3 (drain semantics — no kills).
	if snap.ActiveCount != 3 {
		t.Errorf("activeCount after lower want 3 (no kill) got %d", snap.ActiveCount)
	}

	// T014c: Reject ≤0.
	if err := o.SetCapacity(0); err == nil {
		t.Error("SetCapacity(0) should return error")
	}
	if err := o.SetCapacity(-5); err == nil {
		t.Error("SetCapacity(-5) should return error")
	}
}

// T022: dispatch.queued emitted for enqueued runs; dispatch.admitted emitted when
// a queued run is promoted.
func TestScheduler_T022_WS_Events(t *testing.T) {
	repoPath := initGitRepo(t)
	setupFakeClaude(t)

	var (
		mu     sync.Mutex
		frames []ws.Frame
	)
	publish := func(f ws.Frame) {
		mu.Lock()
		frames = append(frames, f)
		mu.Unlock()
	}

	transport := &fakeTransport{deadDead: false}
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))
	worktreesDir := t.TempDir()
	o := orchestrator.New(orchestrator.Config{
		Adapters:        reg,
		Transport:       transport,
		RepoMap:         orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir:    worktreesDir,
		MaxConcurrent:   1,
		DefaultPermMode: core.PermAcceptEdits,
		Publish:         publish,
		RunRetention:    100 * time.Millisecond,
	})

	// Dispatch the first run — admitted (cap=1).
	res1, err := o.Dispatch(context.Background(), dispatchReq("mp-evt1"))
	if err != nil {
		t.Fatalf("Dispatch mp-evt1: %v", err)
	}
	if res1.Queued {
		t.Fatal("mp-evt1 should be admitted")
	}

	// Dispatch the second run — queued.
	res2, err := o.Dispatch(context.Background(), dispatchReq("mp-evt2"))
	if err != nil {
		t.Fatalf("Dispatch mp-evt2: %v", err)
	}
	if !res2.Queued {
		t.Fatal("mp-evt2 should be queued")
	}

	// Verify dispatch.queued event was emitted for mp-evt2.
	mu.Lock()
	var queuedFrame *ws.Frame
	for i := range frames {
		if frames[i].Type == ws.EventDispatchQueued && frames[i].BeadID == "mp-evt2" {
			cp := frames[i]
			queuedFrame = &cp
			break
		}
	}
	mu.Unlock()

	if queuedFrame == nil {
		t.Error("dispatch.queued event not emitted for mp-evt2")
	} else {
		if queuedFrame.WaitingPos == nil {
			t.Error("dispatch.queued event missing waitingPos")
		} else if *queuedFrame.WaitingPos != 0 {
			t.Errorf("dispatch.queued waitingPos want 0 (first in queue) got %d", *queuedFrame.WaitingPos)
		}
	}

	// Simulate first run finishing — this should admit mp-evt2.
	transport.forceDead.Store(true)

	// Wait for dispatch.admitted event.
	var admittedCount atomic.Int32
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		for _, f := range frames {
			if f.Type == ws.EventDispatchAdmitted && f.BeadID == "mp-evt2" {
				admittedCount.Store(1)
			}
		}
		mu.Unlock()
		if admittedCount.Load() > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if admittedCount.Load() == 0 {
		t.Error("dispatch.admitted event not emitted for mp-evt2 after first run ended")
	}
}

// TestSetCapacity_AdmittedRunsAreActive is the regression for Copilot #361:
// when SetCapacity admits a queued waiter, its State must flip to StepActive
// under the lock (before the async launch), so a concurrent idempotent Dispatch
// joins it as active rather than wrongly reporting Queued:true.
func TestSetCapacity_AdmittedRunsAreActive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	setupFakeClaude(t)
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))
	transport := &fakeTransport{deadDead: false}

	o := orchestrator.New(orchestrator.Config{
		Adapters:        reg,
		Transport:       transport,
		RepoMap:         orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir:    t.TempDir(),
		DefaultPermMode: core.PermAcceptEdits,
		MaxConcurrent:   1,
	})
	t.Cleanup(func() { transport.forceDead.Store(true) })

	// A fills the single slot; B queues (StepPending).
	if _, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID: "mp-a", Agent: core.AgentClaude, Mode: core.ModeAgent, PermissionMode: core.PermAcceptEdits,
	}); err != nil {
		t.Fatalf("dispatch A: %v", err)
	}
	res, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID: "mp-b", Agent: core.AgentClaude, Mode: core.ModeAgent, PermissionMode: core.PermAcceptEdits,
	})
	if err != nil || !res.Queued {
		t.Fatalf("dispatch B: want queued, got res=%+v err=%v", res, err)
	}
	if run := o.GetRun("mp-b"); run == nil || run.State != core.StepPending {
		t.Fatalf("B should be StepPending while queued, got %v", run)
	}

	// Raise capacity → B is admitted. Its State must already be StepActive on
	// return (flipped under the lock), not StepPending.
	if err := o.SetCapacity(2); err != nil {
		t.Fatalf("SetCapacity: %v", err)
	}
	if run := o.GetRun("mp-b"); run == nil || run.State != core.StepActive {
		t.Errorf("admitted run must be StepActive after SetCapacity (Copilot #361), got %v", run)
	}
}
