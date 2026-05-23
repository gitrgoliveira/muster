# Pre-Implementation Review

**Feature**: M0 — Skeleton (musterd binary + in-memory API + WS)
**Artifacts reviewed**: spec.md, plan.md, tasks.md, data-model.md, contracts/rest-api.md, contracts/ws-events.md
**Review model**: claude-sonnet-4-6
**Date**: 2026-05-22

## Summary

| Dimension | Verdict | Issues |
|-----------|---------|--------|
| Spec-Plan Alignment | WARN | plan.md Phase 10 omitted `serve` subcommand (fixed) |
| Plan-Tasks Completeness | WARN | T027 missing UnknownField test; T012 dispatch missing Step assertion (fixed) |
| Dependency Ordering | PASS | All 11 phases correctly sequenced |
| Parallelization Correctness | FAIL→FIXED | T015/T016 both touched beads.go but were marked [P] (fixed: now sequential) |
| Feasibility & Risk | WARN | T010 Patch typed; T035 large; cover-check algorithm added |
| Standards Compliance | PASS | TDD throughout; coverage gates; lint stack; constitution is unfilled template |
| Implementation Readiness | WARN | ws-events.md channel type fixed; T026 seed init noted; cover-check algorithm added |

**Overall**: READY (all FAIL items resolved; WARN items addressed)

## Fixes Applied

- **F1**: T015/T016 made sequential; T016 explicitly prohibited from modifying beads.go
- **W1**: plan.md Phase 10.2 now mentions `serve` subcommand and `TestNoSubcommand_PrintsUsageExits1`
- **W2**: T027 now includes `TestCreate_400_UnknownField` and explicit `DisallowUnknownFields()` call
- **W3**: T012 dispatch test expanded to assert Step fields (agent, mode, skills=[], status=StepActive)
- **W4**: T010 store.Patch changed from `map[string]interface{}` to typed `store.PatchBeadInput`; T010 also defines `NewMemStore(seeds []core.Bead)` constructor
- **W5**: contracts/ws-events.md updated: `send chan []byte` → `send chan ws.Frame`
- **W6**: T012 specifies `NewMemStore(seeds []core.Bead)` constructor; T026 explicitly seeds with `store.SeedBeads()`
- **W7**: T004 cover-check target now includes awk-based per-package threshold algorithm
