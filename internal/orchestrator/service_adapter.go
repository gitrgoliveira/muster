package orchestrator

import (
	"context"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/services"
)

// AsServiceDispatcher returns a services.OrchestratorDispatcher that delegates
// to this Orchestrator. Use this when wiring into services.BeadService.
func (o *Orchestrator) AsServiceDispatcher() services.OrchestratorDispatcher {
	return &serviceDispatcherAdapter{o: o}
}

// serviceDispatcherAdapter adapts Orchestrator to services.OrchestratorDispatcher.
type serviceDispatcherAdapter struct {
	o *Orchestrator
}

// Dispatch implements services.OrchestratorDispatcher by translating the
// import-cycle-avoiding request type into the orchestrator's own DispatchRequest.
func (a *serviceDispatcherAdapter) Dispatch(ctx context.Context, req services.OrchestratorDispatchRequest) (*core.Bead, error) {
	return a.o.Dispatch(ctx, DispatchRequest{
		BeadID:         req.BeadID,
		BeadTitle:      req.BeadTitle,
		BeadDesc:       req.BeadDesc,
		Agent:          req.Agent,
		Mode:           req.Mode,
		PermissionMode: req.PermissionMode,
	})
}

// AsSessionAttacher returns a services.SessionAttacher backed by this Orchestrator.
func (o *Orchestrator) AsSessionAttacher() services.SessionAttacher {
	return o
}

// Verify Orchestrator implements services.SessionAttacher.
var _ services.SessionAttacher = (*Orchestrator)(nil)
