// Package tmux provides the canonical CLI-agent transport via tmux sessions.
// It implements the Manager interface which the orchestrator uses to spawn,
// stream, attach to, and terminate agent sessions.
// A fallback direct-exec transport is provided when tmux is absent.
package tmux
