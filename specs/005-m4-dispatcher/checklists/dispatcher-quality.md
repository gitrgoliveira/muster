# Requirements Quality Checklist: M4 — Dispatcher

**Purpose**: Unit-tests-for-English — validate that the M4 requirements are complete, clear, consistent, and measurable before implementation.
**Created**: 2026-07-04
**Feature**: [spec.md](../spec.md) · [plan.md](../plan.md)
**Depth**: Release gate · **Audience**: Reviewer (PR)

## Scheduler & Capacity (US1)

- [ ] CHK001 — Is the maximum-concurrency bound specified as an exact, testable relationship (active ≤ capacity)? [Clarity, Spec §FR-001]
- [ ] CHK002 — Are the queued-run ordering semantics unambiguously specified as FIFO? [Clarity, Spec §FR-002]
- [ ] CHK003 — Is the auto-admission trigger ("an active run ends") defined to include all terminal states — success, failure, cancel, timeout? [Completeness, Spec §FR-003]
- [ ] CHK004 — Is "no additional client call required for admission" stated as a requirement, not just an example? [Completeness, Spec §FR-003]
- [ ] CHK005 — Is the startup fail-fast behavior for a non-positive/unparseable capacity specified with a concrete outcome (no silent default)? [Clarity, Spec §FR-005]
- [ ] CHK006 — Are runtime capacity-change semantics defined for the shrink-below-active case (drain vs. kill)? [Edge Case, Spec §FR-005a]
- [ ] CHK007 — Is the observable scheduler status (capacity, active count, ordered waiting set) specified precisely enough to assert in a test? [Measurability, Spec §FR-004]
- [ ] CHK008 — Are requirements defined for a waiting run that is cancelled/abandoned before admission? [Coverage, Spec §Edge Cases]
- [ ] CHK009 — Is server-shutdown behavior for still-queued dispatches specified (drained/reported, not silently lost)? [Gap, Spec §Edge Cases]

## Worktree Write-Side (US2)

- [ ] CHK010 — Is the no-change Finalize outcome specified as a single unambiguous behavior (no-op success, no commit)? [Clarity, Spec §FR-010]
- [ ] CHK011 — Are Finalize/Push/Remove postconditions defined for both git AND jj backends? [Coverage, Spec §FR-006–FR-008]
- [ ] CHK012 — Is the Push target (branch `muster/<beadID>`, remote default `origin`, configurable) fully specified? [Completeness, Spec §FR-007]
- [ ] CHK013 — Is explicit-error-vs-silent behavior specified for Push failure modes (no remote, auth failure, rejected)? [Clarity, Spec §FR-007, §Edge Cases]
- [ ] CHK014 — Is the VCS_UNAVAILABLE requirement consistently applied to all three write-side operations? [Consistency, Spec §FR-009]
- [ ] CHK015 — Are requirements defined to prevent Finalize/Remove from racing a still-active step agent? [Coverage, Spec §Edge Cases]
- [ ] CHK016 — Is Remove's postcondition ("subsequent Status reports absent") stated as verifiable? [Measurability, Spec §FR-008]

## Multi-Step Chain (US3)

- [ ] CHK017 — Is the chain-source resolution order (per-dispatch override → config default → single implicit step 0) specified without ambiguity? [Clarity, Spec §FR-012]
- [ ] CHK018 — Is the M2 single-step (index 0) default explicitly preserved as a requirement? [Consistency, Spec §FR-012]
- [ ] CHK019 — Are per-step invocation profile contents (permission mode + prompt) specified, and is silent-defaulting of permission mode prohibited? [Completeness, Spec §FR-012a]
- [ ] CHK020 — Are advance and loop-back defined as operator-driven with explicit range checks (advance < len, loopback ≥0 and < current)? [Clarity, Spec §FR-014]
- [ ] CHK021 — Is out-of-range step transition behavior specified as a typed error, not a silent clamp? [Edge Case, Spec §Edge Cases]
- [ ] CHK022 — Is the widening of `/steps/{idx}` to accept idx>0 stated while still rejecting invalid indices? [Completeness, Spec §FR-013]
- [ ] CHK023 — Is chain progression explicitly required to be run/orchestrator state and NOT a new beads column? [Consistency, Spec §FR-016]

## Idempotent Dispatch (US4) & the 409→200 Migration

- [ ] CHK024 — Is the idempotency key (bead identity + in-flight state) explicitly defined, and the absence of an Idempotency-Key header stated? [Clarity, Spec §FR-017]
- [ ] CHK025 — Is the response distinction between "new run started" and "existing run joined" specified? [Completeness, Spec §FR-018]
- [ ] CHK026 — Is the 409→200 status change documented as an explicit, versioned migration (not a silent break)? [Conflict, Plan §Complexity Tracking]
- [ ] CHK027 — Are the two migrated M2 tests named so the change is traceable? [Traceability, Plan §Complexity Tracking]
- [ ] CHK028 — Is concurrent-duplicate behavior (racing dispatches → exactly one run) specified? [Coverage, Spec §FR-019]
- [ ] CHK029 — Is post-completion behavior specified (a fresh dispatch of a no-longer-active bead starts a new run — no permanent lock)? [Edge Case, Spec §FR-020]
- [ ] CHK030 — Is idempotency after crash-recovery specified (re-dispatch against a recovered run still joins)? [Coverage, Spec §FR-019]

## Quota (US5, best-effort)

- [ ] CHK031 — Is the quota source unambiguously specified as the on-disk session record (not stream scraping)? [Clarity, Spec §FR-022]
- [ ] CHK032 — Is the unknown/unavailable outcome specified as a non-failing, best-effort result? [Completeness, Spec §FR-023]
- [ ] CHK033 — Is the spike-before-implement dependency (pin path/format against real claude) stated as a prerequisite? [Assumption, Research §R8]
- [ ] CHK034 — Is the droppability of US5 (without affecting US1–US4) documented? [Dependency, Spec §Assumptions]

## Additive Surface & Cross-Cutting (Constitution)

- [ ] CHK035 — Are all new WS event types named and confined to additive dotted namespaces (dispatch.*, step.*, worktree.*, run.quota)? [Consistency, Spec §FR-024a]
- [ ] CHK036 — Is "no M0–M3 route/shape/event changed or removed" stated as a testable success criterion (excepting the one documented migration)? [Measurability, Spec §SC-008]
- [ ] CHK037 — Is the beads-source-of-truth constraint (all issue writes via `bd`; no new durable issue state) explicitly reaffirmed for every new stateful element? [Consistency, Spec §FR-026]
- [ ] CHK038 — Is the thin-handler requirement stated for every new endpoint (logic in service/orchestrator, not api)? [Consistency, Spec §FR-028]
- [ ] CHK039 — Is TDD (failing test first) + fake-on-$PATH + skip-gated real-binary coverage required for each new external-CLI behavior (git push, jj write-side, claude quota)? [Completeness, Spec §FR-027]
- [ ] CHK040 — Is `-race` cleanliness required for the new scheduler/admission/idempotency concurrency paths specifically? [Coverage, Spec §SC-007]

## Notes

- 40 items, ≥80% carry a traceability reference (spec §, plan §, research §, or a [Gap]/[Conflict]/[Assumption] marker).
- Items test whether the **requirements are well-written**, not whether the future code works.
- CHK026/CHK027 track the single intentional Constitution-V migration; CHK033 tracks the quota spike prerequisite.
