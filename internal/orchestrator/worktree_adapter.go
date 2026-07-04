package orchestrator

import (
	"context"
	"io"

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

// Verify worktreeAccessorAdapter implements services.WorktreeAccessor.
var _ services.WorktreeAccessor = (*worktreeAccessorAdapter)(nil)
