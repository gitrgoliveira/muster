package orchestrator

import (
	"errors"
	"fmt"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/gitrgoliveira/muster/internal/tmux"
)

// GetAttach implements services.SessionAttacher.
// Returns the tmux attach command for a running step.
// M2: only stepIdx=0 is valid.
func (o *Orchestrator) GetAttach(beadID string, stepIdx int) (*services.AttachResponse, error) {
	if stepIdx != 0 {
		return &services.AttachResponse{
			Available: false,
			Reason:    fmt.Sprintf("step index %d not supported (M2 only supports step 0)", stepIdx),
		}, nil
	}

	run := o.GetRun(beadID)
	if run == nil || run.State != core.StepActive {
		return &services.AttachResponse{
			Available: false,
			Reason:    "step is not currently running",
		}, nil
	}

	if run.Session == "" {
		// Dispatch registers a StepActive reservation before the tmux session
		// name is known (it's set only after Detect/worktree.Ensure/Invoke/Spawn
		// all succeed). Attaching during that window would otherwise build a
		// command against an empty session name (RealManager.Attach never
		// errors — it just string-concatenates), reporting Available: true
		// with a bogus/unsafe "tmux attach -t " command.
		return &services.AttachResponse{
			Available: false,
			Reason:    "step is starting (tmux session not yet available)",
		}, nil
	}

	if isFallbackTransport(o.transport) {
		// Fallback mode: no tmux session to attach to. (run.Session is non-empty
		// in fallback too — FallbackManager keys its in-memory sessions by name —
		// so check the transport type, not run.Session.)
		return &services.AttachResponse{
			Available: false,
			Reason:    "tmux not available (fallback transport)",
		}, nil
	}

	cmd, err := o.transport.Attach(run.Session)
	if err != nil {
		return &services.AttachResponse{
			Available: false,
			Reason:    fmt.Sprintf("attach unavailable: %v", err),
		}, nil
	}

	return &services.AttachResponse{
		Available: true,
		Command:   cmd,
		Session:   run.Session,
	}, nil
}

// SendKeys implements services.SessionAttacher.
// Forwards keystrokes to the running step's tmux pane.
func (o *Orchestrator) SendKeys(beadID string, stepIdx int, keys string) error {
	if stepIdx != 0 {
		return &services.ServiceError{
			Code:    services.CodeNotFound,
			Message: fmt.Sprintf("step %d not found", stepIdx),
		}
	}

	run := o.GetRun(beadID)
	if run == nil || run.State != core.StepActive {
		return &services.ServiceError{
			Code:    services.CodeInvalidState,
			Message: "step is not currently running",
		}
	}

	if run.Session == "" {
		// Same reservation-window gap as GetAttach: Dispatch registers a
		// StepActive reservation before the tmux session name is known.
		// Rejecting here (instead of forwarding an empty name to the
		// transport, which fails with a confusing tmux error) gives the
		// client a clear, retryable "not ready yet" signal.
		return &services.ServiceError{
			Code:    services.CodeInvalidState,
			Message: "step is starting (tmux session not yet available)",
		}
	}

	if isFallbackTransport(o.transport) {
		// Same as GetAttach: run.Session is non-empty under fallback, so the
		// transport type is the reliable signal.
		return &services.ServiceError{
			Code:    services.CodeCLIUnavailable,
			Message: "tmux not available (fallback transport)",
		}
	}

	if err := o.transport.Send(run.Session, keys); err != nil {
		if errors.Is(err, tmux.ErrAttachUnavailable) {
			return &services.ServiceError{
				Code:    services.CodeCLIUnavailable,
				Message: "send unavailable: " + err.Error(),
			}
		}
		return &services.ServiceError{
			Code:    services.CodeInternal,
			Message: "send failed: " + err.Error(),
		}
	}
	return nil
}
