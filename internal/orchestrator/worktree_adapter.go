package orchestrator

import (
	"context"
	"io"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/gitrgoliveira/muster/internal/wt"
)

// AsWorktreeAccessor returns a services.WorktreeAccessor backed by this
// Orchestrator's wt.Backend. Use this when wiring into services.BeadService.
func (o *Orchestrator) AsWorktreeAccessor() services.WorktreeAccessor {
	return &worktreeAccessorAdapter{o: o}
}

// worktreeAccessorAdapter adapts Orchestrator to services.WorktreeAccessor.
type worktreeAccessorAdapter struct {
	o *Orchestrator
}

// WorktreeStatus delegates to the orchestrator's selected wt.Backend so reads
// honor the configured VCS (git or jj) rather than always using git.
func (a *worktreeAccessorAdapter) WorktreeStatus(ctx context.Context, beadID string) (wt.WorktreeStatus, error) {
	return a.o.backend.Status(ctx, beadID)
}

// DiffSummary delegates to the orchestrator's selected wt.Backend.
func (a *worktreeAccessorAdapter) DiffSummary(ctx context.Context, beadID string) ([]wt.FileChange, error) {
	return a.o.backend.DiffSummary(ctx, beadID)
}

// Diff delegates to the orchestrator's selected wt.Backend.
// path must already be validated by safeRelPath before reaching this method.
func (a *worktreeAccessorAdapter) Diff(ctx context.Context, beadID, path string) (io.ReadCloser, error) {
	return a.o.backend.Diff(ctx, beadID, path)
}

// DefaultVCS returns the string representation of the orchestrator's default VCS.
func (a *worktreeAccessorAdapter) DefaultVCS() string {
	return string(a.o.defaultVCS)
}

// Finalize delegates to the orchestrator's selected wt.Backend.
func (a *worktreeAccessorAdapter) Finalize(ctx context.Context, beadID, message string) (bool, error) {
	return a.o.backend.Finalize(ctx, beadID, message)
}

// Push delegates to the orchestrator's selected wt.Backend.
func (a *worktreeAccessorAdapter) Push(ctx context.Context, beadID, remote string) error {
	return a.o.backend.Push(ctx, beadID, remote)
}

// Remove delegates to the orchestrator's selected wt.Backend.
func (a *worktreeAccessorAdapter) Remove(ctx context.Context, beadID string) error {
	return a.o.backend.Remove(ctx, beadID)
}

// BeadRunState returns the current run state for the given bead.
// Returns an empty StepStatus (zero value) when no run record exists.
func (a *worktreeAccessorAdapter) BeadRunState(beadID string) core.StepStatus {
	run := a.o.GetRun(beadID)
	if run == nil {
		return ""
	}
	return run.State
}

// Verify worktreeAccessorAdapter implements services.WorktreeAccessor.
var _ services.WorktreeAccessor = (*worktreeAccessorAdapter)(nil)
