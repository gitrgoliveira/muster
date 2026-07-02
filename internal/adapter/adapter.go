package adapter

import (
	"context"
	"errors"

	"github.com/gitrgoliveira/muster/internal/core"
)

// ErrNotSupported is returned by Login when the adapter does not support
// in-process login (e.g., claude M2 is detect-only; user runs `claude auth login`).
var ErrNotSupported = errors.New("not supported")

// QuotaSource describes where the adapter reports quota/cost information.
// M2 claude returns QuotaNone; M4 will add QuotaCLIOutput.
type QuotaSource int

const (
	QuotaNone       QuotaSource = iota // adapter does not surface quota
	QuotaCLIOutput                     // quota available in CLI stdout (e.g. --output-format json)
	QuotaAPIHeaders                    // quota available in HTTP response headers
)

// DetectResult holds adapter availability information gathered by Detect.
type DetectResult struct {
	Installed bool
	Version   string
	LoggedIn  bool // from `claude auth status --json`.loggedIn
}

// Mode describes a supported invocation profile and how to build its argv.
type Mode struct {
	// ID is the core.Mode identifier (e.g. ModePlan, ModeAgent).
	ID core.Mode
	// Args returns the argv fragment for this mode given the permission mode.
	// For plan mode: ["--permission-mode","plan"].
	// For agent mode: ["--permission-mode","<pm>"].
	Args func(pm core.PermissionMode) []string
}

// InvokeReq carries all the information needed to build a launch Spec.
type InvokeReq struct {
	Mode           core.Mode
	PermissionMode core.PermissionMode // user-supplied (FR-021); never defaulted by muster
	Worktree       string              // cwd for the agent process
	// PromptFile is the path to the assembled prompt file.
	// CONTRACT: PromptFile MUST live directly inside Worktree (i.e. its path
	// relative to Worktree is a single non-empty filename, no separators). The
	// claude adapter validates this via filepath.Rel and rejects anything with
	// a separator, "..", or "."; it then runs with cwd=Worktree so any
	// subdirectory or absolute path would break prompt delivery.
	PromptFile string
}

// Spec is the resolved, transport-agnostic launch description returned by Invoke.
// The orchestrator hands this to the tmux/fallback transport.
type Spec struct {
	Argv []string // e.g. ["sh", "-c", "claude --permission-mode acceptEdits < .muster-prompt-0.txt"]
	Env  []string // additional environment variables (merged with process env)
	Cwd  string   // working directory for the agent process
}

// LoginFlow carries instructions for out-of-band login.
// Only returned when Login is supported; otherwise Adapter returns ErrNotSupported.
type LoginFlow struct {
	Instructions string // human-readable instructions (e.g. "run: claude auth login")
}

// Adapter is the stable seam between the orchestrator and a specific CLI agent.
// M2 implements only claude; M5 adds gemini/codex/opencode.
type Adapter interface {
	// ID returns the unique agent identifier (e.g. core.AgentClaude).
	ID() core.AgentID

	// Detect probes the agent binary and auth state. Side-effect-free and fast.
	// "not installed" / "not logged in" are DetectResult fields, not errors.
	Detect(ctx context.Context) (DetectResult, error)

	// Modes returns the supported invocation profiles and their argv builders.
	Modes() []Mode

	// Invoke returns the transport-agnostic launch Spec. It does NOT spawn
	// anything; the orchestrator hands the Spec to the transport.
	Invoke(ctx context.Context, req InvokeReq) (Spec, error)

	// Login returns instructions for out-of-band login, or ErrNotSupported.
	// Adapters MUST NOT store credentials.
	Login(ctx context.Context) (LoginFlow, error)

	// QuotaSource returns advisory metadata about quota reporting.
	// M2 claude returns QuotaNone.
	QuotaSource() QuotaSource
}
