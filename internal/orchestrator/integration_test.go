package orchestrator_test

import (
	"context"
	"os"
	"os/exec"
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

// skipIfNoRealTmux skips the test if tmux is not installed.
func skipIfNoRealTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed; skipping integration test")
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

	bead, err := o.Dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if bead.Column != core.ColRunning {
		t.Errorf("bead column want running got %q", bead.Column)
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
