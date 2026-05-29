package orchestrator

import (
	"context"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/services"
)

// DispatchForService implements the services.OrchestratorDispatcher interface.
// This allows *Orchestrator to be wired into services.BeadService without
// creating an import cycle (services does not import orchestrator).
func (o *Orchestrator) DispatchForService(ctx context.Context, req services.OrchestratorDispatchRequest) (*core.Bead, error) {
	return o.Dispatch(ctx, DispatchRequest{
		BeadID:         req.BeadID,
		BeadTitle:      req.BeadTitle,
		BeadDesc:       req.BeadDesc,
		Agent:          req.Agent,
		Mode:           req.Mode,
		PermissionMode: req.PermissionMode,
	})
}

// AsServiceDispatcher returns a services.OrchestratorDispatcher that delegates
// to this Orchestrator. Use this when wiring into services.BeadService.
func (o *Orchestrator) AsServiceDispatcher() services.OrchestratorDispatcher {
	return &serviceDispatcherAdapter{o: o}
}

// serviceDispatcherAdapter adapts Orchestrator to services.OrchestratorDispatcher.
type serviceDispatcherAdapter struct {
	o *Orchestrator
}

func (a *serviceDispatcherAdapter) Dispatch(ctx context.Context, req services.OrchestratorDispatchRequest) (*core.Bead, error) {
	return a.o.DispatchForService(ctx, req)
}
