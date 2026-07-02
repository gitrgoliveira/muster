# Tasks: M2 — First CLI Adapter (Claude Code)

**Feature**: `003-m2-cli-adapter` | **Plan**: [plan.md](plan.md) | **Spec**: [spec.md](spec.md)

**Tests**: INCLUDED — the constitution (Principle IV, NON-NEGOTIABLE) and the plan mandate test-first per layer. Each story's tests precede its implementation.

## Conventions

- Layout is the existing Go tree: `internal/…`, `cmd/muster/…` (no `src/`).
- `[P]` = parallelizable (different files, no incomplete-task dependency).
- Story labels `[US1]`–`[US7]` map to the spec's user stories.
- Fakes: a fake `tmux` and fake `claude` on `$PATH` (the M1 fake-`bd` pattern) plus a real-`git` tmpdir helper. Integration tests gate on real `tmux` (skip if absent).

## Structural note (read first)

M2's user stories share one engine (transport + adapter + worktree + orchestrator), so they are **not** fully independent — acknowledged in spec/plan. Foundational (Phase 2) holds only cheap shared seams (interfaces, enums, config, WS types). Each story phase implements the *behavior* and tmux methods it needs. **MVP = Phase 1 + Phase 2 + US2 + US1** (a bead that actually runs in isolation). US2 precedes US1 because dispatch runs inside a worktree.

---

## Phase 1: Setup (Shared Infrastructure)

- [x] T001 Create package skeletons with `doc.go` for `internal/adapter/`, `internal/adapter/claude/`, `internal/tmux/`, `internal/worktree/`, `internal/orchestrator/`
- [x] T002 [P] Add fake `tmux` test binary (argv-recording shell script) under `internal/tmux/testdata/`
- [x] T003 [P] Add fake `claude` test binary (canned `auth status --json` + controllable exit) under `internal/adapter/claude/testdata/`
- [x] T004 [P] Add a tmpdir git-repo test helper (init + initial commit) in `internal/worktree/testhelp_test.go`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: cheap shared seams every story builds on. No user-facing behavior yet. MUST complete before Phase 3+.

- [x] T005 [P] Add `core.PermissionMode` enum + `Valid()` allow-list (`default`/`acceptEdits`/`dontAsk`/`bypassPermissions`/`plan`/`auto`) with test in `internal/core/enums.go` + `internal/core/enums_test.go`
- [x] T006 Parse new flags/env into config: repeatable `--repo prefix=path` → `RepoMap`, `--worktrees-dir`, `--run-timeout`, `--default-permission-mode` (`MUSTER_*` env), with tests in `internal/config/`
- [x] T007 [P] Add WS event types `runlog.line`, `tmux.session.opened`, `tmux.session.closed` and `Frame` fields (`BeadID`,`StepIdx`,`Seq`,`Data`,`Session`,`ExitCode`) in `internal/ws/event.go` (additive; keep M1 frames intact)
- [x] T008 Define `Adapter` interface + `RunEvent`/`DetectResult`/`Mode`/`Spec`/`QuotaSource` + `Registry`, with interface/registry tests in `internal/adapter/adapter.go`, `internal/adapter/registry.go`, `internal/adapter/adapter_test.go`
- [x] T009 [P] Define `tmux.Manager` interface + `Session` type + `name.go` encode/parse (`muster/<bead>/<step>/<loop>`) with tests in `internal/tmux/manager.go`, `internal/tmux/name.go`, `internal/tmux/name_test.go`
- [x] T010 Implement orchestrator `Run` registry (one-active-run-per-bead, `sync.RWMutex`) + skeleton `Orchestrator` type with test in `internal/orchestrator/orchestrator.go`, `internal/orchestrator/run_test.go`
- [x] T011 Inject `Orchestrator` dependency into `services.BeadService` (new constructor param; nil-safe) keeping all M1 tests green, in `internal/services/beads.go`

---

## Phase 3: User Story 2 — Per-bead isolated worktree (P1)

**Goal**: an agent runs in its own git worktree, never the main checkout. **Independent test**: two beads → two distinct worktrees/branches; edits in one are invisible in the other and in the main checkout; non-git repo errors cleanly.

- [x] T012 [US2] Test `worktree.Ensure` create + reuse + non-git-repo error + two-bead isolation against a real git tmpdir, in `internal/worktree/worktree_test.go`
- [x] T013 [US2] Implement `worktree.Ensure(worktreesDir, repoPath, beadID)` → `git worktree add -b muster/<beadID> <path>` (reuse existing) in `internal/worktree/worktree.go`
- [x] T014 [US2] Resolve `--worktrees-dir` from config and pass into the orchestrator; default to `~/.muster/worktrees` (falling back to `<platform-temp>/muster/worktrees` if `$HOME` is unavailable), in `internal/orchestrator/orchestrator.go` + `cmd/muster/main.go`

**Checkpoint**: worktrees can be created/reused in isolation, exercised by tests.

---

## Phase 4: User Story 1 — Dispatch a bead to Claude Code (P1, MVP)

**Goal**: `POST /dispatch` makes the agent actually run. **Depends on**: Phase 2 + US2. **Independent test**: dispatch `mp-abc` → a `claude` process runs in `muster/mp-abc` worktree, bead is `running`, `tmux.session.opened` fires; on exit the run's outcome is recorded on the bead (M2 records completion via an appended note + `bead.updated`, not a persisted `review` column — see spec.md's M2 note).

- [x] T015 [US1] Test claude adapter `Detect` (parse `claude auth status --json`), `Modes` (plan→`--permission-mode plan`; agent→`--permission-mode <pm>`), `Invoke` Spec, `Login`=ErrNotSupported — using fake `claude`, in `internal/adapter/claude/claude_test.go`
- [x] T016 [US1] Implement claude adapter per [contracts/claude-adapter.md](contracts/claude-adapter.md) in `internal/adapter/claude/claude.go`; register it in `internal/adapter/registry.go`
- [x] T017 [US1] Test tmux `Spawn` (default socket, `remain-on-exit on`) + `DeadStatus` (`#{pane_dead_status}`) + `Kill` against fake `tmux`, in `internal/tmux/manager_test.go`
- [x] T018 [US1] Implement tmux `Detect`/`Spawn`/`DeadStatus`/`Kill` in `internal/tmux/manager.go`
- [x] T019 [US1] Test orchestrator `Dispatch` happy path + `409` duplicate-run + `422` unmapped-prefix + permission-mode resolution (request→default→error), with fakes, in `internal/orchestrator/orchestrator_test.go`
- [x] T020 [US1] Implement `Orchestrator.Dispatch`: resolve repo (`repomap`) → `worktree.Ensure` → write `<worktree>/.muster-prompt-0.txt` (bead Title+Desc) → `adapter.Invoke` → `tmux.Spawn` → register `Run` → goroutine watches exit (`DeadStatus`) → record outcome on the bead (note + `bead.updated`; no persisted `review` column in M2) → emit `tmux.session.opened`/`closed`, in `internal/orchestrator/orchestrator.go`
- [x] T021 [US1] Implement `repomap.Resolve(beadID)` (prefix before first `-`) + permission-mode allow-list/default resolution in `internal/orchestrator/repomap.go` (+ test)
- [x] T022 [US1] Test dispatch handler: `202` running, `409` duplicate, `422` unmapped/unknown agent-mode-perm, `501` adapter-missing, unauth → actionable message, in `internal/api/beads/handlers_test.go`
- [x] T023 [US1] Wire `services.Dispatch` → `Orchestrator.Dispatch`; implement real `handlers.Dispatch` body (validate agent+mode+`permissionMode`) in `internal/services/beads.go` + `internal/api/beads/handlers.go`
- [x] T024 [US1] Integration test (skip if no real `tmux`): dispatch → fake `claude` runs in worktree → bead `running` → exit 0 → completion recorded via note + `bead.updated` (no `review` column in M2), in `internal/orchestrator/integration_test.go`

**Checkpoint**: MVP — a dispatched bead runs Claude Code in isolation and transitions on exit.

---

## Phase 5: User Story 4 — Live runlog streaming over WebSocket (P2)

**Goal**: the agent's output streams to the UI as `runlog.line`. **Independent test**: WS client sees ordered `runlog.line` during a run; a late joiner gets `capture-pane` catch-up.

- [x] T025 [US4] Test tmux `Pipe` (raw bytes) + `Capture` (`capture-pane -ep -S -`) against fake `tmux`, in `internal/tmux/manager_test.go`
- [x] T026 [US4] Implement tmux `Pipe`/`Capture` in `internal/tmux/manager.go`
- [x] T027 [US4] Implement orchestrator runlog fan-out: pipe reader → seq-numbered `runlog.line` frames to `ws.Hub`; `capture-pane` catch-up accessor, in `internal/orchestrator/runlog.go` (raw bytes, no ANSI strip — plan D1)
- [x] T028 [US4] Integration test: WS client receives ordered `runlog.line` + `tmux.session.opened`/`closed` for a run, in `internal/orchestrator/integration_test.go`

**Checkpoint**: runs are observable in real time without attaching.

---

## Phase 6: User Story 3 — Attach to a live agent session (P2)

**Goal**: retrieve a `tmux attach` command and forward keystrokes. **Independent test**: `GET …/steps/0/attach` returns a working command + pane; `POST …/steps/0/send` delivers keys; `idx≠0`→`404`; not-running→`available:false`.

- [x] T029 [US3] Test tmux `Attach` (command string) + `Send` against fake `tmux`; handler cases (idx≠0, not-running, fallback), in `internal/tmux/manager_test.go` + `internal/api/beads/handlers_test.go`
- [x] T030 [US3] Implement tmux `Attach`/`Send` in `internal/tmux/manager.go`
- [x] T031 [US3] Add routes + handlers `GET /api/v1/beads/{id}/steps/{idx}/attach` and `POST /api/v1/beads/{id}/steps/{idx}/send` in `internal/api/router.go` + `internal/api/beads/handlers.go` (send route uses M1 `BodyLimit` middleware)

**Checkpoint**: users can watch and intervene in a live agent.

---

## Phase 7: User Story 5 — Detect adapter + surface auth state (P2)

**Goal**: `orchestrator/status` reports tmux + adapter availability; dispatch to a missing/unauth adapter fails clearly. **Independent test**: status shows `tmuxAvailable`/`tmuxVersion`/`adapters[]`/`runningCount`; uninstalled `claude` → clear dispatch error.

- [x] T032 [US5] Test status DTO additions + adapter/tmux detection surfacing + dispatch-to-missing/unauth errors, in `internal/api/health/handler_test.go`
- [x] T033 [US5] Add `TmuxAvailable`/`TmuxVersion`/`RunningCount`/`Adapters []AdapterInfo` to the status DTO + handler in `internal/api/health/dto.go` + `internal/api/health/handler.go`
- [x] T034 [US5] Probe `tmux.Detect` + `adapter.Detect` at startup; feed results into status + orchestrator transport selection, in `cmd/muster/main.go`

**Checkpoint**: operational state is observable; bad dispatches fail fast.

---

## Phase 8: User Story 6 — Survive a muster restart (P3)

**Goal**: a running agent survives restart; muster re-discovers and resumes streaming. **Independent test**: dispatch → kill muster → restart → session rediscovered, bead `running`, streaming resumes; orphan session killed after grace.

- [x] T035 [US6] Test tmux `List` (`muster/` prefix) + recovery scan rebuilding the `Run` registry + orphan-kill-after-grace, with fake `tmux`, in `internal/orchestrator/recovery_test.go`
- [x] T036 [US6] Implement tmux `List` + `orchestrator/recovery.go` startup scan (re-`Pipe`, resume; kill unmatched after grace) in `internal/tmux/manager.go` + `internal/orchestrator/recovery.go`
- [x] T037 [US6] Call the recovery scan during startup in `cmd/muster/main.go`

**Checkpoint**: muster restart no longer loses or orphans running agents.

---

## Phase 9: User Story 7 — Degrade gracefully when tmux is absent (P3)

**Goal**: agents still run (direct exec) without tmux; attach/send report unavailable. **Independent test**: with tmux off, dispatch runs to completion with streaming; `attach` → unavailable + reason.

- [x] T038 [US7] Test fallback transport: child runs, stdout/stderr stream, exit via `Wait()`; attach/send return unavailable, in `internal/tmux/fallback_test.go`
- [x] T039 [US7] Implement direct-exec fallback transport in `internal/tmux/fallback.go`
- [x] T040 [US7] Orchestrator selects transport by `tmux.Detect`; attach/send report `available:false`+reason; warn when a prompting permission-mode is chosen in fallback (FR-021), in `internal/orchestrator/orchestrator.go`

**Checkpoint**: M2 works on hosts without tmux (attach disabled).

---

## Phase 10: Polish & Cross-Cutting Concerns

- [x] T041 [P] Enforce optional `--run-timeout` (cancel run, kill session, emit `closed`/failed) per FR-017, in `internal/orchestrator/orchestrator.go` (+ test)
- [x] T042 [P] Treat `<muster:subbead …>`/`<muster:checkpoint>` markers as inert with consistent handling (FR-020), in `internal/orchestrator/runlog.go` (+ test)
- [x] T043 [P] Verify graceful shutdown does NOT kill agent tmux sessions (FR-018), in `cmd/muster/main.go` (+ test)
- [x] T044 [P] Update [quickstart.md](quickstart.md) + README + `make help`/startup banner to mention `--repo`, `--worktrees-dir`, tmux dependency
- [x] T045 [P] Add `bd remember` note — recorded cross-session gotchas: fake script permissions, FR-018 context isolation, fifo coverage gap, e2e build tag
- [x] T046 Confirm per-package coverage gates (plan targets) and `go test -race ./...` clean
- [x] T047 Validate the [quickstart.md](quickstart.md) end-to-end walkthrough — automated path: `make test-e2e` (real claude + real tmux; gated by `//go:build e2e`)

---

## Dependencies & Execution Order

- **Phase 1 (Setup)** → **Phase 2 (Foundational)** block everything.
- **US2 (Phase 3)** before **US1 (Phase 4)** — dispatch runs inside a worktree.
- **US1** is the MVP. **US4/US3/US5 (P2)** build on US1's running session; **US6/US7 (P3)** build on the transport.
- tmux.Manager methods are distributed: `Spawn/DeadStatus/Kill` (US1), `Pipe/Capture` (US4), `Attach/Send` (US3), `List` (US6), fallback (US7) — all against the interface from T009.
- Polish (Phase 10) last.

```
Setup → Foundational → US2 → US1 (MVP) → US4 → US3 → US5 → US6 → US7 → Polish
                                   └── P2 stories ──┘   └─ P3 ─┘
```

## Parallel Opportunities

- Setup: T002, T003, T004 in parallel.
- Foundational: T005, T007, T009 in parallel (distinct files); T006/T008/T010/T011 follow.
- Within a story, the test task precedes its impl (TDD); `[P]` polish tasks T041–T045 can run together once behavior lands.

## Implementation Strategy

1. **MVP**: Phases 1–2 → US2 → US1. Stop and demo: a dispatched bead runs Claude Code in an isolated worktree and transitions on exit.
2. **Observability increment**: US4 (runlog) + US3 (attach/send).
3. **Operability increment**: US5 (status/detect).
4. **Robustness increment**: US6 (recovery) + US7 (fallback).
5. **Polish**: Phase 10.

Each increment is independently demoable and leaves `go test -race ./...` green.
