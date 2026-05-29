# M2 Spike — Empirical CLI/tmux Findings

**Date**: 2026-05-29 | **Host**: macOS (darwin 24.6.0) | **Purpose**: pin the `claude` CLI surface and tmux transport before `/speckit-plan` finalizes contracts. Mirrors M1's Phase 7.5 discipline — verify falsifiable assumptions, don't trust the handoff prose.

## Environment (all present, versions pinned)

| Tool | Version | Floor | Verdict |
|---|---|---|---|
| `tmux` | 3.6b | ≥ 3.2 (handoff §7.1) | ✅ well above floor |
| `git` | 2.54.0 | ≥ 2.40 (handoff §18) | ✅ |
| `claude` | 2.1.145 | — | ✅ |
| `gemini` | present | — | (M5) |
| `codex` | absent | — | (M5) |

## CRITICAL — the handoff's mode flags are WRONG

**Handoff §6.2 / §20 say**: `claude --plan --no-streaming` for plan mode.
**Reality (claude 2.1.145)**: **neither flag exists.** Verified against `claude --help`.

Correct mapping:
- **Plan mode** → `--permission-mode plan` (the `plan` value is in the documented `--permission-mode` choice set: `acceptEdits | auto | bypassPermissions | default | dontAsk | plan`). **Verified working**: `claude -p "…" --permission-mode plan --output-format json` returned `is_error=false, terminal_reason=completed`.
- **"Agent mode"** → there is **no `--plan`/agent toggle**. The default interactive session *is* the agent. Autonomy is tuned via `--permission-mode` (`default` / `acceptEdits` / `dontAsk` / `bypassPermissions`). `--agent <name>` selects a *named* sub-agent — a different concept, not "agent mode".
- There is **no `--no-streaming`**. Output mode is controlled by `-p/--print` + `--output-format {text|json|stream-json}`.

**Action**: the adapter `Modes()` table for `claude` must map `plan → --permission-mode plan` and `agent → --permission-mode <autonomy>`, where **autonomy is supplied by the user per dispatch** (FR-021), not a muster default. Drop all references to `--plan`/`--no-streaming`.

## Auth detection — clean, non-interactive, structured

`claude auth status --json` (JSON is the **default**; `--text` for human) returns, exit 0:
```json
{"loggedIn": true, "authMethod": "claude.ai", "apiProvider": "firstParty",
 "subscriptionType": "max", "email": "…", "orgId": "…", "orgName": "…"}
```
- **Detect mechanism (FR-016)**: shell `claude auth status --json`, parse `loggedIn`. No TTY needed.
- **Out-of-band login (confirmed detect-only decision)**: `claude auth login` (interactive; flags `--claudeai` default, `--console`, `--sso`, `--email`). muster instructs the user to run this; it does not drive it.

## Invocation + exit-code contract

`claude -p "<prompt>" --output-format json` → exit 0, ~6.4 s wall (2.7 s TTFT) for a trivial prompt, and a rich result envelope:
```
{"type":"result","subtype":"success","is_error":false,"result":"SPIKE_OK",
 "terminal_reason":"completed","num_turns":1,"session_id":"…","total_cost_usd":0.192,
 "usage":{"input_tokens":6,"output_tokens":12,"cache_creation_input_tokens":30663,…},
 "modelUsage":{"claude-opus-4-7":{…,"costUSD":0.192}},"permission_denials":[]}
```
- **Quota/cost is available for free** in `--output-format json` (`total_cost_usd`, `usage`, `modelUsage`). This is relevant to **M4** quota tracking — but only on the `-p` path (see tension below). Consistent with M2 deferring quota.
- Trivial-prompt cost was **$0.19** (dominated by 30 k cache-creation tokens for the system prompt). Real runs cost more — relevant for run-cost expectations.

## tmux transport primitives — all validated end-to-end

Spawn → `pipe-pane` → `send-keys` → `capture-pane` → `list-sessions` (prefix filter) → `kill-session` all work. Session names **with slashes** (`muster/spike-bead/0/0`) are accepted.

Two nuances that change the runlog contract:
1. **`pipe-pane` emits RAW pane bytes** — including terminal control sequences (observed `[?2004l` bracketed-paste markers and echoed input). **`capture-pane -p` emits clean rendered text.** → The `runlog.line` stream (FR-009) must **strip/handle ANSI/control sequences** before broadcasting, or the UI gets terminal garbage. Plan must pick: filter pipe-pane output, or poll capture-pane.
2. **Default socket required for user attach.** The spike used `-L muster_spike` to avoid polluting the user's tmux, but for `tmux attach -t muster/<bead>/0/0` (FR-011) to work from the user's normal shell, muster MUST spawn on the **default** tmux socket (or the attach command it returns must include the `-L` socket).

## Exit-code capture gap (FR-013)

Running `claude` directly as the pane command means the **pane/session disappears on exit** — `list-sessions` no longer shows it, but the **exit code is not surfaced** by session-gone alone. Two viable mechanisms (pin in plan):
- **(a) wrapper records `$?`**: pane runs `sh -c 'cat prompt | claude … ; echo $? > <worktree>/.muster-exit'`; read the file when the session ends.
- **(b) `remain-on-exit on` + `#{pane_dead_status}`**: keep the dead pane, read its exit status via `display-message`/`list-panes` format, then kill. Cleaner; tmux-native.

The handoff §7.1 wrapper (`sh -c 'cat prompt | <agent>'`) does **not** capture `$?` — this is a real gap to close.

## claude has BUILT-IN worktree + tmux — do NOT use them

`claude` exposes `-w/--worktree [name]` (creates a git worktree) and `--tmux` ("Create a tmux session for the worktree … iTerm2 native panes when available; `--tmux=classic` for traditional tmux").

**Decision: muster owns worktree + tmux itself; do not delegate to claude's built-ins.** Rationale: the handoff (§7) makes `internal/tmux` the **adapter-agnostic** transport for *every* CLI agent (gemini, codex, cli-generic). Relying on claude's `-w/--tmux` would (a) only work for claude, (b) couple session naming/iTerm2 behavior to one vendor, (c) break uniform attach/send/recovery. muster runs `claude` as a plain command inside a muster-created worktree + muster-managed tmux session.

## The central architectural tension (decide in plan)

| | tmux pane capture (handoff §7 choice) | `-p`/`--output-format stream-json` |
|---|---|---|
| Live `tmux attach` + TTY-aware TUI | ✅ the whole value prop | ❌ it's a pipe, not an attachable TUI |
| Structured result / cost / usage | ❌ must parse rendered TUI | ✅ clean JSON envelope |
| Exit semantics | needs wrapper/`pane_dead_status` | ✅ exit code direct |
| Adapter-agnostic | ✅ same for all CLIs | ❌ claude-specific shape |

**M2 stays with tmux pane capture** (live attach is the differentiator; quota deferred). Record `--output-format stream-json --include-partial-messages --input-format stream-json` as the **SDK-ish path** to revisit for M5 (opencode) and M4 (quota), not M2.

## Net effect on the spec

- FR-006 mode mapping rewritten to the real `--permission-mode` surface.
- FR-016 auth detect pinned to `claude auth status --json` / login to `claude auth login`.
- Added runlog ANSI-handling and default-socket notes; added the exit-code-capture mechanism as a plan input.
- Confirmed tmux ≥ 3.2 floor satisfied (3.6b) and all transport primitives feasible.
- Reaffirmed muster-owns-transport (don't use `claude -w/--tmux`).
