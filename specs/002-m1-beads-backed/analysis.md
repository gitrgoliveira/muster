# Phase 6 Analysis: M1 Cross-Artifact Consistency

**Date**: 2026-05-24 | **Reviewer**: Opus 4.6

14 findings. All CRITICAL and HIGH items resolved by updating tasks.md, checklist, and spec.md.

## Resolved (fixed in artifacts)

| # | Severity | Finding | Fix |
|---|---|---|---|
| 1 | CRITICAL | `cmd/musterd/` vs `cmd/muster/` — build broken | T001 marked as BLOCKER |
| 2 | HIGH | Store write methods disappear; BeadService has no write path | Added T015–T018: mapper, service refactor, handler refactor |
| 3 | HIGH | `idPattern` regex rejects real beads IDs (mp-kbj, muster-xyz) | Added T007 and CHK008 |
| 4 | HIGH | Issue→Bead mapping layer missing from tasks | Added T015–T016, CHK037–CHK038 |
| 5 | HIGH | Handlers bypass service layer; type mismatch after Backend change | Added T017–T018, CHK039–CHK039a |
| 6 | MEDIUM | Filter.Status type mismatch (string vs []string) | Fixed spec.md to []string |
| 10 | MEDIUM | Spec oversimplifies exit-code mapping | Fixed spec US5.4 to reference contract table |
| 13 | LOW | yaml.v3 already indirect dep | Fixed T004 to "promote" |
| 14 | LOW | `memory.go` → actual file is `memstore.go` | Fixed plan.md and checklist |

## Acknowledged → addressed in Phase 7+

| # | Severity | Finding | Status |
|---|---|---|---|
| 7 | MEDIUM | Filter.Limit undocumented purpose | Added comment in spec: "internal use only" |
| 8 | MEDIUM | OrchestratorStatusResponse needs BackendConfig | Resolved by T034/T034b |
| 9 | MEDIUM | quickstart.md bin/muster inconsistency | Fixed in prior revision |
| 11 | MEDIUM | CommentRequest.actor silently dropped | Resolved: actor prepended into note text via `--append-notes` (CHK111c, T070) |
| 12 | MEDIUM | "scheduled" column collapses to "open" | Known M1 simplification — explicitly documented in bd-cli-bridge.md |

See `review.md` for the Phase 7 cross-model review and the additional 18 findings (CRITICAL/HIGH/MEDIUM/LOW) it surfaced and resolved.
