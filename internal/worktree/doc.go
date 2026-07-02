// Package worktree manages per-bead git worktrees.
//
// Each bead gets its own linked worktree checked out on branch
// muster/<beadID>, so the agent's file edits and commits land in a separate
// working directory on a dedicated branch rather than in the main checkout's
// working tree. This is working-directory isolation, not a sandbox: linked
// worktrees still share the repository's common object store and refs, and the
// package does not constrain what the agent process does — it only guarantees
// where the checkout lives.
package worktree
