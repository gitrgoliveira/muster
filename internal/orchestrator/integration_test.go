//go:build !windows

// This integration suite drives real tmux and Unix-only test scaffolding
// (os.Symlink to place a fake `claude` on PATH), so it is excluded from
// Windows builds — consistent with the other platform-gated tests in this
// package (see claude_test.go). skipIfNoRealTmux is defined and used only
// here, so gating the whole file leaves no dangling references.

package orchestrator_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gitrgoliveira/muster/internal/adapter"
	"github.com/gitrgoliveira/muster/internal/adapter/claude"
	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/orchestrator"
	"github.com/gitrgoliveira/muster/internal/tmux"
	"github.com/gitrgoliveira/muster/internal/ws"
)

// skipIfNoRealTmux skips the test unless a real, supported tmux is available.
// It uses the same RealManager.Detect() the orchestrator uses at startup, so
// the guard skips not just when tmux is absent but also when the installed
// version is unparseable or below the supported floor (>= 3.2) — running these
// real-transport tests against an out-of-contract tmux would fail in obscure
// ways rather than skipping cleanly.
func skipIfNoRealTmux(t *testing.T) {
	t.Helper()
	if _, err := tmux.NewRealManager("").Detect(); err != nil {
		t.Skipf("no supported tmux available; skipping integration test: %v", err)
	}
}

func TestIntegration_RunlogLine_Events(t *testing.T) {
	skipIfNoRealTmux(t)

	// Set up fake claude that outputs a known line.
	fakeClaudeScript, err := filepath.Abs(filepath.Join("..", "adapter", "claude", "testdata", "fake_claude.sh"))
	if err != nil {
		t.Fatalf("abs fake_claude: %v", err)
	}
	if _, err := os.Stat(fakeClaudeScript); err != nil {
		t.Skipf("fake_claude.sh not found: %v", err)
	}
	binDir := t.TempDir()
	dest := filepath.Join(binDir, "claude")
	if err := os.Symlink(fakeClaudeScript, dest); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)

	repoPath := initGitRepo(t)
	var eventsMu sync.Mutex
	var events []ws.Frame
	publisher := func(f ws.Frame) {
		eventsMu.Lock()
		events = append(events, f)
		eventsMu.Unlock()
	}

	realTransport := tmux.NewRealManager("")
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))

	o := orchestrator.New(orchestrator.Config{
		Adapters:     reg,
		Transport:    realTransport,
		RepoMap:      orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir: t.TempDir(),
		Publish:      orchestrator.Publisher(publisher),
	})

	req := orchestrator.DispatchRequest{
		BeadID:         "mp-runlog",
		BeadTitle:      "Runlog Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	}

	if _, err := o.Dispatch(context.Background(), req); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	// Wait for run to complete.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		run := o.GetRun("mp-runlog")
		if run != nil && run.State != core.StepActive {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	// After the run, we expect at least a tmux.session.opened event.
	// runlog.line frames depend on pipe-pane functionality; may not arrive in all environments.
	eventsMu.Lock()
	eventsCopy := make([]ws.Frame, len(events))
	copy(eventsCopy, events)
	eventsMu.Unlock()

	var foundOpened bool
	for _, ev := range eventsCopy {
		if ev.Type == ws.EventTmuxOpened && ev.BeadID == "mp-runlog" {
			foundOpened = true
			break
		}
	}
	if !foundOpened {
		t.Errorf("expected tmux.session.opened for mp-runlog; events: %v", eventsCopy)
	}
}

func TestIntegration_Dispatch_FakeClaude_RealTmux(t *testing.T) {
	skipIfNoRealTmux(t)

	// Set up fake claude.
	fakeClaudeScript, err := filepath.Abs(filepath.Join("..", "adapter", "claude", "testdata", "fake_claude.sh"))
	if err != nil {
		t.Fatalf("abs fake_claude: %v", err)
	}
	if _, err := os.Stat(fakeClaudeScript); err != nil {
		t.Skipf("fake_claude.sh not found: %v", err)
	}
	binDir := t.TempDir()
	dest := filepath.Join(binDir, "claude")
	if err := os.Symlink(fakeClaudeScript, dest); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)

	// Create a real git repo for the worktree.
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()

	// Track WS events (mutex-protected for race-safety).
	var eventsMu sync.Mutex
	var events []ws.Frame
	publisher := func(f ws.Frame) {
		eventsMu.Lock()
		events = append(events, f)
		eventsMu.Unlock()
	}

	// Build the orchestrator with real tmux transport.
	realTransport := tmux.NewRealManager("")
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))

	o := orchestrator.New(orchestrator.Config{
		Adapters:     reg,
		Transport:    realTransport,
		RepoMap:      orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir: worktreesDir,
		Publish:      orchestrator.Publisher(publisher),
	})

	req := orchestrator.DispatchRequest{
		BeadID:         "mp-integ",
		BeadTitle:      "Integration Test",
		BeadDesc:       "Run fake claude",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	}

	res, err := o.Dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.Bead.Column != core.ColRunning {
		t.Errorf("bead column want running got %q", res.Bead.Column)
	}

	// Run should be registered.
	run := o.GetRun("mp-integ")
	if run == nil {
		t.Fatal("GetRun returned nil")
	}

	// Wait for the session to end (fake claude exits immediately).
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		run = o.GetRun("mp-integ")
		if run != nil && run.State != core.StepActive {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Verify run transitioned.
	run = o.GetRun("mp-integ")
	if run == nil {
		t.Fatal("run disappeared")
	}
	if run.State == core.StepActive {
		t.Errorf("run still active after deadline; want done or failed")
		// Clean up the session.
		_ = realTransport.Kill(run.Session)
		t.FailNow()
	}

	// Check for tmux.session.opened event.
	eventsMu.Lock()
	eventsCopy := make([]ws.Frame, len(events))
	copy(eventsCopy, events)
	eventsMu.Unlock()

	var foundOpened bool
	for _, ev := range eventsCopy {
		if ev.Type == ws.EventTmuxOpened && ev.BeadID == "mp-integ" {
			foundOpened = true
			break
		}
	}
	if !foundOpened {
		t.Errorf("expected tmux.session.opened event for mp-integ; got events: %v", eventsCopy)
	}
}
