// Package worktree manages per-bead git worktrees.
// Each bead gets an isolated worktree on branch muster/<beadID> so the agent
// never touches the main checkout.
package worktree
