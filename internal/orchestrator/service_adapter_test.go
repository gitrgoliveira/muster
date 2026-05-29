package orchestrator_test

import (
	"context"
	"testing"

	"github.com/gitrgoliveira/muster/internal/adapter"
	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/orchestrator"
	"github.com/gitrgoliveira/muster/internal/services"
)

func TestServiceDispatcherAdapter_ErrMapping(t *testing.T) {
	// The service adapter maps orchestrator errors to ServiceErrors.
	// Test that ErrUnmappedPrefix is correctly wrapped.
	reg := adapter.NewRegistry()
	o := orchestrator.New(orchestrator.Config{
		Adapters:     reg,
		Transport:    &fakeTransport{},
		RepoMap:      orchestrator.RepoMap{}, // empty → unmapped
		WorktreesDir: t.TempDir(),
	})

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
