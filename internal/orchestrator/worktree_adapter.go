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

// WorktreeStatus delegates to wt.GitStatusAt using the orchestrator's
// configured worktreesDir.
func (a *worktreeAccessorAdapter) WorktreeStatus(ctx context.Context, beadID string) (wt.WorktreeStatus, error) {
	return wt.GitStatusAt(ctx, a.o.worktreesDir, beadID)
}

// DiffSummary delegates to wt.GitDiffSummaryAt using the orchestrator's
// configured worktreesDir.
func (a *worktreeAccessorAdapter) DiffSummary(ctx context.Context, beadID string) ([]wt.FileChange, error) {
	return wt.GitDiffSummaryAt(ctx, a.o.worktreesDir, beadID)
}

// Diff delegates to wt.GitDiffAt using the orchestrator's worktreesDir.
// path must already be validated by safeRelPath before reaching this method.
func (a *worktreeAccessorAdapter) Diff(ctx context.Context, beadID, path string) (io.ReadCloser, error) {
	return wt.GitDiffAt(ctx, a.o.worktreesDir, beadID, path)
}

// DefaultVCS returns the string representation of the orchestrator's default VCS.
func (a *worktreeAccessorAdapter) DefaultVCS() string {
	return string(a.o.defaultVCS)
}

// Verify worktreeAccessorAdapter implements services.WorktreeAccessor.
var _ services.WorktreeAccessor = (*worktreeAccessorAdapter)(nil)
