package wt

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
)

// VCS identifies a version-control backend.
type VCS string

const (
	// VCSGit selects the git backend (wraps internal/worktree.Ensure).
	VCSGit VCS = "git"
	// VCSJJ selects the jj backend.
	VCSJJ VCS = "jj"
)

// Valid returns true when v is a member of the allow-list (git|jj).
func (v VCS) Valid() bool {
	return v == VCSGit || v == VCSJJ
}

// ChangeKind categorises one file's change in a worktree.
type ChangeKind string

const (
	Added    ChangeKind = "added"
	Modified ChangeKind = "modified"
	Deleted  ChangeKind = "deleted"
	Renamed  ChangeKind = "renamed"
	Copied   ChangeKind = "copied"
)

// FileChange describes one file's status in a worktree.
type FileChange struct {
	// Path is the file's current path, worktree-relative.
	Path string `json:"path"`
	// OldPath is set for Renamed and Copied entries.
	OldPath string `json:"oldPath,omitempty"`
	// Kind is the nature of the change.
	Kind ChangeKind `json:"kind"`
}

// WorktreeStatus summarises the state of a per-bead worktree.
type WorktreeStatus struct {
	// Exists is false when no worktree directory is found for this bead.
	// Callers should return a 404 WORKTREE_NOT_FOUND in that case.
	Exists bool
	// Clean is true when the worktree has no uncommitted changes.
	Clean bool
	// Ahead is the number of commits ahead of the upstream. Best-effort: the git
	// backend computes it via `git rev-list --count --left-right @{u}...HEAD`,
	// returning 0 when there is no upstream or on error; the jj backend returns 0.
	Ahead int
	// Behind is the number of commits behind the upstream (same best-effort rules).
	Behind int
}

// Sentinel errors returned by Backend methods.
var (
	// ErrNotImplemented is returned by write-side methods (Finalize, Push, Remove)
	// in M3. Callers must not depend on them.
	ErrNotImplemented = errors.New("wt: not implemented in M3")

	// ErrWorktreeNotFound is returned when the per-bead worktree directory does
	// not exist. Maps to HTTP 404 WORKTREE_NOT_FOUND.
	ErrWorktreeNotFound = errors.New("wt: worktree does not exist")

	// ErrVCSUnavailable is returned when the selected VCS binary is absent or
	// the source repo is not native to the selected VCS. Maps to HTTP 412
	// VCS_UNAVAILABLE.
	ErrVCSUnavailable = errors.New("wt: selected VCS backend unavailable")
)

// Backend is the VCS-agnostic per-bead worktree interface. Two concrete
// implementations exist: gitBackend (wraps internal/worktree) and jjBackend.
//
// Create, Status, DiffSummary, and Diff are implemented in M3.
// Finalize, Push, and Remove return ErrNotImplemented in M3 (filled in M4).
//
// Every method takes a context.Context so exec.CommandContext subprocesses are
// cancellation-aware.
type Backend interface {
	// Status returns the worktree's state. Returns ErrWorktreeNotFound when the
	// worktree directory for beadID does not exist.
	Status(ctx context.Context, beadID string) (WorktreeStatus, error)

	// Create ensures the per-bead worktree exists, creating it when absent and
	// reusing it (after safety checks) when present. Returns the absolute path.
	Create(ctx context.Context, worktreesDir, srcRepo, beadID string) (path string, err error)

	// DiffSummary returns the set of changed files in the worktree, including
	// untracked files (reported as Added). Non-mutating: never writes to the
	// index. Returns ErrWorktreeNotFound when the worktree does not exist.
	DiffSummary(ctx context.Context, beadID string) ([]FileChange, error)

	// Diff returns a streaming unified diff (git format) for the whole worktree
	// when path is empty, or for the single file identified by path when non-empty.
	// path MUST already be validated by safeRelPath before passing to Diff.
	// The ReadCloser wraps child stdout; Close reaps the child (no zombies).
	// Returns ErrWorktreeNotFound when the worktree does not exist.
	Diff(ctx context.Context, beadID, path string) (io.ReadCloser, error)

	// Finalize commits the worktree's changes. Returns ErrNotImplemented in M3.
	Finalize(ctx context.Context, beadID, msg string) error

	// Push pushes the worktree's branch upstream. Returns ErrNotImplemented in M3.
	Push(ctx context.Context, beadID string) error

	// Remove tears down the per-bead worktree. Returns ErrNotImplemented in M3.
	Remove(ctx context.Context, beadID string) error
}

// For returns the Backend for the given VCS. Returns an error if vcs is not
// in the allow-list.
func For(v VCS) (Backend, error) {
	switch v {
	case VCSGit:
		return &gitBackend{}, nil
	case VCSJJ:
		return &jjBackend{}, nil
	default:
		return nil, errors.New("wt: unknown VCS " + string(v))
	}
}

// Availability reports which VCS binaries are present on PATH.
type Availability struct {
	// Git is true when `git --version` succeeds.
	Git bool
	// JJ is true when `jj --version` succeeds.
	JJ bool
	// JJVer is the jj version string (best-effort; empty when JJ is false).
	JJVer string
}

// Detect probes for git and jj binaries and returns their availability.
// It is safe to call at startup with a short-deadline context.
func Detect(ctx context.Context) Availability {
	var a Availability

	if out, err := exec.CommandContext(ctx, "git", "--version").Output(); err == nil {
		_ = out // version string not used yet (US4/status DTO)
		a.Git = true
	}

	if out, err := exec.CommandContext(ctx, "jj", "--version").Output(); err == nil {
		a.JJ = true
		a.JJVer = strings.TrimSpace(string(out))
	}

	return a
}
