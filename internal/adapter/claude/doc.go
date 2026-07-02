// Package claude implements the Adapter interface for the Claude Code CLI.
// It shells out to the `claude` binary for detection and invocation; the
// transport (tmux/fallback) is handled by the orchestrator, not this package.
package claude
