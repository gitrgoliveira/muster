//go:build windows

// jj backend stub for Windows. jj is not supported on Windows in this codebase
// (the Unix build uses JJ_CONFIG=/dev/null, which is Unix-only), but wt.For and
// orchestrator.New reference jjBackend/NewJJBackend unconditionally, so the
// package must still compile on Windows. Every operation reports the backend as
// unavailable, so callers get a clean VCS_UNAVAILABLE (412) rather than a build
// error or a confusing runtime panic.

package wt

import (
	"context"
	"io"
)

// jjBackend on Windows is a stub whose operations all report ErrVCSUnavailable.
type jjBackend struct{}

// NewJJBackend returns the Windows jj stub. worktreesDir is ignored — jj is
// unavailable on Windows, so every method returns ErrVCSUnavailable.
func NewJJBackend(worktreesDir string) Backend {
	return &jjBackend{}
}

func (j *jjBackend) Status(ctx context.Context, beadID string) (WorktreeStatus, error) {
	return WorktreeStatus{}, ErrVCSUnavailable
}

func (j *jjBackend) Create(ctx context.Context, worktreesDir, srcRepo, beadID string) (string, error) {
	return "", ErrVCSUnavailable
}

func (j *jjBackend) DiffSummary(ctx context.Context, beadID string) ([]FileChange, error) {
	return nil, ErrVCSUnavailable
}

func (j *jjBackend) Diff(ctx context.Context, beadID, path string) (io.ReadCloser, error) {
	return nil, ErrVCSUnavailable
}

func (j *jjBackend) Finalize(ctx context.Context, beadID, msg string) error { return ErrNotImplemented }

func (j *jjBackend) Push(ctx context.Context, beadID string) error { return ErrNotImplemented }

func (j *jjBackend) Remove(ctx context.Context, beadID string) error { return ErrNotImplemented }
