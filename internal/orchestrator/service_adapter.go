package orchestrator

import (
	"context"
	"errors"

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
	bead, err := a.o.Dispatch(ctx, DispatchRequest{
		BeadID:         req.BeadID,
		BeadTitle:      req.BeadTitle,
		BeadDesc:       req.BeadDesc,
		Agent:          req.Agent,
		Mode:           req.Mode,
		PermissionMode: req.PermissionMode,
	})
	return bead, mapDispatchError(err)
}

// mapDispatchError translates orchestrator sentinel errors into typed
// *services.ServiceError values using errors.Is/As (not message-string
// matching). This is the orchestrator side of the boundary — it can reference
// both the orchestrator sentinels and the services error codes without an
// import cycle (services does not import orchestrator).
func mapDispatchError(err error) error {
	if err == nil {
		return nil
	}
	var se *services.ServiceError
	if errors.As(err, &se) {
		return se // already typed (e.g. from a fake)
	}
	var pme *PermModeError
	switch {
	case errors.Is(err, ErrRunAlreadyActive):
		return &services.ServiceError{Code: services.CodeRunAlreadyActive, Message: err.Error()}
	case errors.Is(err, ErrUnmappedPrefix):
		return &services.ServiceError{Code: services.CodeUnmappedPrefix, Message: err.Error()}
	case errors.Is(err, ErrAdapterNotFound):
		return &services.ServiceError{Code: services.CodeAdapterNotFound, Message: err.Error()}
	case errors.Is(err, ErrAdapterNotInstalled):
		return &services.ServiceError{Code: services.CodeAdapterNotInstalled, Message: err.Error()}
	case errors.Is(err, ErrAdapterNotLoggedIn):
		return &services.ServiceError{Code: services.CodeAdapterNotLoggedIn, Message: err.Error()}
	case errors.Is(err, ErrNoPermissionMode), errors.Is(err, ErrUnsupportedMode), errors.As(err, &pme):
		return &services.ServiceError{Code: services.CodeInvalidRequest, Message: err.Error()}
	default:
		return err // services maps anything unrecognized → Internal
	}
}

// AsSessionAttacher returns a services.SessionAttacher backed by this Orchestrator.
func (o *Orchestrator) AsSessionAttacher() services.SessionAttacher {
	return o
}

// Verify Orchestrator implements services.SessionAttacher.
var _ services.SessionAttacher = (*Orchestrator)(nil)
