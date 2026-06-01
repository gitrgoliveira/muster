package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gitrgoliveira/muster/internal/adapter"
	"github.com/gitrgoliveira/muster/internal/core"
)

// Options configures the claude Adapter.
type Options struct {
	// Bin is the explicit path to the claude binary.
	// If empty, the adapter searches PATH.
	Bin string
}

// Adapter implements adapter.Adapter for the Claude Code CLI.
type Adapter struct {
	bin string // resolved binary path (empty = search PATH)
}

// New creates a new claude Adapter. Register it with adapter.Registry after creation.
func New(opts Options) *Adapter {
	return &Adapter{bin: opts.Bin}
}

// ID implements adapter.Adapter.
func (a *Adapter) ID() core.AgentID { return core.AgentClaude }

// resolve returns the resolved binary path, searching PATH if not explicit.
func (a *Adapter) resolve() (string, error) {
	if a.bin != "" {
		return a.bin, nil
	}
	return exec.LookPath("claude")
}

// authStatus is the JSON shape returned by `claude auth status --json`.
type authStatus struct {
	LoggedIn bool `json:"loggedIn"`
}

// Detect implements adapter.Adapter. It shells out to `claude --version` and
// `claude auth status --json` to determine installation and auth state.
// "not installed" and "not logged in" are reported as DetectResult fields, not errors.
func (a *Adapter) Detect(ctx context.Context) (adapter.DetectResult, error) {
	bin, err := a.resolve()
	if err != nil {
		// Binary not found — not installed, not an error.
		return adapter.DetectResult{Installed: false}, nil
	}

	// Get version.
	versionCmd := exec.CommandContext(ctx, bin, "--version")
	versionOut, err := versionCmd.Output()
	if err != nil {
		return adapter.DetectResult{Installed: false}, nil
	}
	version := strings.TrimSpace(string(versionOut))

	// Get auth status.
	authCmd := exec.CommandContext(ctx, bin, "auth", "status", "--json")
	authOut, err := authCmd.Output()
	if err != nil {
		// Binary exists but auth check failed — installed but status unknown; treat as not logged in.
		return adapter.DetectResult{
			Installed: true,
			Version:   version,
			LoggedIn:  false,
		}, nil
	}

	var status authStatus
	if err := json.Unmarshal(authOut, &status); err != nil {
		// JSON parse failed; treat as not logged in.
		return adapter.DetectResult{
			Installed: true,
			Version:   version,
			LoggedIn:  false,
		}, nil
	}

	return adapter.DetectResult{
		Installed: true,
		Version:   version,
		LoggedIn:  status.LoggedIn,
	}, nil
}

// Modes implements adapter.Adapter.
// Contract (spike-verified, claude 2.1.145):
//   - plan mode  → --permission-mode plan
//   - agent mode → --permission-mode <pm>  (pm is user-supplied, FR-021)
func (a *Adapter) Modes() []adapter.Mode {
	return []adapter.Mode{
		{
			ID: core.ModePlan,
			Args: func(_ core.PermissionMode) []string {
				return []string{"--permission-mode", "plan"}
			},
		},
		{
			ID: core.ModeAgent,
			Args: func(pm core.PermissionMode) []string {
				return []string{"--permission-mode", string(pm)}
			},
		},
	}
}

// Invoke implements adapter.Adapter.
// Returns a transport-agnostic Spec describing how to run claude.
// Prompt delivery: the orchestrator writes the assembled prompt to
// <worktree>/.muster-prompt-0.txt; the Argv wraps it in a shell one-liner.
func (a *Adapter) Invoke(_ context.Context, req adapter.InvokeReq) (adapter.Spec, error) {
	bin, err := a.resolve()
	if err != nil {
		return adapter.Spec{}, fmt.Errorf("claude not on PATH: %w", err)
	}

	// Build the mode argument fragment.
	var modeArgs []string
	for _, m := range a.Modes() {
		if m.ID == req.Mode {
			modeArgs = m.Args(req.PermissionMode)
			break
		}
	}
	if modeArgs == nil {
		return adapter.Spec{}, fmt.Errorf("claude adapter: unsupported mode %q", req.Mode)
	}

	// Prompt file path relative to cwd (the worktree).
	promptRel := filepath.Base(req.PromptFile)

	// Build the shell one-liner that feeds the prompt file to claude via stdin.
	// Using sh -c avoids multi-line shell-escaping issues.
	// bin and promptRel are single-quoted to handle paths with spaces or
	// special characters (e.g. /Users/Some User/bin/claude).
	claudeCmd := shellQuote(bin) + " " + strings.Join(modeArgs, " ") + " < " + shellQuote(promptRel)
	argv := []string{"sh", "-c", claudeCmd}

	return adapter.Spec{
		Argv: argv,
		Cwd:  req.Worktree,
	}, nil
}

// Login implements adapter.Adapter. Claude login is out-of-band; muster
// does not drive it. Returns ErrNotSupported per the contract.
func (a *Adapter) Login(_ context.Context) (adapter.LoginFlow, error) {
	return adapter.LoginFlow{}, adapter.ErrNotSupported
}

// QuotaSource implements adapter.Adapter. M2 does not track quota.
func (a *Adapter) QuotaSource() adapter.QuotaSource { return adapter.QuotaNone }

// shellQuote wraps s in single quotes, escaping any embedded single quotes
// using the POSIX idiom: ' becomes '\” (end-quote, literal-single-quote, re-open-quote).
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
