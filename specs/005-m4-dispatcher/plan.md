# Implementation Plan: M4 — Dispatcher

**Branch**: `claude/optimistic-dijkstra-76a000` (spec dir `005-m4-dispatcher`; work on the current branch) | **Date**: 2026-07-04 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/005-m4-dispatcher/spec.md` (fully clarified — 9 clarifications, 3 rounds)

## Summary

Turn M2's single-shot `Dispatch` into a managed **dispatcher**: (1) a **capacity-gated FIFO scheduler** inside the orchestrator that admits at most N concurrent active runs (default 4, runtime-mutable via an additive endpoint) and auto-admits the next waiter when a slot frees; (2) the **worktree write-side** — `wt.Backend.Finalize`/`Push`/`Remove` filled in for both git and jj, replacing the M3 `ErrNotImplemented` sentinels, with no-op-on-no-change finalize and `muster/<beadID> → origin` push; (3) a **multi-step plan→build→review chain** with a per-bead step pointer, per-step invocation profiles (permission mode + prompt), and **operator-driven** advance/loop-back (`/steps/{idx}` now accepts `idx>0`); (4) **idempotent dispatch** keyed on bead identity + in-flight state (a repeat joins the existing run); and (5) **best-effort quota** read from Claude Code's on-disk per-session usage record (spike-pinned) and surfaced on run status + a `run.quota` event. All new REST routes and WS event types are additive (`dispatch.*`, `step.*`, `worktree.*`, `run.quota`); the sole intentional contract change is the duplicate-dispatch status (409 → 200+join), taken as an explicit Constitution-V migration (Complexity Tracking).

## Technical Context

**Language/Version**: Go (same toolchain as M0–M3; `go.mod` unchanged — no new modules)

**Primary Dependencies**: stdlib only (`os/exec`, `net/http`, `sync`, `encoding/json`, `path/filepath`); external runtime tools shelled out: `git` (required), `jj` ≥ 0.42 (optional), `tmux`, `claude` (quota record read from its on-disk session file)

**Storage**: none of its own (Constitution I) — scheduler state (active set, FIFO waiting queue, per-bead step pointer, captured quota) is **in-memory orchestrator run state**, reconstructable and disposable; worktrees live under `--worktrees-dir` as in M2/M3; beads stays the source of truth

**Testing**: `go test` with fakes-on-`$PATH` (fake `git`/`jj`/`claude`, argv-recording) + real-binary integration tests gated on tool presence (skip when absent); `go test -race ./...` clean — the scheduler and admission-on-completion paths are the highest-risk concurrency surface and are `-race` tested explicitly

**Target Platform**: local loopback server (Constitution: local-first), macOS/Linux

**Project Type**: single Go project (CLI + embedded HTTP/WS server) — extends the M0–M3 layout

**Performance Goals**: no new hot path; scheduler admission is O(1) amortized on run completion; quota read is one post-run file read; diff/finalize stream where already streaming

**Constraints**: additive REST/WS surface (Constitution V) except the one documented duplicate-dispatch migration; no credential handling (push uses the repo's existing remote/auth, FR-007); no silent defaults (capacity fail-fast, per-step permission mode never defaulted); write-side must not race a live agent (finalize/remove guard on run state)

**Scale/Scope**: ~1 new internal sub-component (`orchestrator` scheduler) + write-side fill-in of `internal/wt` + step-chain model; ~7 new endpoints; additive status/DTO fields; 1 new config flag; 4 new WS event families

## Constitution Check

*GATE: evaluated before Phase 0 and re-checked after design. One justified migration (see Complexity Tracking); all else PASS.*

| Principle | Assessment |
|---|---|
| **I. Single Binary, Self-Contained** | ✅ No new Go modules. `git`/`jj`/`tmux`/`claude` remain shelled-out runtime deps. Quota is read from `claude`'s own on-disk session file — no library, no embedded store. |
| **II. Beads Is Source of Truth** | ✅ Scheduler queue, step pointer, and quota are in-memory, reconstructable run state — **not** new authoritative durable issue state. Any bead-visible status still flows through `bd` (`store/bdshell`). No direct-to-Dolt writes. Step chains are config/request-supplied, not a new beads column (same posture as M2 `review` / M3 `vcs`). |
| **III. Layered Architecture** | ✅ Scheduler lives inside the `orchestrator` package behind its existing service-facing interfaces; finalize/push/remove live behind the `wt.Backend` interface. Handlers stay thin (parse → service → render). No business logic in `internal/api`. |
| **IV. Test-First, Per-Layer Coverage** | ✅ TDD: failing tests first, per layer. Fakes-on-`$PATH` for git-push/jj/claude-quota **plus** skip-gated real-binary integration tests. `-race` clean, with dedicated concurrency tests for admission and idempotency. Per-package gates below. |
| **V. Additive, Backward-Compatible Surface** | ⚠️ **One intentional migration**: duplicate dispatch changes from **409 Conflict** (M2 `ErrRunAlreadyActive`) to **200 + existing-run + `joined:true`** to deliver idempotency (FR-017/FR-018). Everything else is strictly additive (new routes, new `dispatch.*`/`step.*`/`worktree.*`/`run.quota` events, additive status fields). Justified in Complexity Tracking; the two M2 contract tests are migrated, not silently broken. |
| **Security & Orchestration** | ✅ No credentials (push reuses the repo's configured remote/auth; muster stores nothing). Per-step permission mode is user-supplied, never silently defaulted (FR-012a). Isolation preserved (write-side operates only on the per-bead worktree). Adapter-agnostic (quota via the `QuotaSource` interface, claude-specific reader behind it). Local-first. |

## Project Structure

### Documentation (this feature)

```text
specs/005-m4-dispatcher/
├── spec.md              # ✅ clarified (9 clarifications)
├── plan.md              # ← this file
├── research.md          # Phase 0 — scheduler/idempotency/write-side/quota decisions + quota spike
├── data-model.md        # types: Scheduler, Run (extended), StepChain/Step/StepProfile, DispatchResult, QuotaUsage
├── quickstart.md        # end-to-end: capacity queue → multi-step run → finalize/push → quota
├── contracts/
│   ├── http-endpoints.md    # new routes: capacity, finalize/push/remove, step advance/loopback (+ idx>0)
│   ├── wt-writeside.md       # Finalize/Push/Remove behavioral contract (git + jj)
│   └── ws-events.md          # dispatch.* / step.* / worktree.* / run.quota envelopes
├── checklists/          # requirements.md (from specify)
└── tasks.md             # Phase 5 (speckit-tasks)
```

### Source Code Layout (extends M0–M3)

```text
internal/
├── orchestrator/
│   ├── orchestrator.go     # Dispatch: consult scheduler; admit-or-enqueue; idempotent join.
│   │                       #   On run end, admit next FIFO waiter (in the watcher-completion path).
│   ├── scheduler.go        # NEW — capacity semaphore + FIFO waiting queue; SetCapacity (runtime,
│   │                       #   drain-not-kill); Snapshot(active,capacity,waiting) for status. -race safe.
│   ├── scheduler_test.go   # NEW — admission bound, FIFO order, auto-admit on completion, runtime resize,
│   │                       #   fail-fast on non-positive; concurrency/-race tests.
│   ├── steps.go            # NEW — StepChain/StepProfile, per-bead step pointer, Advance/LoopBack
│   │                       #   (operator-driven, range-checked); default chain + per-dispatch override.
│   ├── steps_test.go       # NEW — advance/loopback bounds, single-step (M2) default preserved.
│   ├── quota.go            # NEW — read claude on-disk session usage (path/format from spike); best-effort,
│   │                       #   → QuotaUsage{unknown} on missing/garbled. Behind adapter.QuotaSource.
│   ├── quota_test.go       # NEW — fake session file parse + unknown fallback; real-claude skip-gated.
│   ├── recovery.go         # reconcile recovered runs with scheduler active set + idempotency.
│   └── service_adapter.go  # map new sentinels (ErrStepOutOfRange, write-side errs); dup→joined result.
├── wt/
│   ├── git.go              # Finalize: `git -C add -A && commit -m` (no-op if `status --porcelain` empty);
│   │                       #   Push: `git push <remote> muster/<beadID>`; Remove: `git worktree remove`.
│   ├── jj.go               # Finalize: `jj describe -m` on the working revision (jj auto-snapshots); Push:
│   │                       #   `jj git push`/`git push` on the colocated remote; Remove: workspace forget + rm.
│   ├── writeside_test.go   # NEW — fake + real git/jj: commit exists, empty=no-op, push reaches bare remote,
│   │                       #   remove→Status absent, VCS_UNAVAILABLE honored. Skip-gated on binaries.
│   └── remote.go           # NEW — remote-name resolution (default origin, configurable), branch = muster/<id>.
├── services/
│   └── beads.go            # NEW methods: FinalizeWorktree/PushWorktree/RemoveWorktree; AdvanceStep/LoopBackStep;
│                           #   SetCapacity; DispatchResult carries joined flag. Thin delegation to orchestrator.
├── api/
│   ├── router.go           # + POST /beads/{id}/worktree/finalize, POST .../worktree/push,
│   │                       #   DELETE /beads/{id}/worktree, POST /beads/{id}/steps/advance,
│   │                       #   POST /beads/{id}/steps/loopback, PUT /orchestrator/capacity.
│   ├── beads/handlers.go   # thin handlers for the above; parseStepIdx now accepts idx>0 (was "0" only);
│   │                       #   Dispatch returns 200+joined on in-flight duplicate (was 409).
│   └── health/             # OrchestratorStatus DTO: + capacity, activeCount, waiting[]; per-run: + stepIdx,
│                           #   chainLen, quota (additive fields).
├── ws/
│   └── event.go            # + EventDispatchQueued/Admitted, EventStepAdvanced/LoopedBack,
│                           #   EventWorktreeFinalized/Pushed/Removed, EventRunQuota (additive EventType consts).
├── adapter/
│   ├── adapter.go          # (unchanged interface) QuotaSource already declared.
│   └── claude/claude.go    # QuotaSource() returns QuotaCLIOutput (was QuotaNone); on-disk reader wired in orch.
└── config/
    └── dispatcher.go       # NEW — ParseMaxConcurrent (--max-concurrent-dispatches / MUSTER_MAX_CONCURRENT_DISPATCHES,
                            #   default 4, fail-fast on ≤0).
cmd/muster/main.go          # + --max-concurrent-dispatches flag; construct scheduler with capacity; wire remote name.
```

**Structure Decision**: **Extend in place, isolate the new engine.** The scheduler, step-chain, and quota reader are new files **inside** the existing `orchestrator` package (they share the `runs` map, `mu`, and watcher-completion path — splitting them into a separate package would force exporting that internal state and invite the very TOCTOU the M2 reservation closed). The write-side fills the **existing** `wt.Backend` methods, so no caller changes (M3 designed for exactly this). No prior-milestone package is refactored; `internal/worktree`, the M3 read endpoints, and the M2 transport are untouched. This keeps the highest-risk concurrency surface (admission on completion) co-located with the state it guards, behind the orchestrator's existing service-facing interfaces (Constitution III).

### Per-Package Coverage Gates (added to Makefile `thresholds`)

| Package | Gate | Rationale |
|---|---|---|
| `internal/orchestrator` | maintain existing gate (no regression) | scheduler/steps/quota land here; -race concurrency tests |
| `internal/wt` | maintain existing gate | write-side fills existing methods; fake + real git/jj |
| `internal/config` | maintain existing gate | one new parser (ParseMaxConcurrent) mirrors ParseDefaultVCS |
| `internal/services`, `internal/api/beads`, `internal/ws` | maintain existing gates | thin additive delegation/handlers/events |

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| **Duplicate-dispatch contract change (409 → 200 + `joined:true`)** — the single non-additive surface change (Constitution V), migrating M2 tests `TestDispatch_409_RunAlreadyActive` / `TestDispatch_409_DuplicateRun`. | FR-017/FR-018 require an idempotent, retry-safe `/dispatch` that **returns the existing run** rather than surfacing a retry as an error; the clarified decision (Session 2026-07-04) chose bead-identity idempotency with no key header. A 409 makes a benign retry look like a failure. | **Keep 409, enrich body** was considered (fully additive, zero migration) but rejected: a 409 is still an error status, contradicting the clarified "return the existing run, not an error," and would leave clients special-casing an error path for a successful join. The change is small, mechanical, and versioned (the two M2 tests are rewritten to assert the new idempotent contract, not deleted) — exactly the "explicit, versioned migration decision" Constitution V permits. |

*All other principles: no violations (no new modules, no heavyweight deps, new engine behind existing interfaces, additive surface).*

## Phase 0 → research.md

Resolves: scheduler admission/queue design, the idempotency migration shape, git+jj write-side command contracts (incl. no-op-empty finalize and push remote), the step-chain/step-pointer model, and the **quota on-disk spike** (pin Claude Code's per-session usage file path/format against the real binary before implementing FR-022). See [research.md](research.md).

## Phase 1 → design artifacts

- [data-model.md](data-model.md) — Scheduler, extended Run (Waiting/StepIdx/Quota), StepChain/Step/StepProfile, DispatchResult, QuotaUsage, config knobs.
- [contracts/http-endpoints.md](contracts/http-endpoints.md), [contracts/wt-writeside.md](contracts/wt-writeside.md), [contracts/ws-events.md](contracts/ws-events.md).
- [quickstart.md](quickstart.md) — end-to-end operator walkthrough.
- Agent context: CLAUDE.md's plan pointer updated to this file.
