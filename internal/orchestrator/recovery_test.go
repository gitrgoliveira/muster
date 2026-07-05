package orchestrator_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gitrgoliveira/muster/internal/adapter"
	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/orchestrator"
	"github.com/gitrgoliveira/muster/internal/tmux"
)

// controlledTransport is a fakeTransport with configurable list output.
type controlledTransport struct {
	fakeTransport
	pipeReader io.ReadCloser
}

func (c *controlledTransport) Pipe(name string) (io.ReadCloser, error) {
	if c.pipeReader != nil {
		return c.pipeReader, nil
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func TestRecoverSessions_LiveSession(t *testing.T) {
	transport := &controlledTransport{
		fakeTransport: fakeTransport{
			// Not dead → session is live.
			deadDead: false,
			listReturns: []tmux.Session{
				{
					Name:      "muster/mp-abc/0/0",
					BeadID:    "mp-abc",
					StepIdx:   0,
					Loop:      0,
					StartedAt: time.Now(),
				},
			},
		},
	}

	// This session is live (deadDead=false), so recovery starts a watchRun
	// goroutine that polls DeadStatus forever. Flip forceDead at cleanup so it
	// observes death and exits instead of leaking for the rest of the suite.
	t.Cleanup(func() { transport.forceDead.Store(true) })

	o := orchestrator.New(orchestrator.Config{
		Adapters:     adapter.NewRegistry(),
		Transport:    transport,
		RepoMap:      orchestrator.RepoMap{"mp": "/tmp/repo"},
		WorktreesDir: t.TempDir(),
	})

	// Before recovery, no runs.
	if o.GetRun("mp-abc") != nil {
		t.Fatal("run should not exist before recovery")
	}

	o.RecoverSessions(context.Background())

	// After recovery, the run should be re-registered.
	run := o.GetRun("mp-abc")
	if run == nil {
		t.Fatal("run should exist after recovery")
	}
	if run.State != core.StepActive {
		t.Errorf("run state want active got %q", run.State)
	}
	if run.Session != "muster/mp-abc/0/0" {
		t.Errorf("session want muster/mp-abc/0/0 got %q", run.Session)
	}
}

func TestRecoverSessions_DeadSessionKilled(t *testing.T) {
	transport := &controlledTransport{
		fakeTransport: fakeTransport{
			// Dead session.
			deadDead: true,
			deadCode: 1,
			listReturns: []tmux.Session{
				{
					Name:    "muster/mp-dead/0/0",
					BeadID:  "mp-dead",
					StepIdx: 0,
				},
			},
		},
	}

	o := orchestrator.New(orchestrator.Config{
		Adapters:     adapter.NewRegistry(),
		Transport:    transport,
		RepoMap:      orchestrator.RepoMap{},
		WorktreesDir: t.TempDir(),
	})

	o.RecoverSessions(context.Background())

	// Dead session should be killed — no run registered.
	if run := o.GetRun("mp-dead"); run != nil {
		t.Error("dead session should not register a run")
	}
	if !transport.killCalled.Load() {
		t.Error("Kill should have been called for dead session")
	}
}

func TestRecoverSessions_InvalidBeadIDKilled(t *testing.T) {
	// A session name parses to a bead ID that doesn't match the expected
	// pattern. Recovery must kill it and must NOT register a run.
	transport := &controlledTransport{
		fakeTransport: fakeTransport{
			deadDead: false, // live, so only the validation guard can reject it
			listReturns: []tmux.Session{
				{
					Name:    "muster/bad..id/0/0",
					BeadID:  "bad..id",
					StepIdx: 0,
					Loop:    0,
				},
			},
		},
	}

	o := orchestrator.New(orchestrator.Config{
		Adapters:     adapter.NewRegistry(),
		Transport:    transport,
		RepoMap:      orchestrator.RepoMap{},
		WorktreesDir: t.TempDir(),
	})

	o.RecoverSessions(context.Background())

	if run := o.GetRun("bad..id"); run != nil {
		t.Error("session with invalid bead ID should not register a run")
	}
	if !transport.killCalled.Load() {
		t.Error("Kill should have been called for the invalid-bead-ID session")
	}
	if count := o.RunCount(); count != 0 {
		t.Errorf("RunCount want 0 got %d", count)
	}
}

// TestRecoverSessions_MalformedIndicesKilled is the M4 T053 rewrite of the
// former TestRecoverSessions_UnsupportedIndicesKilled.
//
// New boundary (M4 US4 T053a): only genuinely malformed/*negative* indices are
// killed. A session with StepIdx ≥ 0 AND Loop ≥ 0 is re-registered (not killed)
// so a live multi-step / looped agent survives a muster restart. Negative
// StepIdx or negative Loop remain kill-on-sight (they cannot be produced by a
// real muster session name).
func TestRecoverSessions_MalformedIndicesKilled(t *testing.T) {
	// Only malformed (negative) values are killed; a non-negative StepIdx or
	// Loop is re-registered (see TestRecoverSessions_NonzeroStepIdx and
	// TestRecoverSessions_NonzeroLoop).
	cases := []struct {
		name    string
		session tmux.Session
	}{
		{"negative step", tmux.Session{Name: "muster/mp-abc/-1/0", BeadID: "mp-abc", StepIdx: -1, Loop: 0}},
		{"negative loop", tmux.Session{Name: "muster/mp-abc/0/-1", BeadID: "mp-abc", StepIdx: 0, Loop: -1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			transport := &controlledTransport{
				fakeTransport: fakeTransport{
					deadDead:    false, // live, so only the validation guard can reject it
					listReturns: []tmux.Session{tc.session},
				},
			}
			o := orchestrator.New(orchestrator.Config{
				Adapters:     adapter.NewRegistry(),
				Transport:    transport,
				RepoMap:      orchestrator.RepoMap{},
				WorktreesDir: t.TempDir(),
			})

			o.RecoverSessions(context.Background())

			if run := o.GetRun("mp-abc"); run != nil {
				t.Error("session with malformed indices should not register a run")
			}
			if !transport.killCalled.Load() {
				t.Error("Kill should have been called for the malformed-index session")
			}
			if count := o.RunCount(); count != 0 {
				t.Errorf("RunCount want 0 got %d", count)
			}
		})
	}
}

// TestRecoverSessions_NonzeroStepIdx verifies that a session with a valid bead
// ID and a non-negative StepIdx (e.g. step 1 of a multi-step chain) is
// re-registered as an active run rather than killed (M4 T053a relaxed guard).
//
// The recovered run reconstructs as a single-step run pinned at that StepIdx
// with no Chain — Advance/LoopBack are refused until the bead is re-dispatched.
func TestRecoverSessions_NonzeroStepIdx(t *testing.T) {
	// A live multi-step session at step 2 must survive a restart.
	transport := &controlledTransport{
		fakeTransport: fakeTransport{
			deadDead: false,
			listReturns: []tmux.Session{
				{Name: "muster/mp-abc/2/0", BeadID: "mp-abc", StepIdx: 2, Loop: 0, StartedAt: time.Now()},
			},
		},
	}
	t.Cleanup(func() { transport.forceDead.Store(true) })

	o := orchestrator.New(orchestrator.Config{
		Adapters:     adapter.NewRegistry(),
		Transport:    transport,
		RepoMap:      orchestrator.RepoMap{},
		WorktreesDir: t.TempDir(),
	})

	o.RecoverSessions(context.Background())

	run := o.GetRun("mp-abc")
	if run == nil {
		t.Fatal("session with non-negative StepIdx must be re-registered, not killed")
	}
	if transport.killCalled.Load() {
		t.Error("Kill must NOT be called for a non-negative StepIdx session")
	}
	if run.State != core.StepActive {
		t.Errorf("recovered run state want active got %q", run.State)
	}
	if run.StepIdx != 2 {
		t.Errorf("recovered run StepIdx want 2 got %d", run.StepIdx)
	}
}

// TestRecoverSessions_NonzeroLoop verifies that a session with a non-negative
// Loop (a looped step) is re-registered rather than killed — symmetric with
// StepIdx (M4 US4 T053a; resolves the prior Loop!=0 kill limitation). The Loop
// counter is preserved on the recovered run.
func TestRecoverSessions_NonzeroLoop(t *testing.T) {
	transport := &controlledTransport{
		fakeTransport: fakeTransport{
			deadDead: false,
			listReturns: []tmux.Session{
				{Name: "muster/mp-abc/1/3", BeadID: "mp-abc", StepIdx: 1, Loop: 3, StartedAt: time.Now()},
			},
		},
	}
	t.Cleanup(func() { transport.forceDead.Store(true) })

	o := orchestrator.New(orchestrator.Config{
		Adapters:     adapter.NewRegistry(),
		Transport:    transport,
		RepoMap:      orchestrator.RepoMap{},
		WorktreesDir: t.TempDir(),
	})

	o.RecoverSessions(context.Background())

	run := o.GetRun("mp-abc")
	if run == nil {
		t.Fatal("session with non-negative Loop must be re-registered, not killed")
	}
	if transport.killCalled.Load() {
		t.Error("Kill must NOT be called for a non-negative Loop session")
	}
	if run.Loop != 3 {
		t.Errorf("recovered run Loop want 3 got %d", run.Loop)
	}
}

// TestRecoverSessions_RecoveredRunIsInFlight verifies that a recovered run is
// treated as in-flight for idempotency: a re-dispatch of the same bead joins
// the recovered run (Joined:true) rather than starting a new one (M4 T053d).
func TestRecoverSessions_RecoveredRunIsInFlight(t *testing.T) {
	transport := &controlledTransport{
		fakeTransport: fakeTransport{
			deadDead: false,
			listReturns: []tmux.Session{
				{Name: "muster/mp-abc/1/0", BeadID: "mp-abc", StepIdx: 1, Loop: 0, StartedAt: time.Now()},
			},
		},
	}
	t.Cleanup(func() { transport.forceDead.Store(true) })

	o := orchestrator.New(orchestrator.Config{
		Adapters:     adapter.NewRegistry(),
		Transport:    transport,
		RepoMap:      orchestrator.RepoMap{},
		WorktreesDir: t.TempDir(),
	})

	o.RecoverSessions(context.Background())

	// The run should be registered.
	if o.GetRun("mp-abc") == nil {
		t.Fatal("recovered run should be registered")
	}

	// Re-dispatch must join the recovered in-flight run.
	res, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         "mp-abc",
		BeadTitle:      "Re-dispatch",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	})
	if err != nil {
		t.Fatalf("re-dispatch of recovered bead: %v", err)
	}
	if !res.Joined {
		t.Error("re-dispatch of a recovered bead must return Joined:true")
	}
}

func TestRecoverSessions_EmptyList(t *testing.T) {
	transport := &controlledTransport{
		fakeTransport: fakeTransport{
			listReturns: nil, // no sessions
		},
	}
	o := orchestrator.New(orchestrator.Config{
		Adapters:     adapter.NewRegistry(),
		Transport:    transport,
		RepoMap:      orchestrator.RepoMap{},
		WorktreesDir: t.TempDir(),
	})

	// Should not panic with empty list.
	o.RecoverSessions(context.Background())

	if count := o.RunCount(); count != 0 {
		t.Errorf("RunCount want 0 got %d", count)
	}
}
