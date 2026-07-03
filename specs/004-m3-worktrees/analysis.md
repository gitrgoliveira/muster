# Cross-Artifact Analysis: M3 — Worktrees

Non-destructive consistency check across spec.md ↔ plan.md ↔ data-model ↔ contracts ↔ tasks.md.
Date: 2026-07-03.

## FR → Task coverage matrix

| FR | Covered by | OK |
|---|---|---|
| FR-001 Backend interface | T005 | ✅ |
| FR-002 git backend, M2-preserving | T008, T009 | ✅ |
| FR-003 jj backend | T022, T023, T024 | ✅ |
| FR-004 / 004a resolver + jj-source-only | T007, T021 | ✅ |
| FR-005 /worktree summary | T016–T019 | ✅ |
| FR-006 /diff whole+file | T015, T018 | ✅ |
| FR-007 ?path= safety | T006, T018 | ✅ |
| FR-008 streaming | T015 | ✅ |
| FR-009 404 no-worktree | T017 | ✅ |
| FR-009a config default VCS | T012, T032 | ✅ |
| FR-010 startup probe | T007, T012 | ✅ |
| FR-011 / 011a VCS_UNAVAILABLE | T012, T021 | ✅ |
| FR-012 status DTO additive | T027, T028 | ✅ |
| FR-013 change-kind classification | T013, T014, T023 | ✅ |
| FR-014 additive surface | T034 | ✅ |
| FR-015 immutability (deferred) | n/a | ✅ |
| FR-016 fakes + skip-gated integration | T002, T003, T008, T021, T025 | ✅ |
| FR-017 ErrNotImplemented write-side | T009, T022 | ✅ |
| FR-018 --default-vcs allow-list | T032 | ✅ |

**All 18 FRs (+sub) have ≥1 implementing task. No orphan FRs.**

## SC → verification mapping

| SC | Verified by |
|---|---|
| SC-001 M2 suite green through wt | T008, T011 checkpoint |
| SC-002 summary+diff correct (git+jj) | T020, T025 |
| SC-003 single-file diff | T020 |
| SC-004 VCS_UNAVAILABLE always | T012, T021 |
| SC-005 traversal rejected | T006, T017 |
| SC-006 -race + coverage gates | T033 |
| SC-007 additive surface intact | T034 |

## Consistency checks

- ✅ Interface method set identical across data-model.md, contracts/wt-backend.md, plan.md (7 methods; 4 impl / 3 `ErrNotImplemented`).
- ✅ Error→HTTP mapping consistent: `ErrWorktreeNotFound`→404, `ErrVCSUnavailable`→412, path→400 (data-model ↔ http-endpoints).
- ✅ git non-mutating decision consistent (research §3 ↔ plan invariant 1 ↔ T014 note).
- ✅ jj "source repos only" consistent (clarification ↔ FR-004a/011a ↔ T021).
- ✅ Additive surface: no M2 route/DTO/event modified (contracts ↔ T034).
- ✅ No task references an out-of-scope item (Finalize/Push/Remove only as ErrNotImplemented; GC/sub-beads absent).

## Findings

- **No blocking inconsistencies.** Coverage complete, no contradictions, scope boundaries respected.
- **Minor (carried, not blocking)**: CHK018 git rename detection → explicitly owned by T013; ahead/behind decision → owned by T029. Both are implementation choices with a defined default, not spec gaps.

**Verdict: READY for review + implementation.**
