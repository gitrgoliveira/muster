# Checklist Status — post round-3 P1 closure

**Generated**: 2026-05-22 (round 3)
**Source checklists**: `requirements.md`, `contracts.md`, `gates.md` (173 items total)

This file is an audit of the three checklists against the post-resolution artifacts. It
classifies each item as **Resolved** (the spec/contract/plan now answers it), **Open Gap**
(a genuine missing requirement still to address before implementation), or **Deferred**
(intentionally postponed to M1 or later).

---

## Tally

| Round | Resolved | Open Gap | Deferred |
|---|---:|---:|---:|
| Round 1 (post-checklist generation) | 0 | 161 | 12 |
| Round 2 (post conflict reconciliation) | 71 (41%) | 90 (52%) | 12 (7%) |
| **Round 3 (post P1 gap closure)** | **~107 (62%)** | **~54 (31%)** | **12 (7%)** |

**Round 3 closed 36 additional items** via 4 user decisions + 11 default resolutions. The
remaining ~54 items are P2/P3/hygiene — none block implementation.

---

## Resolved — by category

All resolutions reflected in artifact updates dated 2026-05-22 (round 2).

### Cross-document conflicts (every [Conflict] item now resolved)

| Item | Resolution | Authority |
|---|---|---|
| WS event type list (7 vs 3) | All 7 documented | spec FR-012 / ws-events.md |
| Hello within 1s | Added to WS contract + Hub | spec FR-013 |
| Ping/pong protocol | Added to WS contract + readPump | spec FR-014 |
| Move request shape `{toColumn, beforeID?}` | Unified across spec/contracts/data-model | spec FR-009 |
| `beforeID` semantics | Implemented in M0 (slice splice) | clarifications round 2 |
| POST /comments returns 201 | Aligned (was 200 in contracts) | spec US9 |
| Dispatch: scheduled→running + 2 history events + `INVALID_STATE` gate | Aligned across all docs | spec FR-010 / US8 |
| Error codes `INTERNAL` (not `INTERNAL_ERROR`); `INVALID_STATE` added | Aligned | spec §Error Codes |
| `/orchestrator/status` full 6-field payload | Aligned (was `{running, mode}` stub) | spec US10 |
| Empty PATCH body → 400 | Aligned (was 200 no-op in contracts) | clarifications round 2 |
| Startup banner format | Aligned to spec format | spec §Assumptions |
| ID-collision retries | Max 3 attempts (1+2) across all docs | research §Decision 4 |
| Module layout (resource subpackages + services) | Spec §Technical Context updated | plan.md authority |
| Coverage targets | Spec SC-006 defers to plan per-package matrix | clarifications round 2 |
| Provider/Capacity/DOLT seed scope | All 4 datasets in M0 store | clarifications round 2 |

### Per-checklist resolved items

**`requirements.md`** — 32 of 80 resolved:
CHK003, CHK010, CHK011, CHK017, CHK022, CHK023, CHK024, CHK025, CHK026, CHK027, CHK028,
CHK029, CHK030, CHK031, CHK032, CHK033, CHK034, CHK035, CHK036, CHK037, CHK038, CHK042,
CHK043, CHK056, CHK059, CHK072, CHK073, CHK074, CHK075, CHK076, CHK077, CHK078.

**`contracts.md`** — 30 of 50 resolved:
CHK002, CHK003, CHK005, CHK006, CHK007, CHK009, CHK010, CHK011, CHK012, CHK013, CHK015,
CHK016, CHK018, CHK019, CHK020, CHK021, CHK022, CHK023, CHK024, CHK025, CHK030, CHK031,
CHK032, CHK034, CHK035, CHK036, CHK037, CHK045, CHK046, CHK047.

**`gates.md`** — 9 of 43 resolved:
CHK001, CHK004, CHK007, CHK010, CHK011, CHK012, CHK013, CHK014, CHK023.

---

## Open Gaps — genuine missing requirements

These are real holes the implementer would have to guess at. Grouped by impact.

### Should-decide-before-tasks (P1 — affects test design)

- **`requirements.md` CHK001** — Acceptance scenarios still missing for FR-001 (single binary), FR-015 (healthz), FR-017 (X-Request-ID), FR-020 (libraries-as-requirements).
- **`requirements.md` CHK005** — Graceful shutdown spec (SIGINT/SIGTERM, in-flight drain timeout, WS close code).
- **`requirements.md` CHK006** — `--addr` flag: format validation, IPv6 support, behaviour when port-in-use beyond "log error and exit".
- **`requirements.md` CHK008** — Body size / max request size for POST/PATCH.
- **`requirements.md` CHK040, CHK041** — `SC-002 <50ms`, `SC-005 <100ms` — percentile + sample-size + warm/cold.
- **`contracts.md` CHK014** — Title 255-char limit in **bytes vs runes** for UTF-8.
- **`contracts.md` CHK017** — Validation order for multi-field-invalid requests (which error returned first).
- **`contracts.md` CHK027** — Max WS frame size (defaults to library setting unless we override).
- **`contracts.md` CHK040, CHK041** — Timestamp format inconsistency (RFC3339 for new events vs prototype's "Mon 09:14" calendar strings in seed). Pick one and convert seed.
- **`contracts.md` CHK042, CHK043, CHK044** — `NowPlaying.Kind`, `LogEntry.Kind`, `FileChange.Status` as enums (currently free-form strings).
- **`gates.md` CHK002** — Coverage measurement: line vs branch vs statement.
- **`gates.md` CHK003** — Coverage as gate vs aspiration.
- **`gates.md` CHK008** — Explicit FR → test traceability link.
- **`gates.md` CHK015–CHK020** — Lint/format/build-vet requirements.
- **`gates.md` CHK026–CHK030** — Pre-merge gates (PR checklist, commit conventions, merge strategy).

### Should-decide-eventually (P2 — operational concerns)

- **`requirements.md` CHK013** — Observability beyond logging (metrics, /metrics endpoint).
- **`requirements.md` CHK060–CHK066** — Non-functional budgets: memory, CPU, request rate, logging level config.
- **`requirements.md` CHK067** — FR-020 mandating specific libraries — anti-pattern; move to plan/research.
- **`requirements.md` CHK068, CHK069** — Dependency version pinning policy; Go 1.26-specific feature audit.
- **`gates.md` CHK006** — Test runtime budget.
- **`gates.md` CHK031–CHK035** — CI matrix, security checks (`govulncheck`/`gosec`), dependency update policy.
- **`gates.md` CHK036–CHK038** — Performance gate enforcement (benchmarks, memory regression bounds).
- **`gates.md` CHK039–CHK041** — Test naming convention, flake policy, race-detector severity.

### Low-impact edge cases (P3 — implementer can make a reasonable call)

- **`requirements.md` CHK052** — Repeated `?column=` query params.
- **`requirements.md` CHK053** — UTF-8 multi-byte string in `title`.
- **`requirements.md` CHK054** — Integer overflow in numeric fields.
- **`requirements.md` CHK055** — Concurrent PATCH semantics (last-write-wins assumed; not spec'd).
- **`requirements.md` CHK057** — Empty `ui/` directory at runtime.
- **`requirements.md` CHK058** — `X-Request-ID` with junk content (control chars, length).
- **`contracts.md` CHK048** — Server-shutdown end-to-end (REST drain semantics).
- **`gates.md` CHK022** — `cp -r` cross-platform (Windows build hosts).
- **`gates.md` CHK024** — Exact prototype-to-ui file mapping (include `debug_test.html`?).
- **`gates.md` CHK025** — Reproducible build flags (`-trimpath`, `-buildvcs=false`).
- **`gates.md` CHK042, CHK043** — FR → SC → test → file traceability table.

### Documentation / traceability hygiene

- **`requirements.md` CHK015, CHK016** — Inline Bead shape vs external reference; prototype-files-list enumeration.
- **`requirements.md` CHK020, CHK021** — SC-002 "matching seed count" hard or soft; SC percentile pinning.
- **`requirements.md` CHK044, CHK045** — Measurable SCs for "tests pass" and "self-contained binary".
- **`requirements.md` CHK046–CHK051** — Alternate/exception/recovery scenario coverage (every error-matrix row should have a US acceptance scenario).
- **`requirements.md` CHK070, CHK071** — Babel-in-browser assumption validity; cross-doc references pinned.
- **`requirements.md` CHK079, CHK080** — Requirement-ID scheme + bidirectional cross-references.
- **`contracts.md` CHK049, CHK050** — FR-→endpoint-→contract-section mapping table.

---

## Deferred to M1 (intentional, not gaps)

These are explicitly out of M0 scope and noted in spec.md:

- WS `seq` counter + replay buffer (FR-012)
- `/move` lifecycle gating beyond the dispatch case (FR-009)
- Real agent dispatching (Assumptions)
- Dolt/Beads integration (Assumptions)
- Tmux integration (Assumptions)
- Providers / capacity endpoints (Seed Data — data is in store, no endpoints)
- `bead.deleted` event (data-model §5 / ws-events.md — reserved)
- JSON log format + configurable log level (Assumptions)
- Pagination (`nextCursor` always null in M0)

---

## Round-3 closures (P1 gaps)

User-decided (4 items):

- **Seed timestamp format** → opaque strings; new events use RFC3339, seed retains prototype calendar form. Documented in `data-model.md §7.3` + `contracts/rest-api.md` cross-cutting.
- **Body size limit** → 1 MiB via `http.MaxBytesReader`. Exceeded → 413 with `INVALID_REQUEST`. Documented in `data-model.md §7.4`, `contracts/rest-api.md` cross-cutting, `plan.md §Phase 6.3`.
- **Coverage targets** → CI **gates**, not aspirations. Per-package thresholds in `plan.md §Test Coverage Targets`. Documented in `plan.md §Test Coverage Targets`.
- **Lint stack** → `gofmt + go vet + golangci-lint` with pinned `.golangci.yml`. Linters: errcheck, govet, gofmt, gosimple, ineffassign, staticcheck, unused. Documented in `plan.md §Lint & Format Gates`.

Default-resolved (11 items — sensible defaults applied without further input):

- Graceful shutdown: SIGINT/SIGTERM → 5 s drain → WS close 1001 → exit 0 (or 1 on timeout)
- `--addr` parsing: `net.SplitHostPort`, IPv6 supported, port-in-use exits 1
- Title length: 255 **runes** (not bytes)
- Validation order: body-size → structural → required → enum → numeric range (first failure wins)
- WS read limit: 1 MiB (matches REST body cap)
- `NowPlayingKind`, `LogKind`, `FileStatus` typed enums added to `core` (was free-form string)
- SC-002 / SC-005 latency budgets: p95 over warm samples; sample sizes pinned
- FR-001 acceptance: file-type check + version banner + binary portability test
- FR-015 acceptance: `curl ... /healthz | jq '.ok' == true`
- FR-017 acceptance: X-Request-ID header presence verified on every endpoint, including errors
- FR-020 acceptance: dep versions in `go.mod` match `research.md §Decision 9`
- Pre-merge gate: build clean + race tests green + coverage gate + lint pass

All round-3 decisions are recorded in `spec.md §Clarifications / Session 2026-05-22 (round 3)`.

## Recommended action before Phase 5 (Tasks)

**Round-3 closure leaves only P2/P3/hygiene items outstanding** (~54). These do not block
implementation:

- **~21 P2** — operational concerns (memory/CPU budgets, security checks, CI matrix, dep update policy, observability beyond logging). Safe to defer to M1 or capture as `bd` issues.
- **~11 P3** — edge cases the implementer can reasonably default (repeated query params, UTF-8 multi-byte titles, integer overflow, junk request IDs, `cp -r` on Windows, etc.).
- **~22 hygiene** — traceability and cross-doc reference tightening (FR→endpoint→test mapping tables, requirement ID schemes). Useful but not blocking.

**Ready for Phase 5 (Tasks).** Suggested next step: invoke `speckit-tasks` with the spec, plan,
data-model, and contracts as input. The tasks agent should organise the work into the
phase A–F sequence already in `plan.md §Implementation Phases`.

P2/P3/hygiene items can be captured as `bd` issues post-implementation, or addressed in M1.
