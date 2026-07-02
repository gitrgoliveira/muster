package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gitrgoliveira/muster/internal/adapter"
	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/shellquote"
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
		// Distinguish context cancellation/timeout from "binary failed to run":
		// the caller (Dispatch, health probe) needs to see the cancellation, not
		// a misleading "not installed" verdict.
		if ctx.Err() != nil {
			return adapter.DetectResult{}, ctx.Err()
		}
		// The binary resolved on PATH (resolve() succeeded) but `claude
		// --version` failed to run. It IS installed — report Installed:true and
		// surface a real error (with stderr) rather than a bare Installed:false,
		// which would make Dispatch return ADAPTER_NOT_INSTALLED (501) and tell
		// the operator to install claude when the real problem is a broken or
		// misbehaving binary.
		var stderr string
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		if stderr != "" {
			return adapter.DetectResult{Installed: true}, fmt.Errorf("claude --version failed: %w (stderr: %s)", err, stderr)
		}
		return adapter.DetectResult{Installed: true}, fmt.Errorf("claude --version failed: %w", err)
	}
	version := strings.TrimSpace(string(versionOut))

	// Get auth status.
	authCmd := exec.CommandContext(ctx, bin, "auth", "status", "--json")
	authOut, err := authCmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return adapter.DetectResult{}, ctx.Err()
		}
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

	// Defense in depth: the orchestrator already resolves+validates the
	// permission mode, but Invoke is a public method and must not build a
	// `--permission-mode <value>` argv from an out-of-allow-list value (which
	// would only fail later, in the CLI). Reject it here. (Plan mode hardcodes
	// "plan" and ignores this value, but a valid PermissionMode is still the
	// contract for the request.)
	if !req.PermissionMode.Valid() {
		return adapter.Spec{}, fmt.Errorf("claude adapter: invalid permission mode %q", req.PermissionMode)
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

	// Prompt file path relative to cwd (the worktree). The orchestrator's
	// contract (see Dispatch in internal/orchestrator) is to write the prompt
	// directly at <worktree>/.muster-prompt-0.txt. Validate that contract here
	// so a future caller passing a subdir-or-absolute PromptFile fails fast
	// with a clear adapter error instead of a confusing shell-redirect error.
	promptRel, err := filepath.Rel(req.Worktree, req.PromptFile)
	if err != nil || promptRel == "." || strings.HasPrefix(promptRel, "..") || strings.ContainsRune(promptRel, filepath.Separator) {
		return adapter.Spec{}, fmt.Errorf("claude adapter: PromptFile %q must be directly under Worktree %q", req.PromptFile, req.Worktree)
	}

	// Build the shell one-liner that feeds the prompt file to claude via stdin.
	// Using sh -c avoids multi-line shell-escaping issues.
	// Defense in depth: every fragment is single-quoted before joining. bin and
	// promptRel can legitimately contain spaces or special characters (e.g.
	// /Users/Some User/bin/claude). modeArgs comes from a validated enum today
	// (Orchestrator.resolvePermMode rejects any value outside PermissionMode.Valid()),
	// but quoting it here means a future caller that introduces a free-form
	// fragment cannot accidentally turn this into a shell-injection sink.
	//
	// `exec` prefix: replace the `sh -c` shell with the claude process rather
	// than leaving sh as a parent that forks-and-waits. Under the fallback
	// (direct-exec) transport, Kill signals only the process muster started —
	// i.e. sh — so without `exec` a timeout/cancel could kill sh and orphan the
	// claude child (the run would look terminated while claude kept running).
	// With `exec`, that tracked process IS claude, so Kill terminates it
	// directly. Harmless-to-beneficial under tmux too: the pane's foreground
	// process becomes claude, so remain-on-exit / pane_dead_status track
	// claude's own exit.
	quotedMode := make([]string, len(modeArgs))
	for i, arg := range modeArgs {
		quotedMode[i] = shellquote.Single(arg)
	}
	claudeCmd := "exec " + shellquote.Single(bin) + " " + strings.Join(quotedMode, " ") + " < " + shellquote.Single(promptRel)
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
