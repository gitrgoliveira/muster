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
	if !transport.killCalled {
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
	if !transport.killCalled {
		t.Error("Kill should have been called for the invalid-bead-ID session")
	}
	if count := o.RunCount(); count != 0 {
		t.Errorf("RunCount want 0 got %d", count)
	}
}

func TestRecoverSessions_UnsupportedIndicesKilled(t *testing.T) {
	// A session with a valid bead ID but an unsupported step/loop index
	// (M2 only creates 0/0). A locally-planted "muster/mp-abc/-1/0" must be
	// killed and must NOT register a phantom run — otherwise it would block
	// dispatch for mp-abc forever while being unattachable (idx=0 only).
	cases := []struct {
		name    string
		session tmux.Session
	}{
		{"negative step", tmux.Session{Name: "muster/mp-abc/-1/0", BeadID: "mp-abc", StepIdx: -1, Loop: 0}},
		{"nonzero step", tmux.Session{Name: "muster/mp-abc/1/0", BeadID: "mp-abc", StepIdx: 1, Loop: 0}},
		{"nonzero loop", tmux.Session{Name: "muster/mp-abc/0/2", BeadID: "mp-abc", StepIdx: 0, Loop: 2}},
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
				t.Error("session with unsupported step/loop indices should not register a run")
			}
			if !transport.killCalled {
				t.Error("Kill should have been called for the unsupported-index session")
			}
			if count := o.RunCount(); count != 0 {
				t.Errorf("RunCount want 0 got %d", count)
			}
		})
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
