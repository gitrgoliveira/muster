// Package orchestrator implements the run lifecycle: resolve repo → worktree →
// prompt → spawn transport → stream runlog → watch exit → transition bead.
// It is the glue between the adapter, tmux transport, worktree, and services layers.
package orchestrator
