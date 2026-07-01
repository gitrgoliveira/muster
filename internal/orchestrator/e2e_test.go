//go:build e2e

package orchestrator_test

// TestE2E runs the real end-to-end M2 dispatch flow against the user's real
// claude CLI and real tmux. It is ONLY run via `make test-e2e` (or
// `go test -tags=e2e -run TestE2E`).
//
// Prerequisites:
//   - `claude` installed and logged in (`claude auth status --json` → loggedIn=true)
//   - `tmux` installed (≥ 3.2)
//
// The test uses a trivial prompt that consumes minimal Max-plan usage
// (asking claude to output a single word). It skips gracefully if either
// prerequisite is missing.
//
// Cleanup: kills the muster/<e2eBeadID>/* tmux session and removes temp dirs.

import (
	"context"
	"encoding/json"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/gitrgoliveira/muster/internal/adapter"
	adapterclaude "github.com/gitrgoliveira/muster/internal/adapter/claude"
	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/orchestrator"
	"github.com/gitrgoliveira/muster/internal/tmux"
	"github.com/gitrgoliveira/muster/internal/ws"
)

// e2eBeadID is the bead ID used throughout this test. It must match the
// validated bead-ID format `^[a-z]+-[0-9a-z]+$` (see idPattern in
// internal/api/beads/handlers.go and beadIDPattern in
// internal/orchestrator/recovery.go) so the test exercises a representative
// ID: a suffix with a second hyphen would fail that validation and would be
// skipped by restart recovery. The "mp" prefix is mapped to the temp repo in
// the orchestrator config below.
const e2eBeadID = "mp-e2e01"

// checkE2EPrerequisites skips the test if claude or tmux is absent/unauthed.
func checkE2EPrerequisites(t *testing.T) {
	t.Helper()

	// Check tmux.
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed; skipping e2e test. Install tmux >= 3.2 to run.")
	}

	// Check claude installed.
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude not installed; skipping e2e test. Install Claude Code CLI to run.")
	}

	// Check claude logged in.
	out, err := exec.Command("claude", "auth", "status", "--json").Output()
	if err != nil {
		t.Skipf("claude auth status failed: %v; skipping e2e test.", err)
	}
	var status struct {
		LoggedIn bool `json:"loggedIn"`
	}
	if err := json.Unmarshal(out, &status); err != nil {
		t.Skipf("cannot parse claude auth status: %v; skipping e2e test.", err)
	}
	if !status.LoggedIn {
		t.Skip("claude is not logged in (run: claude auth login); skipping e2e test.")
	}
}

func TestE2E_Dispatch_RealClaude_RealTmux(t *testing.T) {
	checkE2EPrerequisites(t)

	t.Log("E2E test starting: real claude + real tmux")

	// Set up a temporary git repo.
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()

	// Collect WS events.
	var eventsMu sync.Mutex
	var events []ws.Frame
	publisher := func(f ws.Frame) {
		eventsMu.Lock()
		events = append(events, f)
		eventsMu.Unlock()
	}

	// Build real orchestrator.
	realTransport := tmux.NewRealManager("")
	reg := adapter.NewRegistry()
	reg.Register(adapterclaude.New(adapterclaude.Options{}))

	o := orchestrator.New(orchestrator.Config{
		Adapters:     reg,
		Transport:    realTransport,
		RepoMap:      orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir: worktreesDir,
		Publish:      orchestrator.Publisher(publisher),
		// Generous timeout so a slow response doesn't hang forever.
		RunTimeout: 90 * time.Second,
	})

	// Trivial prompt: costs ~minimal tokens, asks for a single word.
	req := orchestrator.DispatchRequest{
		BeadID:    e2eBeadID,
		BeadTitle: "E2E test",
		BeadDesc:  "Reply with exactly one word: 'done'. Nothing else.",
		Agent:     core.AgentClaude,
		Mode:      core.ModePlan, // plan mode: read-only, no filesystem changes
		// plan mode maps to --permission-mode plan (no autonomy needed).
		PermissionMode: core.PermPlan,
	}

	// Cleanup: kill any lingering session.
	t.Cleanup(func() {
		sessionName := tmux.SessionName(e2eBeadID, 0, 0)
		_ = realTransport.Kill(sessionName)
	})

	t.Log("Dispatching bead...")
	bead, err := o.Dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if bead.Column != core.ColRunning {
		t.Errorf("bead column want running got %q", bead.Column)
	}
	t.Logf("Bead dispatched; column=%s", bead.Column)

	// Verify run registered.
	run := o.GetRun(e2eBeadID)
	if run == nil {
		t.Fatal("GetRun returned nil immediately after Dispatch")
	}
	t.Logf("Session: %s", run.Session)

	// Wait up to 120 seconds for the run to complete (claude may take time).
	t.Log("Waiting for run to complete...")
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		run = o.GetRun(e2eBeadID)
		if run != nil && run.State != core.StepActive {
			t.Logf("Run completed: state=%s exitCode=%d", run.State, run.ExitCode)
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	run = o.GetRun(e2eBeadID)
	if run == nil {
		t.Fatal("run disappeared from registry")
	}
	if run.State == core.StepActive {
		t.Errorf("run still active after 120s deadline; expected done/failed")
	}

	// Check for runlog.line events.
	eventsMu.Lock()
	eventsCopy := make([]ws.Frame, len(events))
	copy(eventsCopy, events)
	eventsMu.Unlock()

	var foundOpened, foundRunlog, foundClosed bool
	for _, ev := range eventsCopy {
		switch ev.Type {
		case ws.EventTmuxOpened:
			if ev.BeadID == e2eBeadID {
				foundOpened = true
			}
		case ws.EventRunlogLine:
			if ev.BeadID == e2eBeadID {
				foundRunlog = true
			}
		case ws.EventTmuxClosed:
			if ev.BeadID == e2eBeadID {
				foundClosed = true
			}
		}
	}

	if !foundOpened {
		t.Error("expected tmux.session.opened event")
	}
	if !foundRunlog {
		t.Log("warning: no runlog.line events received (pipe may not have delivered in time)")
	}
	if !foundClosed {
		t.Log("warning: no tmux.session.closed event received (may still be processing)")
	}

	t.Logf("E2E complete: opened=%v runlog=%v closed=%v state=%s",
		foundOpened, foundRunlog, foundClosed, run.State)
}
