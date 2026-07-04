# Cross-Model Review: M3 — Worktrees (`wt.Backend`)

**Reviewer model**: Opus 4.8 (operator-selected) · **Date**: 2026-07-03 · **Mode**: read-only

Evaluates spec.md, plan.md, tasks.md, data-model, contracts, research, checklist across
7 dimensions before implementation.

## Summary

| Dimension | Verdict |
|---|---|
| 1. Spec ↔ Plan alignment | **PASS** |
| 2. Plan ↔ Tasks completeness | **PASS** |
| 3. Dependency ordering | **PASS** |
| 4. Parallelization correctness | **PASS** (with 1 note) |
| 5. Feasibility & risk | **PASS** (spike de-risked the unknowns) |
| 6. Standards / constitution compliance | **PASS** |
| 7. Implementation readiness | **PASS** |

**Overall: READY to implement.** No FAIL items. 2 WARN-level notes below (non-blocking).

## Findings by dimension

**1. Spec ↔ Plan alignment — PASS.** Every FR maps to a plan element; the plan introduces
nothing the spec didn't ask for. The scope cut (read+create only; write-side deferred) is
stated identically in both.

**2. Plan ↔ Tasks completeness — PASS.** analysis.md confirms 18/18 FR coverage and 7/7 SC
mapping. Per-package coverage gates in the plan are reflected in T033.

**3. Dependency ordering — PASS.** Setup → interface → git (MVP) → diff endpoints → jj →
status → polish is correctly topologically ordered. The orchestrator refactor (T011) lands
after the git backend exists (T009) and before the endpoints depend on it.

**4. Parallelization correctness — PASS (WARN note).** `[P]` groups touch disjoint files.
*Note*: T021–T026 (jj) are labeled droppable/parallel-safe vs US2 but the tasks doc rightly
says they share the `/diff` handler test infra, so they must land after T018. Ensure a jj
implementer doesn't start before the handler exists — the sequencing note covers this.

**5. Feasibility & risk — PASS.** The two genuine unknowns were retired by the real-binary
spike: (a) jj/git diff format compatibility (confirmed byte-compatible), (b) the git
untracked-files trap (caught — plan uses `status --porcelain`, non-mutating). This is
exactly the constitution's "verify empirically before building." Highest residual risk is
the US1 orchestrator refactor regressing M2 behavior; mitigated by wrap-don't-rewrite +
SC-001 (M2 suite must stay green).

**6. Standards / constitution compliance — PASS.** No new Go module (Principle I); capability
behind an interface, thin handlers (III); TDD + fakes + skip-gated integration (IV); additive
surface (V); no credential handling and no silent VCS fallback (Security). Clean.

**7. Implementation readiness — PASS.** Command contracts are pinned to exact invocations;
error→HTTP mappings are specified; DTO shapes are in the contract. An implementer has no
undefined decisions except the two explicitly-owned ones (T013 rename flag, T029 ahead/behind).

## WARN notes (non-blocking, for the implementer)

- **W1 — git rename detection**: `git status --porcelain` reports renames only with rename
  detection active and the two files staged/related; for an agent's *uncommitted* worktree,
  a rename often surfaces as delete+add. T013 must test the real behavior and either enable
  rename detection or document that renames appear as D+A in M3. Do not assume `R` entries.
- **W2 — streaming child lifecycle**: the `Diff` `io.ReadCloser` wraps a `git`/`jj` child's
  stdout; `Close()` must `Wait()` the child to avoid zombies, and a client disconnect
  mid-stream must cancel the ctx so the child is killed. T015 should assert no leaked
  process/goroutine.

## Recommendation

Proceed to implementation. Address W1/W2 within their owning tasks (T013/T015). Keep the M2
test suite as the green tripwire throughout US1.
