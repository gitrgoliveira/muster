package orchestrator_test

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/gitrgoliveira/muster/internal/adapter"
	"github.com/gitrgoliveira/muster/internal/adapter/claude"
	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/orchestrator"
	"github.com/gitrgoliveira/muster/internal/ws"
)

// TestDispatch_RunTimeout_KillsAndEmitsClosed exercises the FR-017 run-timeout
// path in watchRun: when --run-timeout elapses and the pane is still alive, the
// run is cancelled, the session killed, the step marked failed, and a
// tmux.session.closed frame emitted with exitCode -1.
//
// Verified via the published frame (not the fake's kill flag) to stay race-free:
// the kill happens on the watcher goroutine.
func TestDispatch_RunTimeout_KillsAndEmitsClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	setupFakeClaude(t)

	transport := &fakeTransport{deadDead: false} // pane never dies on its own
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))

	var mu sync.Mutex
	var closedExit *int
	pub := orchestrator.Publisher(func(f ws.Frame) {
		if f.Type == ws.EventTmuxClosed && f.ExitCode != nil {
			mu.Lock()
			v := *f.ExitCode
			closedExit = &v
			mu.Unlock()
		}
	})

	o := orchestrator.New(orchestrator.Config{
		Adapters:     reg,
		Transport:    transport,
		RepoMap:      orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir: t.TempDir(),
		Publish:      pub,
		RunTimeout:   50 * time.Millisecond, // FR-017 opt-in
	})

	_, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         "mp-timeout",
		BeadTitle:      "Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	// The 50ms timeout fires before the 500ms DeadStatus poll → closed event.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := closedExit
		mu.Unlock()
		if got != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	gotExit := closedExit
	mu.Unlock()
	if gotExit == nil {
		t.Fatal("expected tmux.session.closed after run timeout")
	}
	if *gotExit != -1 {
		t.Errorf("timeout closed exitCode want -1 got %d", *gotExit)
	}

	if run := o.GetRun("mp-timeout"); run == nil || run.State != core.StepFailed {
		t.Errorf("run want failed after timeout, got %+v", run)
	}
}
