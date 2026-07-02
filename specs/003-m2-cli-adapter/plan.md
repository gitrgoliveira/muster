# Implementation Plan: M2 — First CLI Adapter (Claude Code)

**Branch**: `003-m2-cli-adapter` | **Date**: 2026-05-29 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/003-m2-cli-adapter/spec.md`

## Summary

M2 makes a dispatched bead *execute*. `POST /api/v1/beads/{id}/dispatch` (a no-op-ish stub in M1) gains a real body: muster resolves the bead's source repo (by ID-prefix→repo mapping), creates a per-bead **git worktree**, assembles the bead Title+Description into a prompt file, and launches **Claude Code** inside a **tmux session** as the canonical, adapter-agnostic transport. The pane's raw terminal output is streamed over the existing WebSocket hub as `runlog.line` frames (rendered by a terminal emulator in the UI; no server-side stripping) and replayable via `capture-pane`; the user can `attach` to the live pane and `send` keystrokes. On exit, the step is marked done/failed and the bead transitions. muster survives restart by re-discovering `muster/*` tmux sessions. The autonomy (`--permission-mode`) is supplied by the user per dispatch — muster never defaults it. M1's store layer and REST/WS surface are unchanged; M2 is purely additive.

## Technical Context

**Language/Version**: Go 1.26 (unchanged from M0/M1)

**Primary Dependencies** (M1 deps retained; M2 adds **one** new Go module — `golang.org/x/sys`, promoted to a direct dependency for `unix.Mkfifo` (runlog FIFO creation). tmux/git/claude are external runtime binaries shelled out to):

| Dependency | Kind | Purpose |
|---|---|---|
| `tmux` ≥ 3.2 (host: 3.6b) | external binary | canonical CLI-agent transport (spawn/pipe/attach/send/capture/kill/list) |
| `git` ≥ 2.40 (host: 2.54.0) | external binary | per-bead worktrees (`git worktree add`) |
| `claude` (host: 2.1.145) | external binary | the one M2 adapter |
| `golang.org/x/sys` | Go module (direct) | `unix.Mkfifo` for the runlog FIFO (internal/tmux/fifo_unix.go) |
| `os/exec`, `bufio`, `context` | stdlib | process mgmt, stream scanning |

> The only new `go.mod` entry is `golang.org/x/sys` (promoted from indirect to direct, for FIFO creation) — it compiles into the single binary, so the single-binary constitution gate stays intact. tmux/git/claude remain external binaries shelled out to (tmux and `claude` are also probed at startup; git is used at run time but not probed), exactly like M1's `bd` bridge.

**Storage**: none new. Runlog is **transient** (streamed; `capture-pane` for catch-up). Worktrees live on disk under `--worktrees-dir`, managed by git, not a muster store. Beads DB remains the source of truth for issue state (unchanged from M1).

**Testing**: `go test -race ./...`. New packages use fakes: a fake `tmux` binary on `$PATH` (script that records argv, like M1's fake `bd`), a fake `claude` (emits canned output, controllable exit code), and real `git worktree` against a tmpdir repo. Adapter/transport unit tests + an end-to-end dispatch→stream→exit integration test gated on real `tmux` presence (skip if absent).

**Target Platform**: macOS/Linux, local loopback (unchanged).

**Project Type**: CLI / web-service hybrid — single Go binary (unchanged).

**Performance Goals** (from spec SCs):
- Dispatch → agent running in a worktree ≤ **5 s** (SC-001)
- Pane output → `runlog.line` at a WS client ≤ **2 s** (SC-002)
- Restart → rediscovered session streaming resumes ≤ **5 s** (SC-005)

**Constraints**:
- Single binary; no daemons spawned by muster (tmux sessions run under the **user's** default tmux server, not as muster children — this is what gives restart survival).
- Additive only: zero behavioral change to M1 REST endpoints, the store layer, or `bead.*` WS events (FR-019).
- muster never imposes agent autonomy (FR-021) and never handles credentials (FR-016).

**Scale/Scope**: M2 = one adapter, one step per bead (idx 0), one active run per bead (409 on duplicate), a handful of concurrent runs, <10 WS clients (unchanged).

## Constitution Check

Checked against the ratified `.specify/memory/constitution.md` (v1.0.0, 2026-05-29 — drafted during this milestone from the de-facto M0/M1 principles). M2 passes every principle:

| Principle | Status | Notes |
|---|---|---|
| I. Single binary, self-contained | PASS | `cmd/muster/` still one binary; tmux/git/claude are external runtime deps, shelled out not linked (tmux/claude probed at startup; git used at run time); the one new `go.mod` entry (`golang.org/x/sys`, for `unix.Mkfifo`) compiles into that single binary, so the gate holds |
| II. Beads is the source of truth | PASS | Runlog transient; worktrees git-managed; issue state still via the `bd` write path. muster owns no new durable state |
| III. Layered architecture, thin handlers | PASS | Orchestration is its own vertical (`adapter`/`tmux`/`worktree`/`orchestrator`); handlers parse→delegate→render |
| IV. Test-first, per-layer coverage | PASS | TDD ordering + per-package coverage gates below; `-race` clean; fake tmux/claude + real-tmux integration test |
| V. Additive, backward-compatible surface | PASS | New endpoints + WS event types are additive; M1 surface untouched (FR-019) |
| Security & orchestration constraints | PASS | No credential handling (FR-016); user-controlled autonomy, never defaulted (FR-021); adapter-agnostic transport; per-bead worktree isolation |
| Spec-driven, verify-by-spike | PASS | spec→plan→tasks followed; the 2026-05-29 spike pinned the claude/tmux contracts before this plan |

> The constitution was the unfilled template at the start of this milestone; it is now ratified (v1.0.0). Future milestones gate against it directly.

**Coverage targets (CI)** — extend M1's gates:
- `internal/adapter/` ≥ 80%
- `internal/tmux/` ≥ 75% (transport; integration-heavy, fakes cover the rest)
- `internal/worktree/` ≥ 85%
- `internal/orchestrator/` ≥ 80%
- `internal/api/beads/` (dispatch/attach/send handlers) ≥ 75%
- existing M0/M1 package gates unchanged
- `go test -race ./...` clean

## Project Structure

### Documentation (this feature)

```text
specs/003-m2-cli-adapter/
├── plan.md              # This file
├── spec.md              # Feature spec (clarified + spike-pinned)
├── research.md          # Spike findings (Phase 0) + Phase-0 design decisions
├── data-model.md        # Phase 1 — entities, Go interfaces, DispatchInput/WS/status deltas
├── quickstart.md        # Phase 1 — end-to-end dispatch walkthrough
├── contracts/
│   ├── adapter-interface.md   # the Adapter Go interface + RunEvent
│   ├── tmux-manager.md        # internal/tmux.Manager primitives + naming + socket
│   ├── claude-adapter.md      # pinned claude CLI surface (modes, auth, invocation)
│   ├── http-endpoints.md      # dispatch (real body), attach, send, status additions
│   └── ws-events.md           # runlog.line, tmux.session.opened/closed frames
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 (speckit-tasks) — present in this feature
```

### Source Code Layout (extends M1)

The M1 tree is preserved. M2 adds four packages and extends three existing files.

```text
cmd/muster/
└── main.go              # CHANGED: parse --repo (repeatable prefix=path), --worktrees-dir,
                         #          --run-timeout, --default-permission-mode; probe tmux/claude (git used at run time);
                         #          build Orchestrator; restart-recovery scan; wire into services

internal/
├── core/                # CHANGED (small): add PermissionMode enum + Run/step state if needed;
│                        #   AgentID, Mode, Column, StepStatus already exist from M0
│
├── adapter/             # NEW — agent integrations
│   ├── adapter.go       # Adapter interface, RunEvent, DetectResult, Mode, LoginFlow
│   ├── registry.go      # id → Adapter; M2 registers only claude
│   ├── claude/
│   │   ├── claude.go    # Detect (claude auth status --json), Modes, Invoke argv, Login=ErrNotSupported
│   │   └── claude_test.go
│   └── adapter_test.go
│
├── tmux/                # NEW — canonical CLI transport
│   ├── manager.go       # Detect, Spawn, Attach, Pipe, Send, Capture, Kill, List (default socket)
│   ├── fallback.go      # direct exec.Command transport when tmux absent
│   ├── name.go          # muster/<bead>/<step>/<loop> encode/parse
│   └── *_test.go        # fake tmux binary on PATH
│
├── worktree/            # NEW — minimal per-bead git worktree (precursor to M3 wt.Backend)
│   ├── worktree.go      # Create/Reuse(beadID, repoPath) -> path, branch muster/<bead>
│   └── worktree_test.go # real git against tmpdir
│
├── orchestrator/        # NEW — the run lifecycle glue
│   ├── orchestrator.go  # Dispatch(bead, agent, mode, permMode): resolve repo→worktree→prompt→
│   │                    #   transport.Spawn→stream pipe to runlog/WS→watch exit→transition
│   ├── runlog.go        # in-flight run registry; pane stream fan-out to WS; capture-pane catch-up
│   ├── recovery.go      # startup scan of muster/* sessions → re-attach streaming
│   ├── repomap.go       # prefix→repo resolution + permission-mode allow-list/resolve
│   └── *_test.go
│
├── api/                 # CHANGED: router + handlers (additive)
│   ├── beads/handlers.go# Dispatch (real), + steps/{idx}/attach, steps/{idx}/send
│   └── health/          # status DTO + handler: tmuxAvailable/Version, adapters[], runningCount
│
├── services/            # CHANGED: BeadService gains an Orchestrator dep; Dispatch delegates to it
│
├── ws/                  # CHANGED (additive): EventRunlogLine, EventTmuxOpened/Closed + Frame fields
│
├── store/               # UNCHANGED
└── config/              # CHANGED (small): parse the new flags/env into the config struct
```

**Structure Decision**: extend the M1 tree; do not refactor. Orchestration is a new vertical (`adapter` + `tmux` + `worktree` + `orchestrator`) that `services.BeadService.Dispatch` delegates to, keeping handlers thin and the store layer untouched. The transport (`tmux`) is deliberately separate from the adapter (`claude`) so M5 can add gemini/codex/opencode behind the same `Adapter` interface and the same transport.

## Phase 0 — Research (research.md)

`research.md` already holds the **2026-05-29 spike** (empirical CLI/tmux findings). Phase 0 adds the two remaining design decisions (Decision/Rationale/Alternatives), resolving all NEEDS CLARIFICATION:

1. **Runlog transport format** — *Decision*: stream **raw terminal bytes** from `pipe-pane` as `runlog.line` frames; the UI renders them in a terminal emulator (xterm.js). Catch-up via `capture-pane -ep -S -`. *Rationale*: a live agent is a full-screen TUI; stripping ANSI from a redrawing TUI yields garbage, and raw-stream-to-emulator is the conventional pattern (ttyd/gotty/VS Code) that also preserves the pretty TUI + attach + interactivity. **This supersedes the spec's earlier FR-009 "strip ANSI" wording** (FR-009 updated). *Alternatives*: (a) server-side ANSI strip → loses TUI fidelity, garbage on redraws; (b) run `claude -p --output-format stream-json` in the pane → clean structured lines + free quota, but kills interactivity/intervention and the pretty TUI — recorded as the **M4/M5** path, not M2.

2. **Exit-code capture** — *Decision*: tmux mode uses `set remain-on-exit on` + read `#{pane_dead_status}` on pane death, then emit closure and `kill-session`; fallback mode reads `cmd.Wait()` directly. *Rationale*: tmux-native, no temp-file race, works for the vanishing-pane problem the spike found; fallback has the child handle so Wait() is exact. *Alternatives*: `$?`-to-file wrapper (race-prone, needs cleanup), session-gone inference (loses the actual code).

**Output**: research.md updated; zero NEEDS CLARIFICATION remain.

## Phase 1 — Design

### data-model.md (highlights)
- **Adapter / RunEvent / DetectResult / Mode / LoginFlow** Go interface shapes.
- **Run** (in-memory): beadID, stepIdx=0, loop=0, agent, mode, permissionMode, worktree path, session name, state (running/done/failed/cancelled), exit code, started/ended.
- **DispatchInput** extension: add `PermissionMode core.PermissionMode` (new enum, allow-listed) — `Agent`, `Mode` already exist.
- **WS Frame** additions: `runlog.line` (beadID, stepIdx, seq, data), `tmux.session.opened`/`closed` (beadID, stepIdx, name).
- **OrchestratorStatusResponse** additions: `tmuxAvailable bool`, `tmuxVersion string`, `adapters []{id,version,loggedIn}`, `runningCount int`.
- **RepoMapping**: `map[prefix]repoPath`.

### contracts/
- **adapter-interface.md**, **tmux-manager.md**, **claude-adapter.md** (the pinned surface as a frozen contract), **http-endpoints.md** (dispatch real body + 202/409/422 codes, attach, send), **ws-events.md**.

### Agent context update
Update the `<!-- SPECKIT START -->`/`<!-- SPECKIT END -->` block in `CLAUDE.md` to point at this plan.

## Phase 2 — Tasks (`speckit-tasks`)

Ordering (the authoritative task list is [`tasks.md`](tasks.md)): (1) deps/skeletons + new flags parsing; (2) `core` enum `PermissionMode` (TDD); (3) `tmux` manager + fallback (TDD, fake tmux); (4) `worktree` helper (TDD, real git tmpdir); (5) `adapter` interface + `claude` adapter (TDD, fake claude); (6) `orchestrator` dispatch lifecycle + runlog fan-out (TDD); (7) WS frame + status DTO additions; (8) handlers (dispatch real, attach, send) + router; (9) restart-recovery scan; (10) end-to-end integration test (real tmux, fake/real claude); (11) docs/quickstart.

## Complexity Tracking

| Decision | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| Separate `tmux` transport from `claude` adapter | M5 adds 3 more CLI adapters over the same transport (handoff §7) | Folding tmux into the claude adapter would force a rewrite at M5 and couple naming/recovery to one vendor |
| Pull a minimal `worktree` package forward from M3 | The agent must run in isolation, not the repo root (US2, a correctness precondition) | Running in repo root would corrupt the user's checkout and collide across beads |
| Prefix→repo mapping (sliver of M7) | User chose per-bead repo resolution; beads-central holds multi-repo beads | Single global `--repo` rejected by user; full `/repos` CRUD is M7-sized |
| Raw-terminal runlog + UI terminal emulator | TUI agents redraw; clean line-logs aren't extractable from an interactive pane | Server-side ANSI strip yields garbage; `-p` stream-json kills interactivity (deferred to M4/M5) |

---

**Plan status**: Phase 0 + Phase 1 design artifacts written alongside this file; ready for `/speckit-tasks` once reviewed.
