# Feature Specification: M4 — Dispatcher

**Feature Branch**: `claude/optimistic-dijkstra-76a000`

**Created**: 2026-07-04

**Status**: Draft

**Input**: Promote muster's single-shot dispatch (M2) into a real **dispatcher**: a capacity-gated scheduler, a finalize/push worktree write-side, a multi-step plan→build→review chain, safe-to-retry (idempotent) dispatch, and quota/cost tracking parsed from the agent CLI output. Per the canonical roadmap (`prototype/handoff/spec.md` §20, milestone **M4 — Dispatcher**): "Real scheduler, capacity gating, quota tracking from CLI output, idempotency on `/dispatch`." Directory numbered `005-*` to continue the `001/002/003/004` sequence (M0/M1/M2/M3); "M4" and `005-m4-dispatcher` refer to the same milestone.

> **Naming note:** the roadmap labels this milestone **M4**. The spec directory is `005-m4-dispatcher` to continue the existing numbering. Both names refer to the same milestone throughout.

## Context & Prior-Milestone Boundary

M2 shipped a **single-shot** dispatch: one bead → one implicit step (index 0) → one agent process in one worktree, with duplicate-run protection (`ErrRunAlreadyActive`) but **no queue, no capacity limit, no quota tracking, and no write-back** of the agent's work. M3 exposed the agent's work product read-only (file list + unified diff) over a VCS-agnostic `wt.Backend`, and deliberately left the write-side methods — `Finalize`, `Push`, `Remove` — as `ErrNotImplemented` sentinels "for M4 to fill without changing the interface."

M4 is the milestone where dispatch stops being one-shot and best-effort and becomes a **managed lifecycle**: work is admitted under a capacity bound, a bead can progress through more than one step (plan→build→review) and loop back, a completed worktree can be committed and pushed, retries of a dispatch are safe, and the agent's own reported cost/quota is surfaced. It stays within the Constitution: beads remains the source of truth (all issue-state writes go through `bd`), handlers stay thin, the REST/WS surface is strictly additive, and every layer is TDD'd and `-race` clean.

## Clarifications

### Session 2026-07-04

- Q: At capacity (N active runs), what should an over-limit `POST /dispatch` do? → A: **Queue (FIFO, auto-admit)** — accept the dispatch, mark it *waiting*, and launch it automatically when a slot frees.
- Q: What drives plan→build→review advancement and loop-back in M4? → A: **Operator-driven API** — an explicit API call advances the step pointer or loops back; muster provides only the mechanism. The automatic policy engine (auto-split/escalation/auto-loop) stays in M8.
- Q: Default maximum concurrent dispatches when the operator sets no explicit limit? → A: **4**, and the limit MUST be **runtime-configurable** (an additive endpoint so a UI can change it live), not only a startup flag.
- Q: How should Finalize behave on a worktree with no changes? → A: **No-op success** — report "nothing to commit" as a successful no-op (no commit created); idempotent and retry-friendly.
- Q: Given the interactive TUI transport (raw ANSI bytes that redraw), how should M4 capture cost/usage? → A: **On-disk session file** — after the run, read Claude Code's own persisted per-session usage/cost record (ANSI-free, robust). A plan-phase spike pins the exact path/format; absent/unreadable → *unknown* (best-effort).
- Q: Where does a bead's step chain (plan→build→review) come from? → A: **Both** — a configured **default chain** applies when a dispatch omits one, and a `POST /dispatch` request MAY override it with an explicit ordered step list. Omitting a chain entirely still yields the M2 single implicit step (index 0).
- Q: Does each step carry its own agent invocation profile? → A: **Per-step profile** — each step defines its own permission mode + prompt (e.g. plan→'plan'/read-only, build→editing, review→read-only), reusing the existing per-`Mode` permission-mode plumbing.
- Q: How is a duplicate/retry dispatch identified for idempotency? → A: **Bead identity + in-flight state** — a dispatch for a bead that already has an active-or-waiting run joins that run; no separate Idempotency-Key header. Matches M2's existing bead-keyed reservation.
- Q: What should worktree Push target? → A: **`muster/<beadID>` → `origin`**, with the **remote name configurable** via flag/config (default `origin`). Reuses the established per-bead branch convention; no per-bead remote state.
- Q: How should new M4 WebSocket event types be namespaced? → A: **New dotted namespaces** following the existing convention — `dispatch.*` (queued/admitted), `step.*` (advanced/loopedback), `worktree.*` (finalized/pushed/removed), `run.quota`. Additive only; no existing event type changes.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Capacity-gated dispatch scheduler (Priority: P1)

An operator dispatches several beads in quick succession. Instead of every dispatch immediately spawning an agent (unbounded process/host load), the dispatcher admits only up to a configured number of **concurrent active runs**; further dispatches are held and admitted automatically as running slots free up. The operator can see, per bead, whether its run is *active* or *waiting for a slot*.

**Why this priority**: This is the headline of the milestone — the roadmap names it first ("Real scheduler, capacity gating"). It is the difference between a demo command and a server that can be pointed at a backlog without melting the host. It is independently testable: dispatch N+k beads against a capacity of N and assert exactly N run concurrently while k wait, then that waiters start as actives finish.

**Independent Test**: Set capacity to a small N, dispatch N+2 beads, assert exactly N agents are launched and 2 are queued/admitted-later; on completion of an active run, assert the next waiter transitions to active without another client call.

**Acceptance Scenarios**:

1. **Given** capacity N and N runs already active, **When** an operator dispatches one more bead, **Then** the new dispatch is accepted and reported as *waiting* (queued FIFO, not rejected and not launched), and is admitted automatically when a slot frees.
2. **Given** a waiting dispatch and an active run that finishes, **When** a slot frees, **Then** the waiting bead's agent is launched automatically and its state becomes active.
3. **Given** capacity N, **When** the operator queries dispatcher status, **Then** the response reports current active count, capacity, and the set of waiting beads.
4. **Given** the capacity limit is configured at startup, **When** no explicit limit is provided, **Then** the server uses a documented default and never runs unbounded.

---

### User Story 2 - Finalize & push a completed worktree (Priority: P1)

After an agent has produced changes in a bead's worktree (visible via the M3 diff endpoints), the operator finalizes the work: the dispatcher commits the worktree's changes with a message and, on request, pushes the resulting branch to the source repo's remote. This turns the agent's work product from a transient on-disk diff into a durable, reviewable commit/branch — the write-side the M3 interface reserved.

**Why this priority**: Without a write-side, the agent's output is a black box that never leaves the worktree — the loop from "agent worked" to "reviewable change" is broken. M3 explicitly deferred this to M4 and left the interface stubbed. It is independently testable against real git and jj: create a worktree, mutate a file, finalize, assert a commit exists with the given message; push, assert the branch reaches a bare remote.

**Independent Test**: In a temp repo, dispatch (or seed) a worktree, write a file, call finalize with a message, and assert `git log`/`jj log` in the worktree shows the commit; call push and assert the branch exists in a bare upstream. Both git and jj backends pass; the tests skip when the binary is absent (mirroring the M3 pattern).

**Acceptance Scenarios**:

1. **Given** a bead worktree with uncommitted changes, **When** the operator finalizes it with a commit message, **Then** the changes are committed in that worktree and the previously-`ErrNotImplemented` path now succeeds.
2. **Given** a finalized worktree, **When** the operator requests a push, **Then** the bead's branch is pushed to the source repo's configured remote and the operation reports success/failure explicitly (no silent fallback).
3. **Given** a worktree that is no longer needed, **When** the operator removes it, **Then** the worktree is torn down cleanly (working tree removed, VCS metadata pruned) and a subsequent status reports it absent.
4. **Given** a finalize/push/remove request for a backend whose VCS binary is unavailable, **When** the operation is attempted, **Then** it fails with the existing `VCS_UNAVAILABLE` contract rather than a partial or silent result.
5. **Given** finalize is requested on a worktree with **no** changes, **When** it runs, **Then** the outcome is reported unambiguously (an empty-commit vs. no-op decision that is documented and testable, not an accidental error).

---

### User Story 3 - Multi-step plan→build→review chain with loop-back (Priority: P2)

A bead is dispatched not as a single opaque agent run but as an ordered chain of steps — e.g. **plan** (index 0), **build** (index 1), **review** (index 2). Each step is its own agent invocation over the same worktree; the bead carries a **step pointer** to the current step. Existing per-step endpoints (`/steps/{idx}/attach`, `/steps/{idx}/send`) accept `idx > 0`, not only `0`. A step can **loop back** to an earlier step (e.g. review sends work back to build) so the chain can iterate before the bead is considered done.

**Why this priority**: This is the structural leap from "run one agent" to "run a workflow," and it is what makes the finalize/review write-side meaningful (review is a step). It is lower than US1/US2 because the product is already useful with single-step dispatch + capacity + finalize; multi-step is additive on top. Independently testable: drive a bead through steps 0→1→2 and a 2→1 loop-back, asserting the step pointer and per-step endpoints behave.

**Independent Test**: Dispatch a bead configured with a multi-step chain, advance through each step, attach/send to `idx=1` and `idx=2` (previously rejected), trigger a loop-back from a later step to an earlier one, and assert the step pointer, run state, and events reflect each transition.

**Acceptance Scenarios**:

1. **Given** a bead dispatched with a plan→build→review chain and step 0 complete, **When** an operator issues the advance call, **Then** the step pointer advances to step 1 and the correct agent invocation runs for that step. (Advancement and loop-back are operator-driven in M4; an automatic policy engine is M8.)
2. **Given** the chain is on step 2 (review), **When** an operator attaches to `/steps/2/attach`, **Then** the request is accepted (M2 rejected any `idx != 0`).
3. **Given** the chain is on a review step that determines more work is needed, **When** a loop-back to the build step occurs, **Then** the step pointer moves back to that earlier index and a new run for it is startable.
4. **Given** a bead with a multi-step chain, **When** its step pointer and states are queried, **Then** the response reports the current index, the chain length, and each step's state — while all M2 single-step (`idx=0`) behavior remains unchanged.

---

### User Story 4 - Idempotent, retry-safe dispatch (Priority: P2)

A client (UI, script, or a network retry) issues `POST /dispatch` for the same bead more than once. The dispatcher does not launch duplicate agents or corrupt run state: a repeated dispatch for a bead that is already active/waiting returns the **existing** run rather than starting a second one, and the response distinguishes "started a new run" from "already in progress."

**Why this priority**: M2 already rejects a concurrent duplicate with `ErrRunAlreadyActive`, which is safe but blunt (a retry looks like an error). M4 makes retries *idempotent* — the same dispatch is safe to repeat and returns a consistent result — which is what the roadmap means by "idempotency on `/dispatch`." Independently testable by firing the same dispatch twice (sequentially and concurrently) and asserting exactly one run exists and both calls return a coherent, matching result.

**Independent Test**: Issue two identical dispatch requests for one bead (both sequential and racing); assert exactly one agent/session is created, both responses reference the same run, and the second is clearly marked as a no-op-join rather than a hard error.

**Acceptance Scenarios**:

1. **Given** a bead with an active run, **When** an identical dispatch is issued again, **Then** the response returns the existing run and no second agent is launched.
2. **Given** two identical dispatch requests racing concurrently, **When** both are processed, **Then** exactly one run is created and both callers observe the same run (no leaked session/goroutine).
3. **Given** a completed (no longer active) bead, **When** a fresh dispatch is issued, **Then** a new run is allowed to start (idempotency covers in-flight duplicates, not a permanent lock).

---

### User Story 5 - Quota/cost tracking from agent CLI output (Priority: P3)

When an agent run finishes (or as it streams), the dispatcher captures the agent's own reported cost/usage (tokens, cost, or quota) from the Claude CLI output and surfaces it per run. The `QuotaSource` metadata that has been a no-op (`QuotaNone`) becomes real for the claude adapter, and the captured figures are exposed on run status and/or as an event.

**Why this priority**: Useful for operators budgeting agent spend, but the dispatcher is fully functional without it, and it depends on a specific CLI output format that must be captured without breaking the interactive TUI transport. Lowest priority and structured to be droppable if the CLI output contract proves unstable. Independently testable with a faked claude that emits a known cost payload; assert the parsed figures appear on the run.

**Independent Test**: Run a faked `claude` (argv-recording script on `$PATH`) that emits a known usage/cost payload; assert the dispatcher parses it, the run's quota fields are populated, and `QuotaSource` for the claude adapter is no longer `QuotaNone`.

**Acceptance Scenarios**:

1. **Given** an agent run that reports cost/usage in its output, **When** the run completes, **Then** the parsed cost/usage is attached to that run's status.
2. **Given** the claude adapter, **When** its quota source is queried, **Then** it reports a real source (not `QuotaNone`).
3. **Given** an agent run whose output contains no parseable usage, **When** it completes, **Then** quota is reported as *unknown* (best-effort/advisory) without failing the run.

---

### Edge Cases

- **Capacity = 0 or misconfigured**: a non-positive or unparseable capacity is rejected at startup (fail fast) rather than silently disabling gating or running unbounded.
- **Waiter abandoned**: a bead waiting for a slot is dispatched-cancelled (or its process shut down) before admission — it must leave the queue and free nothing it never held.
- **Finalize race with a live agent**: finalize/remove requested while the step's agent is still writing — the operation must not corrupt the worktree or race the agent (consistent with M3's non-mutating-read rule).
- **Push with no remote / auth failure**: push to a source repo lacking a remote or lacking credentials fails explicitly with a clear error, never a silent success.
- **Loop-back past step 0** or **advance past the last step**: out-of-range step transitions are rejected, not clamped silently.
- **Duplicate dispatch after crash/recovery**: after server restart with recovered sessions, a re-dispatch must still be idempotent against the recovered run.
- **Quota parse on partial/garbled output**: malformed usage output yields *unknown*, never a crash or a wrong number.
- **Server shutdown with waiters queued**: queued (never-started) dispatches are drained/reported on shutdown, not silently lost in a way that misleads the operator.

## Requirements *(mandatory)*

### Functional Requirements

**Scheduler & capacity (US1)**

- **FR-001**: The dispatcher MUST enforce a configurable maximum number of concurrently *active* runs (capacity), set at startup via a flag/env with a documented default of **4**, and MUST never launch more than that many agents at once.
- **FR-002**: When at capacity, a new dispatch MUST be **accepted and queued FIFO** as a *waiting* run — never silently dropped and never launched over the limit.
- **FR-003**: When an active run ends (success, failure, or termination), the dispatcher MUST admit the next waiting dispatch in FIFO order and launch its agent **without requiring a new client call**.
- **FR-004**: The dispatcher MUST expose status reporting current active count, capacity, and the ordered set of waiting beads, sufficient to test FR-001–FR-003.
- **FR-005**: A non-positive or unparseable capacity configuration MUST fail fast at startup (no silent default-to-unbounded), consistent with "no silent defaults for user-controlled behavior."
- **FR-005a**: The capacity limit MUST be **runtime-configurable** via an additive endpoint (so a UI can raise/lower it live); a runtime change to a value below the current active count MUST NOT kill running agents — it stops admitting new ones until actives drain below the new limit. A non-positive runtime value MUST be rejected with a typed error (same fail-fast posture as FR-005).

**Worktree write-side (US2)**

- **FR-006**: `wt.Backend.Finalize` MUST commit the bead worktree's current changes with the supplied message, for both the git and jj backends, replacing the M3 `ErrNotImplemented` sentinel.
- **FR-007**: `wt.Backend.Push` MUST push the bead's branch `muster/<beadID>` to the configured remote (default `origin`, name overridable via flag/config) and MUST report success/failure explicitly (no silent fallback). A missing remote or auth failure is an explicit error, never a silent success.
- **FR-008**: `wt.Backend.Remove` MUST tear down the bead worktree (working tree + VCS bookkeeping) such that a subsequent `Status` reports it absent.
- **FR-009**: Finalize/Push/Remove MUST honor the existing `VCS_UNAVAILABLE` contract when the chosen backend's binary is unavailable.
- **FR-010**: Finalize on a **no-change** worktree MUST be a **successful no-op** — it creates no commit and reports "nothing to commit" success (idempotent and retry-friendly), never an error and never an empty commit.
- **FR-011**: The write-side MUST be reachable through an additive REST surface (new route(s)) so an operator/UI can finalize/push/remove a bead's worktree; handlers stay thin (validate → service → render).

**Multi-step chain (US3)**

- **FR-012**: A bead MUST be able to carry an ordered chain of steps (e.g. plan→build→review) with a **step pointer** indicating the current step. The chain comes from a **configured default** applied when a dispatch omits one, and a `POST /dispatch` request MAY **override** it with an explicit ordered step list. A dispatch with no chain configured or supplied MUST run as a single implicit step (index 0), behaving exactly as in M2.
- **FR-012a**: Each step MUST carry its own **invocation profile** — a permission mode and prompt — so different steps run with different agent behavior (e.g. plan→read-only 'plan' mode, build→editing mode, review→read-only). This reuses the existing per-`Mode` permission-mode plumbing; muster never silently defaults a step's permission mode (Constitution — no silent defaults for user-controlled behavior).
- **FR-013**: The step-scoped endpoints (`/steps/{idx}/attach`, `/steps/{idx}/send`, and any status) MUST accept `idx > 0` (M2 accepted only `0`), while continuing to reject out-of-range indices with a clear error.
- **FR-014**: The chain MUST support **loop-back** to an earlier step; advancement and loop-back are **operator-driven** in M4 — an explicit API call moves the step pointer forward or back. No automatic/agent-driven progression is in scope (that policy engine is M8).
- **FR-015**: Step-pointer and per-step state MUST be observable (query returns current index, chain length, per-step state) and MUST emit additive `step.*` WS events (`step.advanced`, `step.loopedback`) for transitions — new dotted event types, no change to existing event shapes.
- **FR-016**: Because beads has no step-pointer column (same constraint M2/M3 hit for `review`/`vcs`), muster MUST hold step-chain progression as **run/orchestrator state**, not as new authoritative durable issue state — beads stays the source of truth (Constitution II). [NEEDS CLARIFICATION resolved in planning: confirm chain progression is in-memory run state, consistent with M2's `runs` map, and that any bead-visible status still flows through `bd`.]

**Idempotent dispatch (US4)**

- **FR-017**: `POST /dispatch` MUST be idempotent for an in-flight bead **keyed on bead identity + in-flight state**: a repeated dispatch for a bead that already has an active-or-waiting run MUST return that existing run rather than launching a second agent. No separate Idempotency-Key header is introduced.
- **FR-018**: The dispatch response MUST distinguish "a new run was started" from "an existing run was joined," so a retry is not surfaced to the client as a hard failure.
- **FR-019**: Idempotency MUST hold under concurrency (racing identical dispatches yield exactly one run) and after crash-recovery of sessions (a re-dispatch against a recovered run is still idempotent).
- **FR-020**: Idempotency MUST NOT permanently lock a bead — once a run is no longer active, a fresh dispatch starts a new run.

**Quota/cost tracking (US5)**

- **FR-021**: The claude adapter's `QuotaSource` MUST report a real source (not `QuotaNone`) reflecting where cost/usage is read from the CLI output.
- **FR-022**: The dispatcher MUST read the agent's reported cost/usage from **Claude Code's on-disk per-session usage record** (not by parsing the redrawing ANSI TUI stream) after the run, and attach it to the corresponding run, on a **best-effort/advisory** basis. A plan-phase spike MUST pin the exact on-disk path/format before implementation.
- **FR-023**: Missing or unparseable usage output MUST yield an *unknown* quota result — never a run failure or a fabricated figure.
- **FR-024**: Captured quota MUST be exposed on run status and/or an additive `run.quota` WS event, without altering existing status/event shapes.
- **FR-024a**: New WS event types MUST use the established dotted-namespace convention — `dispatch.*` (queued/admitted), `step.*` (advanced/loopedback), `worktree.*` (finalized/pushed/removed), `run.quota` — and MUST be purely additive to the M0–M3 event set.

**Cross-cutting (Constitution)**

- **FR-025**: All new REST routes, DTO fields, and WS event types MUST be additive; no M0–M3 route/shape/event is changed or removed, and the M0–M3 test suites MUST stay green.
- **FR-026**: All issue-state mutations that reach beads MUST go through the `bd` CLI (`store/bdshell`); muster introduces no direct-to-Dolt writes and no new authoritative durable issue state.
- **FR-027**: Every new capability MUST be delivered test-first with per-layer coverage and MUST pass `go test -race ./...`; new external-CLI behavior (git/jj push, claude quota output) gets a fake-on-`$PATH` unit test **and** a skip-gated real-binary integration test.
- **FR-028**: Handlers MUST remain thin (validate input → call a service → render); scheduler/finalize/step/quota logic lives in the service/orchestrator layer, not in `internal/api`.

### Key Entities

- **Dispatcher/Scheduler**: the component that admits dispatches under a capacity bound and tracks active vs. waiting runs. Attributes: capacity, current active set, waiting set (order matters if queueing).
- **Run**: an in-flight (or terminal) execution of a bead's current step. Extends the existing M2 `Run` with: waiting/active distinction, step index, and optional captured quota. No new authoritative issue state — this is orchestrator state.
- **Step / Step chain**: an ordered list of step definitions for a bead (e.g. plan, build, review), each mapping to an agent invocation profile; the chain has a current **step pointer** and supports loop-back. Chain progression is run/orchestrator state, not a beads column.
- **Worktree write operation**: finalize (commit), push (to remote), remove (teardown) over the existing `wt.Backend`, per VCS backend (git, jj).
- **Quota/usage record**: best-effort cost/token/usage figures parsed from the agent CLI output, attached to a Run; may be *unknown*.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With capacity configured to N, dispatching N+k beads results in exactly N agents running concurrently and k handled deterministically (queued-and-later-admitted or cleanly rejected) — never N+1 processes.
- **SC-002**: A waiting (FIFO-queued) dispatch is admitted and its agent launched automatically within a short interval of an active run finishing, with no additional client request.
- **SC-002a**: An operator can raise or lower the capacity limit at runtime through an additive endpoint; lowering it below the active count stops new admissions without killing running agents, and raising it admits waiting runs up to the new limit.
- **SC-003**: An operator can finalize a bead worktree's changes into a commit and push the branch to a remote, and observe the commit/branch in the underlying VCS — for both git and jj — through additive endpoints.
- **SC-004**: A bead can be driven through at least a 3-step chain (plan→build→review) including one loop-back, with the step pointer and per-step endpoints (`idx>0`) behaving correctly, while all M2 single-step behavior is unchanged.
- **SC-005**: Issuing the same dispatch twice (sequentially or concurrently) creates exactly one run; the duplicate returns the existing run rather than an error, and no session/goroutine is leaked.
- **SC-006**: For a run whose agent reports usage, the parsed cost/usage appears on the run's status; for a run with no parseable usage, quota is reported as *unknown* and the run still succeeds.
- **SC-007**: `go test -race ./...` passes for the whole repository, the M0–M3 suites remain green, and per-package coverage gates (including any new M4 package added to the Makefile `thresholds` map) hold.
- **SC-008**: No existing M0–M3 REST route, response shape, or WS event type is changed or removed (verified by the prior-milestone contract/suite tests).

## Assumptions

- **Capacity is a global setting** (startup flag + env, default **4**) and is **runtime-mutable** via an additive endpoint so a UI can change it live; it is not a per-bead or per-repo setting — mirroring how M3 made VCS a global default because beads has no per-bead column. A per-repo/per-bead capacity is out of scope.
- **Step chains are configured, not persisted in beads**: a **default chain** comes from configuration and a `POST /dispatch` request may override it; each step carries its own permission-mode + prompt profile. The live step pointer is orchestrator run state — beads gains no new column (consistent with M2's `review` and M3's `vcs` workarounds).
- **Quota comes from Claude Code's on-disk session usage record**, read after the run — not by scraping the redrawing ANSI TUI stream. It is **advisory only** — never blocks or fails a run, and the interactive TUI transport is untouched. A plan-phase spike pins the on-disk path/format against the real `claude`; if that record proves unavailable/unstable, US5 can be dropped without affecting US1–US4 (the milestone still ships a complete scheduler + write-side + chain + idempotency).
- **Push targets remote `origin` by default** (name overridable via flag/config) and pushes the existing per-bead branch `muster/<beadID>`; muster does not create or authenticate remotes and does not store credentials (consistent with M2's detect-only auth posture).
- **Idempotency is keyed on bead identity + in-flight state** — no Idempotency-Key header; this extends M2's bead-keyed run reservation rather than adding a key store.
- **New WS events follow the dotted-namespace convention** (`dispatch.*`, `step.*`, `worktree.*`, `run.quota`), purely additive to `bead.*`/`tmux.session.*`/`runlog.line`.
- **Finalize/push/remove operate on the per-bead worktree created by the existing `wt.Backend.Create`**; no change to how worktrees are created or where they live.
- **Recovery interplay**: the M2 session-recovery path continues to work; M4's idempotency and scheduler state reconcile with recovered runs rather than replacing the recovery mechanism.
- **`git` present at run time; `jj` optional and probed** — same as M2/M3; the write-side integration tests skip when the binary is absent.
- **The two scope forks are now resolved** (see Clarifications, Session 2026-07-04): capacity uses a **FIFO queue** with auto-admission, and step advancement/loop-back is **operator-driven** in M4 with the automatic policy engine deferred to M8.

## Out of Scope (deferred to later milestones)

- **Multi-provider adapters** (Gemini/Codex/OpenCode/direct-API) — **M5**. M4 makes quota real for the claude adapter only.
- **Skills, constitution merge, prompt assembly, memories CRUD** — **M6**.
- **`/repos` CRUD, probe, hot-reload, multi-`.beads` aggregation** — **M7**. M4 keeps the M2 prefix→repo map.
- **Auto-split, escalation, gates, and an automatic loop/policy engine** — **M8**. M4 delivers the step-chain *mechanism*; the policy that decides splits/escalation/automatic loop-backs is M8. (If US3 is clarified to "operator-driven," the automatic policy is entirely M8.)
- **Worktree garbage collection** (`muster gc`, idle cleanup), `/metrics`, runlog compaction, audit log — **M9**. M4 adds `Remove` as an on-demand write-op, not scheduled GC.
- **UI wiring** of scheduler/finalize/step/quota into the embedded prototype UI — tracked with the M7 UI work.
- **Persisting a per-bead `vcs`/step/capacity field in beads** — requires a beads-model change; deferred with the field.

## Dependencies & Roadmap Position

- **Builds on M2**: the orchestrator, `Dispatch`, `DispatchRequest`, the `runs` map + reservation (`ErrRunAlreadyActive`), tmux transport, and the `/steps/{idx}` endpoints all exist and are green.
- **Builds on M3**: the `wt.Backend` interface already declares `Finalize`/`Push`/`Remove` (returning `ErrNotImplemented`) and the git+jj backends + diff endpoints exist — M4 fills the write-side without changing the interface contract.
- **Unblocks**: M5 (more adapters over the same dispatcher + quota interface), M8 (policy/auto-split/loop over the step-chain mechanism), and M9 (GC over `Remove`, metrics over scheduler status).
