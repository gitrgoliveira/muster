# Contract: Claude Code Adapter (`claude`)

**Frozen against claude 2.1.145** (spike 2026-05-29, [../research.md](../research.md)). If the installed `claude` major version changes, re-run the spike before trusting this contract.

## Detection

```
$ claude --version          # -> "2.1.145 (Claude Code)"
$ claude auth status --json  # JSON is the DEFAULT; exit 0
{"loggedIn": true, "authMethod": "claude.ai", "apiProvider": "firstParty",
 "subscriptionType": "max", "email": "...", "orgId": "...", "orgName": "..."}
```
- `Installed` ← `claude` resolves on `$PATH` (or an explicit path via the adapter's `Options.Bin` seam; not exposed as a CLI flag in M2).
- `Version` ← parse `claude --version`.
- `LoggedIn` ← `claude auth status --json` → `.loggedIn`.

## Login (out-of-band; muster does NOT drive it)

`claude auth login` (interactive; flags `--claudeai` default, `--console`, `--sso`, `--email`). When `LoggedIn=false`, muster's `Adapter.Login` returns `ErrNotSupported` and the dispatch error message instructs the user to run `claude auth login` themselves. muster never reads, stores, or proxies credentials (FR-016).

## Modes → argv

| Mode (`core.Mode`) | argv fragment | Notes |
|---|---|---|
| `plan` | `--permission-mode plan` | verified: returns `is_error=false`, `terminal_reason=completed` |
| `agent` | `--permission-mode <permissionMode>` | autonomy is the **user-supplied** value (FR-021), one of `default`/`acceptEdits`/`dontAsk`/`bypassPermissions`/`auto` |

**There is NO `--plan` and NO `--no-streaming`** (handoff prose was stale).

## Invocation (tmux/CLI transport — interactive)

muster runs `claude` as a **plain interactive command** inside a muster-owned worktree + tmux session. It MUST NOT use claude's built-in `-w/--worktree` or `--tmux` (claude-specific; breaks adapter-agnostic transport).

Prompt delivery (handoff §7.1 pattern, hardened): assembled prompt written to `<worktree>/.muster-prompt-0.txt`; the pane runs a wrapper that feeds it to claude:
```
sh -c 'claude --permission-mode <pm> < .muster-prompt-0.txt'
```
(Exit code captured via `remain-on-exit`+`#{pane_dead_status}`, see [tmux-manager.md](tmux-manager.md). Prompt-as-file avoids multi-line shell-escaping.)

## NOT used in M2 (recorded for later milestones)

- `-p/--print` + `--output-format {json|stream-json}` + `--include-partial-messages`: clean structured events incl. `total_cost_usd`/`usage`/`modelUsage` → the **M4 quota** + **M5 opencode/SDK** path. Non-interactive (no attach/intervene), so not the M2 transport.
- `--json-schema`, `--max-budget-usd`, `--agents`, `--mcp-config`: future milestones (M6 skills, M8 policy).
