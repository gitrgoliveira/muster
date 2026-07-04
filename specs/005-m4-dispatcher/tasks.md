# Tasks: M4 — Dispatcher

**Feature**: [spec.md](spec.md) · **Plan**: [plan.md](plan.md) · **Branch**: `claude/optimistic-dijkstra-76a000`

## Format: `- [ ] [TaskID] [P?] [Story?] Description with file path`

- **[P]** = parallelizable (different files, no dependency on an incomplete task). Parallel groups are bounded to **3 concurrent** via `<!-- parallel-group: N -->` comments.
- TDD is **non-negotiable**: within a capability the failing-test task precedes its implementation task (so a test/impl pair on the same file is `<!-- sequential -->`).
- Two **SPIKE** tasks (T030 jj write-side, T059 claude quota) must run and record findings into [research.md](research.md) Spike Log **before** their dependent implementation tasks.
- All new REST/WS surface is additive; the single documented migration (dispatch 409→200+joined) is T048.

---

## Phase 1: Setup (shared scaffolding, no story label)

<!-- parallel-group: 1 (max 3 concurrent) -->
- [ ] T001 [P] Write failing test for `config.ParseMaxConcurrent` (valid int>0, default 4 when empty, typed error on ≤0/non-integer) in `internal/config/dispatcher_test.go`
- [ ] T002 [P] Add additive WS `EventType` constants (`dispatch.queued`, `dispatch.admitted`, `step.advanced`, `step.loopedback`, `worktree.finalized`, `worktree.pushed`, `worktree.removed`, `run.quota`) with a table test asserting they don't collide with M0–M3 types in `internal/ws/event_test.go`
- [ ] T003 [P] Write failing test for additive service error code `CodeStepOutOfRange` / `CodeInvalidCapacity` mapping in `internal/services/beads_test.go`

<!-- sequential (each impl follows its test on the same file) -->
- [ ] T004 Implement `config.ParseMaxConcurrent` (flag `--max-concurrent-dispatches` / `MUSTER_MAX_CONCURRENT_DISPATCHES`, default 4, fail-fast ≤0) in `internal/config/dispatcher.go`
- [ ] T005 Add the new `EventType` constants + doc comments in `internal/ws/event.go` (additive only)
- [ ] T006 Add `CodeStepOutOfRange` and `CodeInvalidCapacity` service codes in `internal/services/beads.go`
- [ ] T007 Wire `--max-concurrent-dispatches` flag parsing into `cmd/muster/main.go` (construct scheduler capacity; fail-fast on error, like `ParseDefaultVCS`)

---

## Phase 2: Foundational (blocking prerequisites — MUST precede user-story phases)

<!-- sequential (shared types touched by US1 + US4) -->
- [ ] T008 Extend the `Run` struct with `Chain *StepChain`, `Quota QuotaUsage`, and waiting/queued distinction (reuse `StepPending` for queued) in `internal/orchestrator/orchestrator.go`; add a compile-only test skeleton in `internal/orchestrator/orchestrator_test.go`
- [ ] T009 Add `DispatchResult { Bead; Joined bool; Queued bool }` (orchestrator return) and mirror as a service-layer type in `internal/orchestrator/orchestrator.go` + `internal/services/beads.go` (kept in sync per existing OrchestratorDispatchRequest pattern)
- [ ] T010 Define `QuotaUsage { Known bool; InputTokens, OutputTokens int64; CostUSD float64 }` placeholder type in `internal/orchestrator/quota.go` (fields only; reader lands in US5)

---

## Phase 3: User Story 1 — Capacity-gated FIFO scheduler (Priority: P1) 🎯 MVP

**Goal**: bounded concurrency with a FIFO waiting queue, auto-admission on completion, runtime-mutable capacity.
**Independent test**: capacity=N, dispatch N+2 → exactly N active, 2 queued FIFO; finishing one admits the next with no client call; runtime resize drains not kills.

<!-- sequential (test before impl, same package state) -->
- [ ] T011 [US1] Write failing `scheduler_test.go`: admission bound (active ≤ capacity), FIFO order of `waiting`, fail-fast on non-positive capacity in `internal/orchestrator/scheduler_test.go`
- [ ] T012 [US1] Write failing `-race` test: N concurrent dispatches at capacity N-k admit exactly N-k and enqueue k in FIFO order (no data race) in `internal/orchestrator/scheduler_test.go`
- [ ] T013 [US1] Write failing test: auto-admit next FIFO waiter when a run reaches a terminal state (success/failure/cancel/timeout) in `internal/orchestrator/scheduler_test.go`
- [ ] T014 [US1] Write failing test: `SetCapacity` raises→admits up to new limit; lowers→drains (never kills active) ; rejects ≤0 in `internal/orchestrator/scheduler_test.go`
- [ ] T015 [US1] Implement `scheduler.go` (capacity, active set, FIFO waiting queue, `admitOrEnqueue`, `onRunEnd`, `SetCapacity`, `Snapshot`) guarded by orchestrator `mu` in `internal/orchestrator/scheduler.go`
- [ ] T016 [US1] Integrate scheduler into `Orchestrator.Dispatch`: admit-or-enqueue under the existing reservation lock in `internal/orchestrator/orchestrator.go`
- [ ] T017 [US1] Hook `onRunEnd` admission into the watcher-completion path (session end/timeout/cancel) in `internal/orchestrator/orchestrator.go` (+ `runlog.go`/watcher as needed)

<!-- sequential (service → api layering) -->
- [ ] T018 [US1] Add `SetCapacity` + scheduler `Snapshot` accessors to the service layer with failing test first in `internal/services/beads_test.go` then `internal/services/beads.go`
- [ ] T019 [US1] Write failing handler test for `PUT /orchestrator/capacity` (200 on n>0 with snapshot body; 400 on ≤0) in `internal/api/beads/handlers_test.go`
- [ ] T020 [US1] Implement thin `PUT /orchestrator/capacity` handler + register route in `internal/api/beads/handlers.go` and `internal/api/router.go`
- [ ] T021 [US1] Add additive `capacity`/`activeCount`/`waiting[]` fields to the orchestrator-status DTO (failing test first) in `internal/api/health/` (+ its test)
- [ ] T022 [US1] Emit `dispatch.queued` / `dispatch.admitted` WS events on enqueue/admit (failing test first) in `internal/orchestrator/orchestrator.go`

**Checkpoint US1**: scheduler + capacity endpoint + status fields + events green; `go test -race ./internal/orchestrator/... ./internal/api/... ./internal/services/...` passes.

---

## Phase 4: User Story 2 — Worktree write-side finalize/push/remove (Priority: P1)

**Goal**: fill `wt.Backend.Finalize/Push/Remove` (git + jj), no-op-empty finalize, `muster/<id>→origin` push, explicit errors, VCS_UNAVAILABLE.
**Independent test**: create worktree, mutate file, finalize→commit exists; empty→no-op; push→branch on bare remote; remove→Status absent; both git and jj (skip if binary absent).

<!-- sequential (spike first, records research.md) -->
- [ ] T030 [US2] **SPIKE (real jj ≥0.42)**: pin the jj Finalize (describe/new), Push (`jj git push --branch`), Remove (workspace forget) commands + no-op-empty detection; record byte-level commands/output into [research.md](research.md) Spike Log R6

<!-- parallel-group: 2 (max 3 concurrent) — independent test files -->
- [ ] T031 [P] [US2] Write failing `internal/wt/writeside_test.go` **git** cases: finalize commits on `muster/<id>`, empty=no-op success, push reaches a **bare** upstream, remove→Status absent, VCS_UNAVAILABLE when git absent (fake-on-$PATH + real-git integration, skip-gated)
- [ ] T032 [P] [US2] Write failing `internal/wt/jj_writeside_test.go` **jj** cases mirroring T031 (real-jj integration, skip when `jj` absent)
- [ ] T033 [P] [US2] Write failing `internal/wt/remote_test.go`: remote-name resolution (default `origin`, configurable) and branch = `muster/<beadID>`

<!-- sequential (impl follows the tests above; git.go & jj.go are different files but share the interface) -->
- [ ] T034 [US2] Implement git `Finalize`/`Push`/`Remove` (replace `ErrNotImplemented`) per [contracts/wt-writeside.md](contracts/wt-writeside.md) in `internal/wt/git.go`
- [ ] T035 [US2] Implement jj `Finalize`/`Push`/`Remove` (replace `ErrNotImplemented`) using the spike-pinned commands in `internal/wt/jj.go`
- [ ] T036 [US2] Implement `remote.go` remote-name resolution + branch helper in `internal/wt/remote.go`

<!-- sequential (service → api) -->
- [ ] T036a [US2] **(review C5)** Widen the `services.WorktreeAccessor` interface with `Finalize`/`Push`/`Remove`, add the three passthrough methods to `worktreeAccessorAdapter` (thin delegation to `a.o.backend.Finalize/Push/Remove` — `worktreesDir` already baked in), and update the `var _ services.WorktreeAccessor` compile-time assertion in `internal/orchestrator/worktree_adapter.go` + `internal/services/beads.go` (failing/compile test first). Without this the write-side does not compile.
- [ ] T037 [US2] Add `FinalizeWorktree`/`PushWorktree`/`RemoveWorktree` service methods that permit finalize/remove only in terminal run states (`StepDone`/`StepFailed`) and reject `StepActive`/`StepPending` with `CodeRunAlreadyActive` **(review M1)** — failing test first (incl. the finish-window race) in `internal/services/beads_test.go` → `internal/services/beads.go`
- [ ] T038 [US2] Write failing handler tests for `POST /beads/{id}/worktree/finalize`, `POST .../worktree/push`, `DELETE /beads/{id}/worktree` in `internal/api/beads/handlers_test.go`
- [ ] T039 [US2] Implement the three thin handlers + register routes in `internal/api/beads/handlers.go` and `internal/api/router.go`
- [ ] T040 [US2] Emit `worktree.finalized`/`worktree.pushed`/`worktree.removed` events (failing test first) in the service/orchestrator layer

**Checkpoint US2**: write-side green for git (and jj when present); M3 read endpoints untouched.

---

## Phase 5: User Story 3 — Multi-step chain + operator-driven advance/loopback (Priority: P2)

**Goal**: per-bead step chain (config default + per-dispatch override), per-step profiles, operator-driven advance/loopback, `/steps/{idx}` accepts idx>0.
**Independent test**: dispatch a 3-step chain; attach idx=1,2; advance 0→1→2; loopback 2→1; pointer + events correct; M2 single-step default unchanged.

<!-- sequential (test before impl) -->
- [ ] T041 [US3] Write failing `steps_test.go`: chain resolution order (request override → config default → single step 0), advance range-check (< len), loopback range-check (≥0 and < current), `ErrStepOutOfRange` on out-of-range in `internal/orchestrator/steps_test.go`
- [ ] T042 [US3] Write failing test asserting M2 single-step (idx 0, no chain) behavior is byte-for-byte preserved in `internal/orchestrator/steps_test.go`
- [ ] T043 [US3] Implement `steps.go` (`StepChain`, `StepProfile{Name,PermissionMode,PromptRef}`, pointer via `Run.StepIdx`, `Advance`, `LoopBack`, chain resolution) in `internal/orchestrator/steps.go`. **(review L2)** `PromptRef` is stored but resolves to the M2 bead prompt in M4 (real assembly is M6) — do not build M6 assembly here.
- [ ] T043a [US3] **(review C4)** Thread `stepIdx` through the launch path (currently hard-coded to literal `0`): session name via `tmux.SessionName(beadID, stepIdx, loop)` (orchestrator.go:593), prompt file `.muster-prompt-<stepIdx>.txt` (orchestrator.go:378), `runlogStreamer.stepIdx` (orchestrator.go:648), and the `tmux.session.opened/closed` events' `StepIdx` (orchestrator.go:659). Failing test first asserting step 1's session/prompt do not collide with step 0's. This is the bulk of US3.
- [ ] T043b [US3] **(review C2)** Specify + implement the run-vs-step interlock: advance/loopback finishes the current step's run, then starts the next step's run under the same `beadID` key carrying forward `Chain` + accumulated `Quota`; `finishRun` must NOT evict a bead whose chain has a pending advance. Add a `-race` test: "advance while the prior step's watcher is finishing" (guards the identity-eviction race at orchestrator.go:314-322/486-495). Failing test first in `internal/orchestrator/orchestrator_test.go`.
- [ ] T044 [US3] Add optional `chain` override to `DispatchRequest` + a configured default chain (never silently default a step's permission mode) in `internal/orchestrator/orchestrator.go` (+ `internal/config/` if a config knob is needed)

<!-- sequential (handler widening + routes) -->
- [ ] T045 [US3] Widen `parseStepIdx` to accept `idx≥0` (was literal `"0"` only), still rejecting negative/out-of-range; update the existing handler test in `internal/api/beads/handlers.go` + `handlers_test.go`
- [ ] T046 [US3] Write failing handler tests then implement `POST /beads/{id}/steps/advance` and `POST /beads/{id}/steps/loopback` (thin) + routes in `internal/api/beads/handlers.go`, `internal/api/router.go`, `handlers_test.go`
- [ ] T047 [US3] Emit `step.advanced` / `step.loopedback` events + additive `stepIdx`/`chainLen` status fields (failing test first) in orchestrator + `internal/api/health/`

**Checkpoint US3**: multi-step chain drivable end-to-end; all M2 `/steps/0` tests still green.

---

## Phase 6: User Story 4 — Idempotent dispatch (Priority: P2) — includes the one migration

**Goal**: bead-identity idempotency; in-flight duplicate returns existing run (200 + `joined:true`); racing duplicates → exactly one run; idempotent after recovery.
**Independent test**: two identical dispatches (sequential + racing) → one run, second `joined:true`; fresh dispatch after completion starts a new run.

<!-- sequential (migration + impl are coupled) -->
- [ ] T048 [US4] **MIGRATION**: rewrite M2 tests `TestDispatch_409_RunAlreadyActive` (in `internal/api/beads/handlers_test.go`) and `TestDispatch_409_DuplicateRun` (in `internal/orchestrator/orchestrator_test.go`) to assert the idempotent contract (200 + existing run + `joined:true`); update `maperr_internal_test.go` if the sentinel mapping changes
- [ ] T049 [US4] Change `Orchestrator.Dispatch` to return `DispatchResult{Joined:true, Bead: existing}` for an in-flight bead instead of `ErrRunAlreadyActive`, under the existing `mu`. **(review M2)** The in-flight check MUST consider **both** `StepActive` (running) and `StepPending` (waiting in the queue) — a duplicate dispatch of a *waiting* bead joins the waiter. Join on bead-identity regardless of params (no key header). Add a test "duplicate dispatch of a waiting bead joins the waiter" in `internal/orchestrator/orchestrator.go` + `orchestrator_test.go`
- [ ] T050 [US4] Update `service_adapter.go` + the `Dispatch` handler to render 200 + `joined`/`queued` fields in `internal/orchestrator/service_adapter.go`, `internal/api/beads/handlers.go`
- [ ] T051 [US4] Write failing `-race` test: many racing identical dispatches yield exactly one run + no leaked session/goroutine in `internal/orchestrator/orchestrator_test.go`
- [ ] T052 [US4] Write failing test: after a run reaches terminal state, a fresh dispatch starts a NEW run (no permanent lock) in `internal/orchestrator/orchestrator_test.go`
- [ ] T053 [US4] **(review C1 — CRITICAL)** Recovery: (a) relax the `recovery.go:80` guard that currently **kills** any recovered session with `StepIdx != 0` — a `StepIdx` within `[0, chainLen)` must **re-register** (not kill) so a live build/review agent survives a restart; keep killing genuinely malformed/negative indices; (b) **rewrite** `TestRecoverSessions_UnsupportedIndicesKilled` (recovery_test.go:149) to assert the new boundary (list it in Complexity Tracking per C3); (c) decide the chain-unknown-at-recovery behavior — the session name carries `StepIdx` but NOT the chain, so a recovered run reconstructs a single-step run at that index and **refuses to advance until re-dispatched** (document this in research.md R9); (d) register recovered actives into the scheduler active set (may transiently exceed capacity → drains, per review L1) and treat a recovered run as in-flight for idempotency. Failing tests first in `internal/orchestrator/recovery.go` + `recovery_test.go`

**Checkpoint US4**: idempotent `/dispatch`; the two migrated tests + new race/recovery tests green.

---

## Phase 7: User Story 5 — Quota from on-disk session record (Priority: P3, best-effort)

**Goal**: read Claude Code's on-disk per-session usage after a run; surface on status + `run.quota`; `QuotaSource`→`QuotaCLIOutput`; unknown on missing/garbled.
**Independent test**: fake session file with known payload → parsed quota on run; no payload → `known:false`, run still succeeds.

<!-- sequential (spike first, records research.md) -->
- [ ] T059 [US5] **SPIKE (real claude)**: pin the on-disk session usage path + JSON field names + run→session correlation (session id / worktree cwd / mtime); record path + redacted sample into [research.md](research.md) Spike Log R8. If no stable record exists, mark US5 dropped and skip T060–T063.

<!-- sequential -->
- [ ] T060 [US5] Write failing `quota_test.go`: parse a fake on-disk session file → `QuotaUsage{Known:true,...}`; missing/garbled → `{Known:false}`; never errors the run in `internal/orchestrator/quota_test.go`
- [ ] T061 [US5] Implement the on-disk quota reader (spike-pinned path/format) in `internal/orchestrator/quota.go`
- [ ] T062 [US5] Change claude adapter `QuotaSource()` from `QuotaNone` to `QuotaCLIOutput` (failing test first) in `internal/adapter/claude/claude.go`. **(review C3)** This rewrites the passing `TestQuotaSource_None` (claude_test.go:232) — an intentional additive-in-spirit test migration listed in Complexity Tracking. Update `claude_test.go`.
- [ ] T063 [US5] Capture quota at run end, attach to `Run`, emit `run.quota` event + additive status field (failing test first) in `internal/orchestrator/orchestrator.go` + `internal/api/health/`

---

## Phase 8: Polish & Cross-Cutting (no story label)

<!-- parallel-group: 3 (max 3 concurrent) — independent files -->
- [ ] T070 [P] Add/adjust per-package coverage gates for M4 in the `thresholds` map of `Makefile` (orchestrator/wt/config/services/api/ws — no regression)
- [ ] T071 [P] Update the README Flags table with `--max-concurrent-dispatches` and `--push-remote` (additive docs) in `README.md`
- [ ] T072 [P] Update [research.md](research.md) Spike Log check-boxes to done with the pinned contracts recorded

<!-- sequential (whole-repo gates last) -->
- [ ] T073 Run `make test` (`go test -race ./...`) and fix any failure; confirm M0–M3 suites green except the two intentionally migrated dispatch tests
- [ ] T074 Run `make cover-check` and `make lint`; resolve gate/lint failures
- [ ] T075 Walk [quickstart.md](quickstart.md) against a temp git repo (and jj repo if present) to confirm the end-to-end operator flow

---

## Dependencies & Execution Order

- **Phase 1 → Phase 2 → Phases 3–7 → Phase 8.** Phase 2 (foundational types) blocks the story phases.
- **US1 (P1)** and **US2 (P1)** are the MVP; US2's write-side is independent of US1's scheduler (different packages) — **US1 and US2 can proceed in parallel after Phase 2** (orchestrator vs wt).
- **US3, US4** depend on Phase 2 + touch the orchestrator; run after US1 (shared `orchestrator.go`/Dispatch). **US4's migration (T048–T050)** should land after US1 wires the scheduler into Dispatch to avoid churn.
- **US5 (P3)** is last and independently droppable (spike-gated).
- **SPIKES**: T030 before T035 (jj impl); T059 before T060–T063 (quota).

## Parallel Opportunities

- Phase 1: T001/T002/T003 (group 1) concurrently.
- After Phase 2: one subagent on **US1** (orchestrator/scheduler) and one on **US2** (wt write-side) concurrently — different packages, no shared files.
- Phase 4: T031/T032/T033 (group 2) — three independent test files.
- Phase 8: T070/T071/T072 (group 3).

## MVP Scope

**US1 + US2** (both P1) deliver the headline: a real capacity-gated dispatcher plus the ability to commit/push an agent's work product. US3/US4/US5 are incremental additive layers.

## Task Count

57 distinct task IDs across phases (T001–T010, T011–T022, T030–T040, T041–T047, T048–T053, T059–T063, T070–T075; numbering gaps T023–29 and T054–58 are intentional per-phase headroom). Test tasks precede their implementations throughout (TDD).
