# Requirements Quality Checklist: M3 — Worktrees (`wt.Backend`)

**Purpose**: Validate the spec/plan are complete, unambiguous, and ready for tasks +
implementation. A "unit test for the spec." Each item is checked against spec.md, plan.md,
research.md, and the contracts. `[x]` = passes; `[ ]` = gap to resolve before Phase 5.

## Requirement Completeness

- [x] CHK001 — Every user story (US1–US4) has a corresponding set of FRs. [US1→FR-001/002; US2→FR-005/006/007/008; US3→FR-003/004/004a/011/011a; US4→FR-010/012]
- [x] CHK002 — Every FR is testable and has at least one Success Criterion or acceptance scenario. [SC-001..007 map to FR-002/005-008/011/013-014]
- [x] CHK003 — The "no worktree" path is specified with an exact HTTP status. [FR-009 → 404 WORKTREE_NOT_FOUND]
- [x] CHK004 — VCS selection source is unambiguous given beads has no `vcs` field. [Clarification + FR-004/009a: config default `--default-vcs`]
- [x] CHK005 — jj-for-git-repo behavior is defined (no silent fallback, no implicit colocate). [FR-004a/011a; research §1]
- [x] CHK006 — The write-side interface methods have a defined M3 behavior. [FR-017: ErrNotImplemented]
- [x] CHK007 — The change-kind taxonomy is enumerated and mapped from both backends' native output. [data-model change-kind table; research §3]

## Requirement Clarity & Measurability

- [x] CHK008 — No `[NEEDS CLARIFICATION]` markers remain. [verified: grep clean]
- [x] CHK009 — "Diff exposure" is concrete: whole-worktree and single-file, streamed, git-format. [FR-006/008; contract]
- [x] CHK010 — Path-safety requirement names the rejected inputs (absolute, `..`, escape). [FR-007; data-model safeRelPath]
- [x] CHK011 — Backward-compatibility is measurable (which M2 surface must not change). [SC-007; http-endpoints additive checklist]
- [x] CHK012 — Coverage gates are quantified per package. [plan: internal/wt ≥85%, others ≥ M2 gate]

## Edge Cases & Failure Modes

- [x] CHK013 — Untracked (newly created) files are covered by the summary/diff. [research §3; FR-013; git uses status --porcelain]
- [x] CHK014 — Large/binary diffs won't exhaust memory. [FR-008 streaming; binary elision noted in edge cases]
- [x] CHK015 — Backend binary missing at startup vs at dispatch is distinguished. [FR-010 startup probe / FR-011 dispatch refusal]
- [x] CHK016 — Concurrent read while agent writes has defined (best-effort snapshot) semantics. [spec edge cases]
- [x] CHK017 — Reads must not mutate the agent's worktree/index. [plan invariant 1; git non-mutating decision]
- [ ] CHK018 — Rename/copy detection reliability across backends is bounded. [PARTIAL: mapping table exists, but git rename needs `-M`/`--find-renames` and the spike's rename probe was inconclusive — implementation must pin git rename flag and test it; jj emits `R` natively. Flag for tasks.]

## Consistency & Non-Duplication

- [x] CHK019 — FRs don't contradict the clarifications. [FR-015 deferred consistent with "config default"; FR-004a consistent with "jj source only"]
- [x] CHK020 — The interface in data-model, contract, and plan agree (same 7 methods, same M3 impl/deferred split).
- [x] CHK021 — Endpoint paths match the roadmap §7/§8 and don't collide with M2 routes. [/worktree, /diff additive]

## Scope Boundary Clarity

- [x] CHK022 — Out-of-scope items are explicit (Finalize/Push/Remove, GC, sub-beads, UI wiring, worktree.changed WS). [spec Out of Scope]
- [x] CHK023 — US3 (jj) is structured to be droppable without touching US1/US2. [clarification; plan Phase-2 note]
- [x] CHK024 — No new Go module / no constitution-level dependency decision is hidden. [plan: go.mod unchanged; Constitution I ✅]

## Constitution Alignment

- [x] CHK025 — No credential/identity handling introduced (jj read+create needs none). [research §6; Constitution]
- [x] CHK026 — No silent defaults for user-controlled behavior (VCS unavailability refused, not defaulted). [FR-011]
- [x] CHK027 — Test-first + fakes-on-`$PATH` + skip-gated real-binary integration is planned. [FR-016; research §7]
- [x] CHK028 — Handlers stay thin; new logic lives behind the `wt.Backend` interface. [plan structure; Constitution III]

---

## Open items to carry into Tasks

1. **CHK018** — git rename/copy detection: pin the exact flag (`git status` renames need
   `-M`/rename detection is on by default for `status`, but `--porcelain` rename entries
   require rename detection enabled) and add a rename test for **both** backends. Do not
   ship rename mapping untested.
2. Confirm `WorktreeStatus.Ahead/Behind` is either implemented (git `rev-list --count`) or
   explicitly left `0` in M3 with a task note (US4/P3).

**Verdict**: spec + plan are implementation-ready. One partial (CHK018) is a
test-coverage note for the tasks phase, not a spec gap.
