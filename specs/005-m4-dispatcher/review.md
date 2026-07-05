# M4 Dispatcher — Pre-Implementation Review

**Reviewer role:** independent adversarial pre-implementation review (did not author these artifacts).
**Date:** 2026-07-04
**Scope:** spec.md, plan.md, research.md, data-model.md, contracts/*, tasks.md vs. the actual M2/M3 code on this branch.

## Summary

**Overall verdict: READY-WITH-WARNINGS.**

The milestone is unusually well-specified: 9 recorded clarifications, an honest Complexity-Tracking entry for the one non-additive change (409→200), two SPIKE tasks correctly ordered before their dependents, and a design that reuses existing seams (`Run.StepIdx`, the baked-in `worktreesDir` backend, the watcher-completion hook) rather than inventing new state. The worktreesDir concern raised in the brief is a non-issue: the orchestrator constructs its backend via `NewGitBackend(cfg.WorktreesDir)` (orchestrator.go:201-207), so `Finalize/Push/Remove` inherit `g.worktreesDir` exactly as `Status/Diff` already do.

However there are **two concrete correctness gaps that will surface as build/test failures during implementation**, both centered on the collision between the *multi-step* story (US3) and the *M2 single-step assumptions still hard-coded in the code*, plus **undocumented non-additive test changes** beyond the single declared migration. These need to be reflected in tasks.md before a competent engineer starts, or US3/US4 will stall mid-implementation.

### Verdict table

| Dimension | Verdict | One-line reason |
|---|---|---|
| 1. Spec ↔ plan alignment | PASS | Every FR maps to a plan element; the one contract break is explicitly justified. |
| 2. Plan ↔ tasks completeness | WARN | Recovery-of-StepIdx>0, per-step session naming, prompt-file-per-step, and the WorktreeAccessor widening have no task. |
| 3. Dependency ordering & TDD | PASS | Test-before-impl throughout; both spikes gated before dependents; phase order sound. |
| 4. Parallelization correctness | PASS | The three `[P]` groups touch disjoint files; the US1‖US2 fan-out is cross-package. |
| 5. Feasibility & risk | WARN | Recovery actively **kills** StepIdx>0 sessions today (recovery.go:80) — directly contradicts FR-019/R9; single-`*Run`-per-bead map complicates advance/loopback. |
| 6. Standards / Constitution | WARN | The "single documented migration" claim is violated: T045/T053/T062 each rewrite a *passing* M2/M3 test not listed in Complexity Tracking. |
| 7. Implementation readiness | WARN | T001 is startable, but US3 cannot be implemented as written without resolving the recovery-kill and session-naming gaps. |

---

## Findings

### CRITICAL

#### C1 — Recovery kills StepIdx>0 sessions today; US3 requires the opposite, and no task changes it
**Artifact:** research.md R9 (l.76), spec FR-019 (l.166), tasks.md T053 (l.122) · **Code:** `internal/orchestrator/recovery.go:80-85`, `internal/orchestrator/recovery_test.go:149` (`TestRecoverSessions_UnsupportedIndicesKilled`).

recovery.go currently *reaps* any recovered session whose name encodes `StepIdx != 0 || Loop != 0`:
```go
if sess.StepIdx != 0 || sess.Loop != 0 {
    slog.Warn("recovery: killing session with unsupported step/loop indices (M2 supports only 0/0)", ...)
    _ = o.transport.Kill(sessionName)
    return
}
```
R9 and FR-019 assert the exact opposite: *"Recovered runs carry `StepIdx` from the session name … so the step pointer survives restart."* Once US3 runs a build/review step, its live tmux session is named `muster/<id>/1/…` or `.../2/…`. After a restart, recovery will **kill that in-flight agent** instead of re-registering it. This is a data-loss bug (kills a running agent doing real work) and it silently breaks FR-019's "idempotent after crash-recovery against a recovered *multi-step* run."

T053 says only "reconcile recovered runs into the scheduler active set + idempotency" — it never mentions relaxing the StepIdx>0 kill guard, and it doesn't budget for rewriting `TestRecoverSessions_UnsupportedIndicesKilled`, which asserts the kill and will *fail* the moment the guard is relaxed.

**Fix:** Add explicit task work: (a) relax the recovery guard so a StepIdx within `[0, chainLen)` (Loop still bounded) re-registers rather than kills; keep killing genuinely-malformed/negative indices; (b) rewrite `TestRecoverSessions_UnsupportedIndicesKilled` to assert the new boundary (list it in Complexity Tracking as part of the surface migration — see C3); (c) state how `chainLen` is known at recovery time when the chain lived only in memory (the recovered session name carries `StepIdx` but **not** the chain — so a recovered run has a pointer but no chain to advance within; decide whether recovery reconstructs a single-step run at that index or refuses to advance until re-dispatched).

#### C2 — `runs` is one `*Run` per bead; advance/loopback and the duplicate-check assume a single active run — the model for "new run per step over the same worktree" is unspecified
**Artifact:** data-model.md (l.29 "advancing produces a **new** active run for the next StepIdx"), research.md R7 · **Code:** `orchestrator.go:104` (`runs map[string]*Run`, keyed by beadID), `orchestrator.go:469` (dup check on `runs[beadID].State == StepActive`), `finishRun` (l.737) evicts/rewrites the single entry.

The whole M2 concurrency design keys `o.runs` on `beadID` with exactly one `*Run`. Every invariant depends on it: the TOCTOU reservation (l.468-479), the identity-guarded eviction (l.314-322), the "kill session before flipping State" ordering (l.729-736), and idempotency (US4). US3 says advance "produces a **new** active run for the next StepIdx over the same worktree." tasks.md never says whether that new run **replaces** `runs[beadID]` (losing the prior step's terminal record and its quota) or requires re-keying the map. If it replaces in place, then:
- `finishRun`'s `scheduleRunEviction` identity guard and the reservation-release `defer` (l.486-495) can race an advance that swapped the pointer.
- US4's idempotency "return the existing run" becomes ambiguous: *which* step's run is "the existing run" when a chain is mid-flight?

**Fix:** Specify the run-vs-step lifecycle explicitly in data-model.md before T016/T043: e.g. one live `*Run` per bead at a time (advance = finish current step's run, then start the next step's run under the same beadID key, carrying forward `Chain` and accumulated `Quota`), and state that `finishRun` on a non-terminal chain must **not** evict if an advance is pending. Add a task for the advance/finish interlock and a `-race` test for "advance while the prior step's watcher is finishing."

### HIGH

#### C3 — The "single documented migration" claim is false: at least three passing M2/M3 tests are rewritten, only one is in Complexity Tracking
**Artifact:** plan.md Complexity Tracking (l.126-128, lists only the 409 pair), plan Constitution-V row (l.41 "the two M2 contract tests are migrated"), FR-025/SC-008 ("M0–M3 suites MUST stay green"). · **Code/tests affected:**
- `TestDispatch_409_RunAlreadyActive` (handlers_test.go:489) + `TestDispatch_409_DuplicateRun` (orchestrator_test.go:302) — declared (T048). ✅
- `TestQuotaSource_None` (claude_test.go:232, asserts `QuotaNone`) — **not declared**; T062 flips `QuotaSource()`→`QuotaCLIOutput`, which necessarily rewrites this passing test.
- `TestRecoverSessions_UnsupportedIndicesKilled` (recovery_test.go:149) — **not declared**; broken by C1's fix.
- `parseStepIdx` widening (T045) rewrites the existing handler test asserting `idx != "0"` → 404 (handlers.go:93-98). This is arguably additive (still rejects out-of-range) but the *existing test's expectation for `idx=1`* changes from 404-not-found to accepted/other, which is a behavior change to a shipped surface.

SC-008 is stated as "verified by the prior-milestone contract/suite tests" — but four of those tests are being edited. The plan's own framing ("*the* single non-additive surface change") is inaccurate.

**Fix:** Either (a) expand Complexity Tracking to enumerate every prior-milestone test being rewritten with the justification per item, or (b) re-word SC-008/the Constitution-V row to "no prior REST *route/shape/event* is removed; the following prior-milestone *tests* are intentionally migrated: [list]." The `QuotaSource` flip and the `parseStepIdx` widening are legitimately additive-in-spirit, but they must be named so the "green M0–M3 suite" gate isn't quietly redefined.

#### C4 — Per-step session naming and prompt-file naming are hard-coded to step 0; US3 has no task to generalize them
**Artifact:** research.md R7, tasks.md T043-T044 · **Code:** `orchestrator.go:593` (`tmux.SessionName(req.BeadID, 0, 0)` — literal 0), `orchestrator.go:378` (`promptFileName = ".muster-prompt-0.txt"` — literal 0), `orchestrator.go:648` (`runlogStreamer{stepIdx:0}`), `orchestrator.go:659` (`StepIdx: intPtr(0)`).

`tmux.SessionName(beadID, stepIdx, loop)` exists and takes indices, but Dispatch passes literal `0`. For advance/loopback to "run that step's profile as a fresh agent invocation," each step needs its own session name (else step 1's `Spawn` collides with step 0's still-registered session name and tmux returns a duplicate-session error — the exact failure finishRun's ordering comment warns about at l.729-736). Likewise the prompt file is `.muster-prompt-0.txt` hard-coded; step 1 would overwrite step 0's prompt or read the wrong one. None of T043/T044/T046 mention threading `stepIdx` through `SessionName`, the prompt filename, the runlog streamer, or the `tmux.session.opened` event's `StepIdx`.

**Fix:** Add explicit sub-tasks under US3 to parameterize the launch path by `stepIdx`: session name = `SessionName(beadID, stepIdx, loop)`, prompt file = `.muster-prompt-<stepIdx>.txt`, `runlogStreamer.stepIdx = stepIdx`, and the opened/closed events' `StepIdx`. This is the bulk of the real US3 work and is currently invisible in the task list (T044 is scoped to "add optional chain override," not to generalize the spawn path).

#### C5 — WorktreeAccessor seam is not widened for the write-side; the service→backend path for Finalize/Push/Remove is missing a task
**Artifact:** plan.md (l.91 services methods, l.113 "write-side fills existing methods, so no caller changes"), tasks.md T037 · **Code:** `internal/orchestrator/worktree_adapter.go:18-45` (`worktreeAccessorAdapter` exposes only Status/DiffSummary/Diff/DefaultVCS), `services.WorktreeAccessor` interface.

The plan says filling `wt.Backend.Finalize/Push/Remove` needs "no caller changes" — true at the `wt.Backend` level, but the **service** doesn't call `wt.Backend` directly; it goes through the `services.WorktreeAccessor` interface, which today has no Finalize/Push/Remove methods (worktree_adapter.go:24-37). T037 adds `FinalizeWorktree/PushWorktree/RemoveWorktree` service methods but there is no task to (a) extend the `WorktreeAccessor` interface, (b) add the three passthrough methods to `worktreeAccessorAdapter`, and (c) update the `var _ services.WorktreeAccessor` compile-time assertion. This will not compile as tasked.

**Fix:** Add a task (before or within T037) to widen `services.WorktreeAccessor` with the three write methods and implement them on `worktreeAccessorAdapter` (thin delegation to `a.o.backend.Finalize/Push/Remove`, which already have `worktreesDir` baked in).

### MEDIUM

#### M1 — Finalize/Remove "must not race a live agent" guard has no defined state to check for a *waiting* or *finishing* run
**Artifact:** contracts/wt-writeside.md Safety (l.44-46), contracts/http-endpoints.md (l.29 "409 if the step's agent is still active"), Edge Cases (spec l.124) · **Code:** `finishRun` flips `State` off `StepActive` only *after* killing the session and closing the pipe (orchestrator.go:759-764).

The guard is specified as "refuse when the step agent is active" → check `run.State == StepActive`. But (a) a *waiting* (StepPending) run has a worktree that may not exist yet — finalize should 404/why?; and (b) there is a window in `finishRun` where the session is already killed but `State` is still `StepActive` (the deliberate ordering at l.729-764) — a finalize arriving there is refused even though the agent is effectively done. Neither the contract nor T037 pins which states permit finalize. This is a correctness/UX edge, not a blocker.

**Fix:** In the write-side contract, enumerate the permitted states for Finalize/Remove (e.g. allow only `StepDone`/`StepFailed`; reject `StepActive`/`StepPending` with `CodeRunAlreadyActive`) and add a test for the finish-window race.

#### M2 — Idempotency vs. capacity: a *waiting* duplicate and an "in-flight parameters differ" case are underspecified
**Artifact:** research.md R4 (l.31 "ErrRunAlreadyActive retained only if still needed for a genuinely conflicting case (different in-flight parameters)"), spec FR-017 · **Code:** current dup check is state-only (`State == StepActive`), it does **not** compare Agent/Mode/PermissionMode.

R4 hand-waves a "genuinely conflicting case (different in-flight parameters)" that would still error — but M2's reservation never stored/compared those params (the reservation `Run` at l.473-477 has only BeadID/State/StartedAt). Deciding to *join* a differing-params dispatch vs. *reject* it is a real semantic choice with no task and no test. Also: the dup check keys on `StepActive` only; a *waiting* (StepPending) run must also be joined (FR-017 says "active-or-waiting"), which requires the check to consider StepPending too — currently it doesn't.

**Fix:** Pin the idempotency comparison in data-model.md/R4: join on bead-identity regardless of params (recommended, matches "no key header"), OR compare and reject — pick one, and update T049 to check both `StepActive` and `StepPending`. Add a test for "duplicate dispatch of a *waiting* bead joins the waiter."

#### M3 — `dispatch.queued` `waitingPos` and Snapshot ordering must be defined to be testable (FR-004/SC-001)
**Artifact:** contracts/ws-events.md (l.7 `waitingPos`), data-model.md Scheduler `Snapshot() (…, waiting []string)`. No finding of severity beyond clarity: FIFO order is asserted in T011/T012 but `waitingPos` semantics (0- or 1-based, stable under cancellation) aren't pinned. Minor.

**Fix:** State `waitingPos` is 0-based index in the FIFO at emit time and that `Snapshot().waiting` preserves FIFO order; one line in the contract.

### LOW

#### L1 — Recovery scheduler-registration can exceed capacity, and the spec acknowledges it but no task bounds behavior
**Artifact:** research.md R9 "Alternatives: Ignore recovered runs in capacity — rejected (would over-admit)." The chosen design registers *all* recovered actives into `active`, which can legitimately push `len(active) > capacity` after a restart if the prior process ran at a higher capacity. data-model.md invariant says `len(active) ≤ capacity` **always** — that invariant is violated by recovery. Not dangerous (drain-not-kill covers it) but the stated invariant is wrong.

**Fix:** Soften the data-model invariant to "≤ capacity for newly admitted runs; recovery may transiently exceed it and drains down," matching R2's drain semantics.

#### L2 — `promptFileName` const and `buildPrompt` produce a single fixed prompt; per-step `PromptRef` (StepProfile) has no assembly path in M4
**Artifact:** data-model.md StepProfile.PromptRef, spec FR-012a. The step profile carries a `PromptRef`, but prompt *assembly* (skills/constitution/memories) is explicitly M6 (spec Out of Scope l.222). So in M4 each step's prompt is just `buildPrompt(title,desc)` regardless of `PromptRef`. That's fine, but tasks/data-model should note `PromptRef` is a carried-but-not-yet-resolved field in M4 to avoid an implementer trying to build the M6 assembly.

**Fix:** One note in data-model.md: "PromptRef is stored but resolves to the M2 bead prompt in M4; real assembly is M6."

---

## Remediation (applied 2026-07-04, post-review)

All findings resolved in the artifacts before implementation:

| Finding | Resolution |
|---|---|
| **C1** (recovery kills StepIdx>0) | T053 rewritten: relax guard to re-register `StepIdx∈[0,chainLen)`, migrate `TestRecoverSessions_UnsupportedIndicesKilled`, recovered run pinned single-step & refuses advance until re-dispatched; documented in research.md R9 correction. |
| **C2** (run-vs-step lifecycle) | data-model.md pins "one live `*Run` per bead": advance = finish then start next step under same key, carry `Chain`+`Quota`; `finishRun` no-evict-with-pending-advance; new task T043b with `-race` interlock test. |
| **C3** (undocumented test migrations) | plan.md Complexity Tracking + Constitution-V row now enumerate all 5 migrated tests (409 pair, `TestQuotaSource_None`, recovery test, parseStepIdx); T062 annotated. |
| **C4** (per-step session/prompt hard-coded to 0) | new task T043a threads `stepIdx` through `SessionName`, `.muster-prompt-<idx>.txt`, `runlogStreamer`, opened/closed events. |
| **C5** (WorktreeAccessor not widened) | new task T036a widens `services.WorktreeAccessor` + `worktreeAccessorAdapter` passthrough + compile assertion, before T037. |
| **M1** (finalize-vs-live-agent states) | wt-writeside.md + T037 pin permitted states = `StepDone`/`StepFailed`; reject `StepActive`/`StepPending`; finish-window race test. |
| **M2** (idempotency waiting run/params) | T049: join on both `StepActive` **and** `StepPending`, bead-identity regardless of params; waiter-join test. |
| **M3** (`waitingPos` semantics) | ws-events.md: 0-based FIFO index; Snapshot preserves FIFO. |
| **L1** (capacity invariant) | data-model.md invariant softened: recovery may transiently exceed capacity, drains down. |
| **L2** (`PromptRef` assembly) | data-model.md + T043 note: `PromptRef` carried but resolves to M2 bead prompt in M4; assembly is M6. |

**Post-remediation verdict: READY.** No CRITICAL/HIGH open. Task count now 62 (added T036a, T043a, T043b).

## What's solid

- **Spike discipline is correct.** T030 (jj write-side) and T059 (claude on-disk quota) are genuinely unknown and are ordered *before* their dependents (T035, T060-063), with a documented drop path if a spike fails — exactly the Constitution's "verify empirically before building." The claude on-disk quota path being unknown is honestly flagged, not assumed.
- **The worktreesDir worry is unfounded.** `Finalize/Push/Remove` take only `beadID`, but the orchestrator's backend is built with `NewGitBackend(cfg.WorktreesDir)` (orchestrator.go:201-207), so they resolve paths from the baked-in `worktreesDir` identically to the already-working `Status/Diff` methods. No new plumbing needed at the backend level.
- **The 409→200 migration reasoning is honest and correct** (Complexity Tracking l.126-128): "keep 409, enrich body" is the right alternative to have considered and the rejection ("a 409 still reads as an error") is sound. `StepPending` reuse for the queue avoids a new core enum, which is a clean call.
- **Scheduler-in-orchestrator co-location** (plan Structure Decision l.113) is the right architecture: putting the queue behind the existing `mu`/`runs` rather than a separate package avoids exporting internal state and re-opening the very TOCTOU the M2 reservation closed. The `-race`-first task ordering (T012, T051) targets the actual highest-risk surface.
- **Parallelization is genuinely safe:** group-1 (T001/T002/T003) and group-2 (T031/T032/T033) touch disjoint files; the US1‖US2 fan-out is `orchestrator` vs `wt`, no shared writes.
