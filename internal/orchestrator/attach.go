package orchestrator

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/gitrgoliveira/muster/internal/tmux"
)

// runGuardKind identifies which pre-flight guard rejected a GetAttach/SendKeys
// call, so resolveActiveRun can share one guard sequence while each caller
// still renders its own response shape (AttachResponse.Reason vs a coded
// services.ServiceError).
type runGuardKind int

const (
	guardStepIdx runGuardKind = iota
	guardNotRunning
	guardStarting
	guardFallback
)

// runGuardError is returned by resolveActiveRun when a guard rejects the
// request; stepIdx is carried along so callers can render it into their
// step-index-specific message.
type runGuardError struct {
	kind    runGuardKind
	stepIdx int
}

func (e *runGuardError) Error() string {
	switch e.kind {
	case guardStepIdx:
		return fmt.Sprintf("step index %d not supported (M2 only supports step 0)", e.stepIdx)
	case guardNotRunning:
		return "step is not currently running"
	case guardStarting:
		return "step is starting (tmux session not yet available)"
	case guardFallback:
		return "tmux not available (fallback transport)"
	default:
		return "step not available"
	}
}

// resolveActiveRun applies the guard sequence shared by GetAttach and
// SendKeys: stepIdx validity (M2 only supports step 0), run existence/active
// state, tmux session assignment, and transport availability. Returns the
// live *Run on success, or a *runGuardError identifying which guard failed.
func (o *Orchestrator) resolveActiveRun(beadID string, stepIdx int) (*Run, *runGuardError) {
	if stepIdx != 0 {
		return nil, &runGuardError{kind: guardStepIdx, stepIdx: stepIdx}
	}

	run := o.GetRun(beadID)
	if run == nil || run.State != core.StepActive {
		return nil, &runGuardError{kind: guardNotRunning}
	}

	if run.Session == "" {
		// Dispatch registers a StepActive reservation before the tmux session
		// name is known (it's set only after Detect/worktree.Ensure/Invoke/Spawn
		// all succeed). Proceeding during that window would otherwise build a
		// command against an empty session name (RealManager.Attach never
		// errors — it just string-concatenates) or forward keys to nothing,
		// so reject with a clear, retryable "not ready yet" signal instead.
		return nil, &runGuardError{kind: guardStarting}
	}

	if isFallbackTransport(o.transport) {
		// Fallback mode: no tmux session to attach/send to. (run.Session is
		// non-empty in fallback too — FallbackManager keys its in-memory
		// sessions by name — so check the transport type, not run.Session.)
		return nil, &runGuardError{kind: guardFallback}
	}

	return run, nil
}

// GetAttach implements services.SessionAttacher.
// Returns the tmux attach command for a running step.
// M2: only stepIdx=0 is valid.
func (o *Orchestrator) GetAttach(beadID string, stepIdx int) (*services.AttachResponse, error) {
	run, gerr := o.resolveActiveRun(beadID, stepIdx)
	if gerr != nil {
		return &services.AttachResponse{Available: false, Reason: gerr.Error()}, nil
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
		Pane:      run.Pane,
	}, nil
}

// SendKeys implements services.SessionAttacher.
// Forwards keystrokes to the running step's tmux pane.
func (o *Orchestrator) SendKeys(beadID string, stepIdx int, keys string) error {
	run, gerr := o.resolveActiveRun(beadID, stepIdx)
	if gerr != nil {
		switch gerr.kind {
		case guardStepIdx:
			return &services.ServiceError{Code: services.CodeNotFound, Message: fmt.Sprintf("step %d not found", stepIdx)}
		case guardFallback:
			return &services.ServiceError{Code: services.CodeCLIUnavailable, Message: gerr.Error()}
		default: // guardNotRunning, guardStarting
			return &services.ServiceError{Code: services.CodeInvalidState, Message: gerr.Error()}
		}
	}

	if err := o.transport.Send(run.Session, keys); err != nil {
		if errors.Is(err, tmux.ErrAttachUnavailable) {
			return &services.ServiceError{
				Code:    services.CodeCLIUnavailable,
				Message: "send unavailable (tmux transport)",
			}
		}
		// Log the internal detail (raw tmux error includes the subcommand and
		// session name) server-side; return a generic client-facing message,
		// consistent with the service boundary's handling of internal errors.
		slog.Error("SendKeys: tmux send failed", "bead", beadID, "err", err)
		return &services.ServiceError{
			Code:    services.CodeInternal,
			Message: "send failed due to an internal error",
		}
	}
	return nil
}
