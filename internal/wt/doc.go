// Package wt provides a VCS-agnostic per-bead worktree abstraction.
//
// It wraps [internal/worktree] for the git backend and exposes a [Backend]
// interface that both the git and jj implementations satisfy. The orchestrator
// calls [For] to resolve a backend by VCS name, then uses [Backend.Create] to
// ensure each bead's per-bead worktree exists before launching an agent run.
//
// The diff-exposure endpoints ([Backend.DiffSummary] and [Backend.Diff]) serve
// the agent's uncommitted work-in-progress to the REST API without mutating
// the worktree or git index.
package wt
