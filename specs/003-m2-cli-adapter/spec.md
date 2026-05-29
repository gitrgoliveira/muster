# Feature Specification: M2 — First CLI Adapter (Claude Code)

**Feature Branch**: `003-m2-cli-adapter`

**Created**: 2026-05-29

**Status**: Draft

**Input**: Deliver the first CLI agent adapter (Claude Code) end-to-end: detect, login, plan/agent mode, run the agent inside a per-bead isolated worktree, stream its output to a runlog and over WebSocket, and let the user attach to the live session. Per the canonical roadmap (`prototype/handoff/spec.md` §20, milestone M2 "One CLI adapter") with two scope decisions taken during M2 prep: (1) a thin vertical slice — single step, single adapter, excluding the full dispatcher/scheduler (M4), multi-provider (M5), and sub-bead/policy (M8); (2) pull a **minimal** per-bead worktree layer forward from M3.

**Canonical Sources**:
- `prototype/handoff/spec.md` §6 (step assembly), §6.2 (agent input contract), §7 (CLI adapters), §7.1 (tmux integration), §20 (milestones)
- M1 spec/contracts (`specs/002-m1-beads-backed/`) — the unchanged REST/WS surface and beads store this milestone builds on

---

## Product Context

Muster is **beads-central**: a server that manages beads, where the beads database is the source of truth. M0 served seed data; M1 made reads/writes real against a live beads database (via `bd`). **M2 makes muster *do work*** — it turns a bead from a tracked task into an executing one. When a user dispatches a bead to the `claude` agent, muster launches Claude Code against an isolated copy of the bead's source repository, feeds it the bead as the task, streams everything the agent does back to the UI in real time, and lets the user attach to the live terminal to watch or intervene.

This is the largest conceptual leap in the project so far: M0/M1 were a read/CRUD data server; M2 introduces process orchestration, terminal multiplexing, version-controlled isolation, and live output streaming. To keep the leap bounded, M2 is a **single vertical slice** — one adapter (`claude`), one step per dispatch, one isolated worktree — deliberately excluding the scheduler, capacity gating, quota tracking, additional providers, and sub-bead policy that later milestones add.

---

## Clarifications

### Session 2026-05-29 (informed-guess defaults — confirm via `/speckit-clarify` if any are wrong)

- Q: Which source repository does a bead's worktree branch from, given beads-central can hold beads from many repos (e.g. `mp-*`)? → A (assumed): A **single source repository** configured at startup via `--repo`/`MUSTER_REPO` (path to a git repo). M2 is single-instance/single-repo, mirroring M1's single-`.beads` model. Per-bead repo routing is **M7 (Repos & routing)**. If `--repo` is unset, dispatch returns a clear error.
- Q: Does M2 introduce the full plan→build→review step chain, or a single step? → A (assumed): A **single implicit step at index 0** per dispatch. The `/steps/{idx}` endpoints accept only `idx=0` in M2; multi-step chains are deferred (M4/M8). This keeps the step-pointer/loop machinery out of M2.
- Q: How is the interactive Claude Code login surfaced through a headless daemon? → A (assumed): muster **detects auth state** at adapter `Detect()` and, when unauthenticated, exposes a **`LoginFlow` that runs `claude` interactive login inside an attachable tmux session** the user attaches to (same attach mechanism as a running step). muster never captures or stores credentials.
- Q: How does muster invoke Claude Code plan vs agent mode? → A (assumed): via documented Claude Code CLI flags resolved from the adapter's `Modes()` table; the **exact flags MUST be pinned by an empirical spike** before implementation (as M1 did in its Phase 7.5 spike), since CLI surface drifts.
- Q: What is the bead's task input to the agent? → A: the bead **Title + Description** (the M1 issue record), written to a temp prompt file in the worktree; the agent reads it via a thin wrapper. No constitution/skill assembly in M2 (that is M6).

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Dispatch a bead to the Claude Code agent (Priority: P1)

A user calls `POST /api/v1/beads/{id}/dispatch` with `agent: "claude"` and a mode. muster creates an isolated worktree for the bead, assembles the bead's Title+Description into a prompt, launches Claude Code against that worktree, marks the bead `in_progress`, and the agent begins working. This is the entire point of M2: a dispatched bead actually runs.

**Why this priority**: Nothing else in M2 has value without the agent actually executing. Every other story (attach, streaming, recovery) is observability or robustness layered on top of this.

**Independent Test**: `POST /api/v1/beads/{id}/dispatch {"agent":"claude","mode":"agent"}` against a bead; observe a `claude` process running in a worktree distinct from the repo root, with the bead moved to `in_progress`/`running`. Verifiable by `tmux ls | grep ^muster/` (or process listing in the tmux-absent fallback) and by inspecting the worktree directory.

**Acceptance Scenarios**:

1. **Given** `--repo` points to a valid git repo and `claude` is installed and authenticated, **When** a user dispatches bead `mp-abc` with `agent:"claude"`, **Then** muster creates a worktree (branch `muster/mp-abc`), writes the prompt file, launches Claude Code in it, returns `202 Accepted` with the bead in `running`, and emits `tmux.session.opened`.
2. **Given** a dispatch is in progress for a bead, **When** the same bead is dispatched again, **Then** muster rejects the second dispatch with `409 CONFLICT` (one active run per bead in M2) rather than spawning a duplicate.
3. **Given** `--repo`/`MUSTER_REPO` is not configured, **When** a user dispatches any bead, **Then** muster returns `400`/`422` with `source repo not configured: set --repo or MUSTER_REPO`.
4. **Given** the agent finishes with exit code 0, **Then** the step is marked `done` and the bead advances to `review`; **Given** a non-zero exit, **Then** the step is marked `failed` and the bead's status reflects the failure (no automatic retry/loop in M2).

---

### User Story 2 — Agent runs in a per-bead isolated worktree (Priority: P1)

Each dispatched bead gets its own git worktree so the agent's file changes are isolated from the main checkout and from other beads. The first (only) step of a bead creates the worktree; the agent's working directory is that worktree.

**Why this priority**: Running an autonomous agent directly in the user's main checkout is unacceptable (it would mutate the working tree and collide across beads). Isolation is a correctness precondition for US1, not an enhancement — hence also P1.

**Independent Test**: Dispatch two different beads; confirm two distinct worktree directories exist on distinct branches, and that edits made by one agent do not appear in the other's working tree or in the main checkout.

**Acceptance Scenarios**:

1. **Given** a bead is dispatched and no worktree exists for it, **When** the run starts, **Then** muster runs `git worktree add` to create `<worktrees-root>/<bead-id>` on branch `muster/<bead-id>` from the configured source repo, and the agent's cwd is that path.
2. **Given** a worktree for the bead already exists (e.g. a prior run), **When** the bead is dispatched again, **Then** muster reuses the existing worktree rather than failing or creating a duplicate.
3. **Given** the source repo path is not a git repository, **When** a bead is dispatched, **Then** muster returns a clear error and does not launch the agent.

---

### User Story 3 — Attach to a live agent session (Priority: P2)

While an agent runs, the user can retrieve a `tmux attach` command and connect to the live terminal to watch the agent's full TUI, scroll back, or type input directly into the session.

**Why this priority**: A core differentiator of running CLI agents through tmux (vs. plain subprocess) is live observability and intervention. It is high-value but the agent still does useful work without it, so P2.

**Independent Test**: With an agent running, `GET /api/v1/beads/{id}/steps/0/attach` returns a tmux command string and pane info; running that command in a terminal connects to the live agent pane. `POST /api/v1/beads/{id}/steps/0/send {"keys":"..."}` forwards keystrokes that appear in the attached session.

**Acceptance Scenarios**:

1. **Given** an agent is running for bead `mp-abc` under tmux, **When** `GET /steps/0/attach`, **Then** the response contains the attach command (`tmux attach -t muster/mp-abc/0/0`) and pane identifier.
2. **Given** an agent is running, **When** `POST /steps/0/send {"keys":"y\n"}`, **Then** the keystrokes are delivered to the live pane and reflected in the agent's input.
3. **Given** tmux is unavailable (fallback mode) or the step is not running, **When** `GET /steps/0/attach`, **Then** muster returns a clear non-error response indicating attach is unavailable and why.
4. **Given** a `send` targets a session that has already exited, **Then** muster returns `409`/`404` rather than silently dropping the input.

---

### User Story 4 — Live runlog streaming over WebSocket (Priority: P2)

The agent's output is captured line-by-line, persisted to an append-only runlog, and broadcast over the existing WebSocket hub so the UI shows output in real time without the user having to attach.

**Why this priority**: The default UI experience for watching an agent is the streamed runlog, not a raw terminal attach. Persistence also enables catch-up after page reload. High value, but the run itself succeeds without it, so P2.

**Independent Test**: Open a WS connection, dispatch a bead, and observe `runlog.line` events arriving as the agent produces output; after the run, the persisted runlog for that bead/step contains the full output in order.

**Acceptance Scenarios**:

1. **Given** a WS client is connected and an agent is running, **When** the agent emits output, **Then** the client receives ordered `runlog.line` events tagged with bead id and step index.
2. **Given** an agent session opens or closes, **Then** `tmux.session.opened` / `tmux.session.closed` events are broadcast.
3. **Given** a burst of rapid output, **Then** runlog writes are batched (not one disk write per line) while WS ordering is preserved.
4. **Given** a client connects mid-run or after reload, **Then** it can retrieve the runlog captured so far (catch-up) before live lines resume.

---

### User Story 5 — Detect and authenticate the Claude Code adapter (Priority: P2)

muster probes for the `claude` CLI (presence, version) and surfaces whether it is authenticated. When it is not, muster provides a login flow. Adapter and tmux availability are reflected in `GET /api/v1/orchestrator/status`.

**Why this priority**: Dispatch (US1) presupposes a detected, authenticated adapter; this story makes that state observable and recoverable. It is P2 because in the common case the user has already logged in via their own terminal and detection is a status read.

**Independent Test**: With `claude` installed, `GET /api/v1/orchestrator/status` reports `tmuxAvailable`, `tmuxVersion`, and that the `claude` adapter is detected with a version. With `claude` absent, status reflects "not detected" and dispatch returns a clear `501`-style error.

**Acceptance Scenarios**:

1. **Given** `claude` is on PATH, **When** muster starts, **Then** `orchestrator/status` includes the adapter id `claude`, its version, `tmuxAvailable`, `tmuxVersion`, and `runningCount`.
2. **Given** `claude` is not installed or not on PATH, **When** a bead is dispatched to `claude`, **Then** muster returns a clear "adapter not available" error and does not attempt to spawn.
3. **Given** `claude` is installed but unauthenticated, **When** the user initiates the login flow, **Then** muster starts an attachable interactive `claude` login session and reports auth success/failure; muster never stores credentials itself.

---

### User Story 6 — Survive a muster restart with a running agent (Priority: P3)

If muster is restarted (crash or intentional) while an agent is running, the agent keeps running (it lives in the user's tmux, not as a child of muster). On startup, muster re-discovers `muster/*` sessions, re-associates them with their beads/steps, and resumes streaming.

**Why this priority**: A real robustness win unique to the tmux transport, but the happy path (US1–US4) delivers M2's value without it; hence P3.

**Independent Test**: Dispatch a bead, confirm the agent is running, kill the muster process, restart muster, and confirm the still-running agent's session is rediscovered, the bead/step shows `running`, and `runlog.line` streaming resumes.

**Acceptance Scenarios**:

1. **Given** an agent is running under tmux and muster is restarted, **When** muster starts up, **Then** it enumerates `muster/*` tmux sessions, matches each to its bead/step, marks the step `running`, and resumes streaming.
2. **Given** a rediscovered session whose bead no longer exists or whose step is already `done`, **Then** muster kills that orphaned session (after a grace period) rather than leaving it dangling.

---

### User Story 7 — Degrade gracefully when tmux is absent (Priority: P3)

If `tmux` (>= the required version) is not installed, CLI adapters fall back to running the agent as a direct child process. Output still streams to the runlog and WS; only live *attach* is disabled, with a clear reason.

**Why this priority**: Broadens portability and prevents a hard dependency from blocking all of M2 on machines without tmux. P3 because the primary target environment has tmux.

**Independent Test**: With tmux uninstalled (or detection forced off), dispatch a bead; confirm the agent runs via direct exec, runlog/WS streaming still works, the run completes, and `GET /steps/0/attach` reports attach unavailable due to missing tmux.

**Acceptance Scenarios**:

1. **Given** tmux is not detected at startup, **When** a bead is dispatched, **Then** the agent runs via direct subprocess and the run completes normally with runlog/WS streaming intact.
2. **Given** the fallback path is active, **When** `GET /steps/0/attach`, **Then** muster returns attach-unavailable with reason `tmux not installed`, and `orchestrator/status` shows `tmuxAvailable: false`.

---

### Edge Cases

- **Agent never produces output / hangs**: a per-run wall-clock timeout (configurable, sane default) cancels the run, marks the step `failed`/`cancelled`, kills the session, and broadcasts closure. M2 documents the default; tunability beyond it is M10.
- **Prompt file cannot be written** (worktree read-only/full): dispatch fails before spawning, with a clear error; no half-started session.
- **tmux session name collision** (stale session from a prior run with the same name): the prior session is killed before the new one is spawned (loop-count suffix in the name reserves room for future multi-run).
- **`send` to a non-CLI / fallback / exited session**: rejected with a clear status, never silently dropped.
- **Attach/send for a step index other than 0** in M2: rejected (`404`) — only `idx=0` exists.
- **Source repo has uncommitted changes / dirty worktree base**: `git worktree add` from a committed ref; document behavior when the base branch is dirty.
- **Concurrent dispatch of the same bead**: second request gets `409 CONFLICT` (one active run per bead in M2).
- **Agent writes the `<muster:subbead …>` / `<muster:checkpoint>` markers**: in M2 these are **not** acted upon (sub-beads = M8); document whether they are stripped from the runlog or passed through verbatim.
- **muster shutdown while an agent runs**: graceful shutdown does **not** kill the agent's tmux session (it must survive per US6); only muster's own streaming goroutines drain.
- **Worktree cleanup**: M2 does not garbage-collect worktrees (that is M9); stale worktrees accumulate and are documented as a known limitation.

---

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST expose an `Adapter` abstraction with operations to detect availability/version, enumerate supported modes, invoke a run (returning a stream of run events), report a quota source, and initiate login. M2 ships exactly one implementation: `claude`.
- **FR-002**: System MUST give `POST /api/v1/beads/{id}/dispatch` a real body for `agent:"claude"`: validate agent+mode, create/claim the bead's worktree, assemble the prompt, launch the agent, and transition the bead to a running state. The endpoint MUST return promptly (the run proceeds asynchronously) and MUST reject a concurrent duplicate run for the same bead with `409 CONFLICT`.
- **FR-003**: System MUST create a per-bead isolated git worktree (`git worktree add` on branch `muster/<bead-id>`) from the configured source repository, reuse an existing one if present, and run the agent with that worktree as its working directory.
- **FR-004**: System MUST resolve the source repository from `--repo` flag → `MUSTER_REPO` env var; if unset, dispatch MUST fail with a clear, actionable error and MUST NOT launch an agent.
- **FR-005**: System MUST assemble the agent's task input from the bead's Title and Description, written to a temp prompt file inside the worktree, and deliver it to the agent via a wrapper that reads the file (avoiding shell-escaping hazards). No constitution/skill assembly is in scope (M6).
- **FR-006**: System MUST resolve the agent invocation (including plan vs agent mode) from the adapter's mode table. The exact `claude` CLI flags MUST be empirically pinned (spike) and recorded in a contract before implementation.
- **FR-007**: For CLI adapters, System MUST run the agent inside a tmux session named `muster/<bead-id>/<step-idx>/<loop-count>` (M2 always `…/0/0`), using tmux as the canonical transport (spawn, pipe output, attach, send keys, capture, kill, list). System MUST probe a minimum tmux version at startup.
- **FR-008**: System MUST fall back to a direct child-process transport when tmux is unavailable, preserving runlog/WS streaming; in fallback mode, attach and send MUST report unavailable with a reason.
- **FR-009**: System MUST capture agent output into an append-only **runlog** keyed by bead id + step index, persisted with batched writes, and ordered.
- **FR-010**: System MUST broadcast new WebSocket event types — `runlog.line`, `tmux.session.opened`, `tmux.session.closed` — in addition to (and without breaking) the M1 `bead.*` events.
- **FR-011**: System MUST expose `GET /api/v1/beads/{id}/steps/{idx}/attach` returning the tmux attach command and pane info (or attach-unavailable + reason), and `POST /api/v1/beads/{id}/steps/{idx}/send` to forward keystrokes to the live pane. In M2 only `idx=0` is valid.
- **FR-012**: System MUST support runlog catch-up: a client connecting mid-run or after reload can retrieve the output captured so far for a bead/step before live lines resume.
- **FR-013**: On agent exit, System MUST mark the step `done` (exit 0) or `failed` (non-zero) and transition the bead accordingly (exit 0 → `review`); M2 performs no automatic retry/loop.
- **FR-014**: On startup, System MUST enumerate surviving `muster/*` tmux sessions, re-associate each with its bead/step, mark running steps `running`, and resume streaming. Sessions with no matching live bead/step MUST be killed after a grace period.
- **FR-015**: System MUST detect the `claude` adapter (presence + version) and tmux (presence + version) at startup and report `tmuxAvailable`, `tmuxVersion`, the detected adapter(s) with versions, and `runningCount` via `GET /api/v1/orchestrator/status`.
- **FR-016**: System MUST provide a login flow for `claude` that runs the vendor's interactive login (via an attachable tmux session when tmux is present) and reports success/failure. System MUST NOT capture, persist, or proxy credentials.
- **FR-017**: System MUST enforce a per-run wall-clock timeout (configurable, with a documented default); on timeout it MUST cancel the run, mark the step failed/cancelled, kill the session, and broadcast closure.
- **FR-018**: Graceful muster shutdown MUST NOT terminate running agent tmux sessions (they must survive for restart recovery); only muster's streaming/goroutine state drains.
- **FR-019**: All M1 REST endpoints, the beads store layer, and existing WebSocket `bead.*` event types MUST remain behaviorally unchanged (additive only).
- **FR-020**: System MUST treat `<muster:subbead …>` and `<muster:checkpoint>` markers as inert in M2 (no sub-bead spawning, no checkpointing); the documented handling (strip vs pass-through) MUST be consistent.

### Key Entities

- **Adapter**: an agent integration — identity, detection result (installed?/version/authenticated?), supported modes, an invoke operation yielding run events, a quota source, and a login flow. M2: only `claude`.
- **Mode**: a named invocation profile for an adapter (e.g. `plan`, `agent`) mapping to a concrete agent invocation.
- **Run / Step**: one agent invocation for a bead. M2 models a **single step at index 0** per bead. Tracks state (`running`/`done`/`failed`/`cancelled`), exit code, timestamps, and its worktree + session.
- **Worktree**: a per-bead isolated checkout (`muster/<bead-id>` branch) of the configured source repo; the agent's working directory.
- **Session** (tmux): the live terminal hosting a CLI agent — name `muster/<bead-id>/<step-idx>/<loop-count>`, pane id, bead/step linkage, start time. Absent in fallback mode.
- **Runlog**: append-only, ordered output for a bead/step, persisted and streamable, supporting catch-up.

### What Changes vs M1

| Aspect | M1 | M2 |
|---|---|---|
| `POST /{id}/dispatch` | stub: shells `bd`, moves the bead | launches the `claude` agent in an isolated worktree |
| Agent execution | none | Claude Code via tmux (or direct-exec fallback) |
| Process isolation | none | per-bead git worktree |
| Output | none | append-only runlog + `runlog.line` WS stream |
| Live interaction | none | `GET …/attach`, `POST …/send` |
| WS events | `bead.*` | `bead.*` + `runlog.line` + `tmux.session.*` |
| `orchestrator/status` | M1 fields | + `tmuxAvailable`, `tmuxVersion`, adapter detection, `runningCount` |
| Runtime deps | `bd` (+ Dolt in remote) | + `tmux` (optional, with fallback), `git` for worktrees |

### What Does NOT Change vs M1

- All M1 REST endpoint paths/shapes and the `{"error":{...}}` format
- The beads store layer (JSONL/Dolt backends) and the `bd` write bridge
- `bead.*` WebSocket event protocol
- Embedded UI serving, `X-Request-ID`, body-size limit, graceful-shutdown drain (extended only to not kill agent sessions)

---

## Success Criteria *(mandatory)*

- **SC-001**: Dispatching a bead to `claude` results in a Claude Code process running in a per-bead worktree distinct from the main checkout, within **5 seconds** of the request, with the bead shown as running.
- **SC-002**: Live agent output reaches a connected WebSocket client as `runlog.line` events within **2 seconds** of the agent producing it.
- **SC-003**: The attach command returned by `GET …/steps/0/attach` successfully connects a terminal to the live agent pane, and keystrokes sent via `POST …/steps/0/send` appear in that session.
- **SC-004**: Two beads dispatched concurrently run in two isolated worktrees; file changes from one are not visible in the other or in the main checkout.
- **SC-005**: After muster is killed and restarted while an agent runs, the still-running agent is rediscovered and its streaming resumes within **5 seconds** of restart, with no loss of the agent process.
- **SC-006**: With tmux uninstalled, a dispatched bead still runs to completion with runlog/WS streaming intact, and attach reports unavailable with a clear reason.
- **SC-007**: On agent exit, the bead reaches `review` (exit 0) or a failed state (non-zero), verifiable via the M1 read endpoints, with the full output retrievable from the runlog.
- **SC-008**: A dispatch with no configured source repo, or to an unavailable/uninstalled adapter, returns a clear actionable error and never leaves a half-started session or worktree.
- **SC-009**: `go test -race ./...` passes — no data races in the new streaming/session/runlog paths.

---

## Assumptions

- **Single source repo (M2 simplification)**: muster serves beads whose worktrees branch from one configured git repo (`--repo`/`MUSTER_REPO`). Per-bead repo routing across many repos is **M7**. *(High-impact assumption — confirm via `/speckit-clarify`.)*
- **Single step per bead**: M2 models one implicit step (index 0). The plan→build→review chain, loop-back, and step pointer are **M4/M8**. *(High-impact assumption.)*
- **Login is detect + attachable interactive session**: muster does not implement or proxy OAuth; it detects auth state and can host the vendor's own interactive login in an attachable session. *(Confirm — affects US5.)*
- **Claude Code CLI flags must be pinned by a spike** before coding (modes, login, non-interactive output), mirroring M1's Phase 7.5 verification discipline — the handoff's `claude --plan` is indicative, not verified.
- **git worktrees only** in M2; `jj` support and the full `wt.Backend` interface (diff/file-list exposure) are **M3**.
- **No quota/cost tracking** parsed from agent output in M2 (the `QuotaSource` exists on the interface but may be a no-op for `claude` until M4).
- **No worktree GC** in M2 (stale worktrees accumulate; cleanup is M9).
- **`tmux >= 3.2`** is the assumed minimum (per handoff §7.1); the exact floor is confirmed at the spike.
- **muster owns no new persistent state of its own beyond the runlog**; the runlog's storage location and retention are defined in the plan (candidate: under the beads dir or a muster-local path), but muster still treats the beads DB as the source of truth for issue state.
- The bead carries enough identity (id, title, description) from M1 to serve as the agent task with no schema change to the beads store.

## Technical Context

### New Components (none exist in the M1 codebase — verified: `internal/` holds only api, config, core, services, store, ws)

- `internal/tmux` — tmux session manager (Detect/Spawn/Attach/Pipe/Send/Capture/Kill/List), with the direct-exec fallback.
- `internal/adapter` — the `Adapter` interface + the `claude` implementation.
- A **runlog** capture/persistence path feeding both storage and the WS hub.
- A **minimal worktree** helper (`git worktree add`/reuse) — a thin precursor to M3's `wt.Backend`.
- New WS event types and the `/steps/{idx}/attach` + `/steps/{idx}/send` endpoints wired into the M1 router.
- `cmd/muster/main.go` wiring: probe tmux/git/`claude` at startup; parse `--repo`; restart-recovery scan.

### New Configuration (anticipated)

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--repo` | `MUSTER_REPO` | (none) | Path to the source git repo worktrees branch from |
| `--worktrees-dir` | `MUSTER_WORKTREES_DIR` | platform temp/muster | Root for per-bead worktrees |
| `--run-timeout` | `MUSTER_RUN_TIMEOUT` | documented default | Per-run wall-clock cap |

### Constitution / Gates Note

The repo's `.specify/memory/constitution.md` is still an unfilled template; M1's "Constitution Check" gates (single binary, embedded UI, handlers separated from logic, no breaking REST/WS changes) derive from `prototype/handoff/spec.md`. M2 preserves all of them: still one binary, still `go:embed` UI, agent orchestration lives behind the new `adapter`/`tmux` packages, and the REST/WS surface is extended additively.

---

## Dependencies & Sequencing

- **Builds on M1** (must be complete — it is): beads store, read/write surface, WS hub, `orchestrator/status`.
- **Pulls forward from M3**: a minimal git-worktree layer (not the full `wt.Backend`).
- **Explicitly excludes**: M4 dispatcher/scheduler/capacity/idempotency-beyond-one-bead, M5 multi-provider, M6 skills/constitution assembly, M8 sub-beads/policy/loops, M9 GC/observability depth.
- **Pre-implementation spike required**: pin the `claude` CLI surface (modes, login, output streaming) and the tmux version floor before `/speckit-plan` finalizes contracts — the single largest correctness risk in M2.
