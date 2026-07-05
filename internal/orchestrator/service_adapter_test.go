package orchestrator_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/adapter"
	"github.com/gitrgoliveira/muster/internal/adapter/claude"
	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/orchestrator"
	"github.com/gitrgoliveira/muster/internal/services"
)

func TestServiceDispatcherAdapter_ErrMapping(t *testing.T) {
	// Verify the service adapter maps orchestrator sentinel errors to typed
	// *services.ServiceError values with the expected Code. (The full
	// errors.Is mapping table is covered package-internally in
	// maperr_internal_test.go; this test exercises the wiring end-to-end via
	// AsServiceDispatcher().)
	tests := []struct {
		name     string
		setup    func(t *testing.T) orchestrator.Config
		wantCode string
	}{
		{
			name: "no adapter registered → ADAPTER_NOT_FOUND",
			setup: func(t *testing.T) orchestrator.Config {
				return orchestrator.Config{
					Adapters:     adapter.NewRegistry(),
					Transport:    &fakeTransport{},
					RepoMap:      orchestrator.RepoMap{"mp": t.TempDir()},
					WorktreesDir: t.TempDir(),
				}
			},
			wantCode: services.CodeAdapterNotFound,
		},
		{
			name: "claude registered, prefix unmapped → UNMAPPED_PREFIX",
			setup: func(t *testing.T) orchestrator.Config {
				setupFakeClaude(t)
				reg := adapter.NewRegistryWithDefaults(claude.New(claude.Options{}))
				return orchestrator.Config{
					Adapters:     reg,
					Transport:    &fakeTransport{},
					RepoMap:      orchestrator.RepoMap{}, // empty → unmapped
					WorktreesDir: t.TempDir(),
				}
			},
			wantCode: services.CodeUnmappedPrefix,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o := orchestrator.New(tc.setup(t))
			disp := o.AsServiceDispatcher()
			_, err := disp.Dispatch(context.Background(), services.OrchestratorDispatchRequest{
				BeadID:         "mp-abc",
				Agent:          core.AgentClaude,
				Mode:           core.ModeAgent,
				PermissionMode: core.PermAcceptEdits,
			})
			if err == nil {
				t.Fatal("expected error from service dispatcher")
			}
			var se *services.ServiceError
			if !errors.As(err, &se) {
				t.Fatalf("want *services.ServiceError, got %T: %v", err, err)
			}
			if se.Code != tc.wantCode {
				t.Errorf("want Code=%q, got %q (msg=%q)", tc.wantCode, se.Code, se.Message)
			}
		})
	}
}

// TestServiceDispatcherAdapter_ChainConversion verifies that
// serviceDispatcherAdapter.Dispatch converts a
// services.OrchestratorDispatchRequest.Chain of N steps into an
// orchestrator.DispatchRequest.Chain (*StepChain) of length N with matching
// fields, in order, and that the run created by Dispatch actually carries
// that chain — this is the wiring that makes /steps/advance and /steps/loopback
// reachable through the real HTTP API (resolveChain treats a non-nil
// req.Chain as "explicit").
func TestServiceDispatcherAdapter_ChainConversion(t *testing.T) {
	repoPath := initGitRepo(t)
	o, transport := newOrchestratorForTest(t, repoPath)
	transport.deadDead = false // keep the run alive so GetRun sees it

	disp := o.AsServiceDispatcher()
	chain := []services.ChainStepInput{
		{Name: "plan", PermissionMode: core.PermPlan, PromptRef: "plan-ref"},
		{Name: "build", PermissionMode: core.PermAcceptEdits},
	}
	_, err := disp.Dispatch(context.Background(), services.OrchestratorDispatchRequest{
		BeadID:         "mp-chainconv",
		BeadTitle:      "Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
		Chain:          chain,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	run := o.GetRun("mp-chainconv")
	if run == nil {
		t.Fatal("GetRun returned nil")
	}
	if run.Chain == nil {
		t.Fatal("run.Chain is nil, want a chain of length 2")
	}
	got := *run.Chain
	if len(got) != len(chain) {
		t.Fatalf("run.Chain length = %d, want %d", len(got), len(chain))
	}
	for i, want := range chain {
		if got[i].Name != want.Name || got[i].PermissionMode != want.PermissionMode || got[i].PromptRef != want.PromptRef {
			t.Errorf("run.Chain[%d] = %+v, want {Name:%q PermissionMode:%q PromptRef:%q}", i, got[i], want.Name, want.PermissionMode, want.PromptRef)
		}
	}
}

// TestServiceDispatcherAdapter_EmptyChainStaysNil verifies that an empty
// (non-nil) services.OrchestratorDispatchRequest.Chain converts to a nil
// orchestrator.DispatchRequest.Chain, not an empty-but-non-nil *StepChain.
// resolveChain treats any non-nil req.Chain as an explicit override, so a
// spurious empty-but-non-nil chain would short-circuit the configured
// default-chain fallback.
func TestServiceDispatcherAdapter_EmptyChainStaysNil(t *testing.T) {
	repoPath := initGitRepo(t)
	o, transport := newOrchestratorForTest(t, repoPath)
	transport.deadDead = false

	disp := o.AsServiceDispatcher()
	_, err := disp.Dispatch(context.Background(), services.OrchestratorDispatchRequest{
		BeadID:         "mp-chainnil",
		BeadTitle:      "Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
		Chain:          []services.ChainStepInput{}, // empty, non-nil
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	run := o.GetRun("mp-chainnil")
	if run == nil {
		t.Fatal("GetRun returned nil")
	}
	if run.Chain != nil {
		t.Errorf("run.Chain = %v, want nil for an empty input chain", run.Chain)
	}
}

func TestServiceAttacher_GetAttach(t *testing.T) {
	o := orchestrator.New(orchestrator.Config{
		Adapters:     adapter.NewRegistry(),
		Transport:    &fakeTransport{},
		RepoMap:      orchestrator.RepoMap{},
		WorktreesDir: t.TempDir(),
	})

	attacher := o.AsSessionAttacher()
	resp, err := attacher.GetAttach("mp-abc", 0)
	if err != nil {
		t.Fatalf("GetAttach: %v", err)
	}
	if resp.Available {
		t.Error("available should be false with no running session")
	}
}

// TestDispatch_RejectsEmptyWorktreesDir verifies Dispatch fails fast when the
// orchestrator was constructed without a WorktreesDir, rather than letting
// worktree.Ensure create a relative "./<beadID>" worktree under the cwd.
func TestDispatch_RejectsEmptyWorktreesDir(t *testing.T) {
	o := orchestrator.New(orchestrator.Config{
		Adapters:     adapter.NewRegistry(),
		Transport:    &fakeTransport{},
		RepoMap:      orchestrator.RepoMap{"mp": t.TempDir()},
		WorktreesDir: "", // mis-wired
	})

	_, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         "mp-abc",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	})
	if err == nil {
		t.Fatal("want error for empty worktreesDir, got nil")
	}
	if !strings.Contains(err.Error(), "worktreesDir") {
		t.Errorf("error %q should mention worktreesDir", err.Error())
	}
}

func TestServiceAttacher_SendKeys(t *testing.T) {
	o := orchestrator.New(orchestrator.Config{
		Adapters:     adapter.NewRegistry(),
		Transport:    &fakeTransport{},
		RepoMap:      orchestrator.RepoMap{},
		WorktreesDir: t.TempDir(),
	})

	attacher := o.AsSessionAttacher()
	err := attacher.SendKeys("mp-abc", 0, "y\n")
	if err == nil {
		t.Error("SendKeys should return error with no running session")
	}
}

// TestServiceAttacher_SendKeys_BadStepIdx verifies an unsupported step index
// yields INVALID_REQUEST (400), not the misleading BEAD_NOT_FOUND (which would
// claim the bead is missing when it's the step index that's unsupported).
func TestServiceAttacher_SendKeys_BadStepIdx(t *testing.T) {
	o := orchestrator.New(orchestrator.Config{
		Adapters:     adapter.NewRegistry(),
		Transport:    &fakeTransport{},
		RepoMap:      orchestrator.RepoMap{},
		WorktreesDir: t.TempDir(),
	})

	err := o.AsSessionAttacher().SendKeys("mp-abc", 1, "y\n")
	se, ok := err.(*services.ServiceError)
	if !ok {
		t.Fatalf("want *services.ServiceError, got %T (%v)", err, err)
	}
	if se.Code != services.CodeInvalidRequest {
		t.Errorf("code = %q, want %q", se.Code, services.CodeInvalidRequest)
	}
}

func TestPermModeError(t *testing.T) {
	o := orchestrator.New(orchestrator.Config{
		Adapters:     adapter.NewRegistry(),
		Transport:    &fakeTransport{},
		RepoMap:      orchestrator.RepoMap{"mp": t.TempDir()},
		WorktreesDir: t.TempDir(),
	})

	req := orchestrator.DispatchRequest{
		BeadID:         "mp-abc",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: "invalid-mode",
	}

	_, err := o.Dispatch(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for invalid permission mode")
	}
	// Error message should mention "invalid permissionMode"
	if err.Error() == "" {
		t.Error("error message should not be empty")
	}
}
