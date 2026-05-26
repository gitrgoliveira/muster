# Requirements Quality Checklist: M0 — Skeleton

**Purpose**: Validate that the functional requirements, acceptance scenarios, success criteria, 
and edge cases in `spec.md` are complete, clear, consistent, and measurable — before any code is 
written.
**Created**: 2026-05-22
**Feature**: [spec.md](../spec.md)

> This checklist tests the **requirements themselves**, not the implementation. Each item asks 
> whether the spec is well-written, not whether the system works.

## Requirement Completeness

- [ ] CHK001 Are acceptance scenarios specified for every FR (FR-001 through FR-020)? Several FRs (e.g., FR-001 `go:embed`, FR-015 `/healthz`, FR-017 `X-Request-ID`, FR-020 library choices) have no matching User Story acceptance scenario. [Completeness, Spec §Requirements]
- [ ] CHK002 Are the exact response shapes specified for every endpoint, including success and error bodies? [Completeness, Spec §FR-004..FR-016]
- [ ] CHK003 Is the `/orchestrator/status` response payload shape fully specified, including required vs optional fields for `build`, `schemaVersion`, `beadsVersion`, `online`, `serverTime`, `dolt`? [Completeness, Spec §US10 / §Technical Context]
- [ ] CHK004 Are the seed data invariants (count, IDs, columns, derived field rules) specified normatively — i.e., is the seed considered a contract, or a starting suggestion? [Completeness, Spec §FR-005]
- [ ] CHK005 Are graceful-shutdown requirements defined (SIGINT/SIGTERM handling, in-flight request drain timeout, WS close frame status)? [Gap]
- [ ] CHK006 Are requirements for the `--addr` flag specified, including format validation, IPv6 support, and behaviour when port is already in use? [Completeness, Spec §FR-002, §Edge Cases]
- [ ] CHK007 Are CORS / Origin requirements explicitly addressed (even if the answer is "none in M0")? [Completeness, Spec §Assumptions]
- [ ] CHK008 Are body-size / max-request-size requirements specified for `POST` and `PATCH` endpoints? [Gap]
- [ ] CHK009 Are JSON encoding rules specified (e.g., `null` for nested optional structs vs empty array for slice fields)? [Gap]
- [ ] CHK010 Is the providers/capacity reference data (4 providers + 4 capacity entries) requirement still in scope for M0, given no endpoints reference it? Or should FR-005 be narrowed to "14 beads only"? [Completeness, Spec §FR-005 / §Seed Data]
- [ ] CHK011 Are requirements for the in-memory store's behaviour on duplicate IDs (collision during create) specified? [Gap]
- [ ] CHK012 Is the behaviour on server restart documented (i.e., "state is lost — clients should expect a fresh seed")? [Completeness, Spec §Assumptions]
- [ ] CHK013 Are observability requirements specified beyond logging (e.g., must `/metrics` exist? request counts? error rates?)? [Gap]
- [ ] CHK014 Is the M0/M1 boundary explicit for every deferred feature (seq counter, replay buffer, lifecycle gating, real agent dispatch, Dolt/Beads, tmux)? [Completeness, Spec §Assumptions]

## Requirement Clarity

- [ ] CHK015 Is "matching the `Bead` shape from `spec.md §3.1`" replaced by an inline shape definition in this spec, or is the external reference normatively pinned to a specific revision? [Clarity, Spec §FR-004]
- [ ] CHK016 Is "the prototype files" enumerated explicitly, so the embed surface is well-defined? [Clarity, Spec §FR-003]
- [ ] CHK017 Is "no lifecycle ordering is enforced" in FR-009 (move) reconciled with FR-010 (dispatch) which implies a scheduled→running ordering? [Clarity, Spec §FR-009 vs §FR-010]
- [ ] CHK018 Is the term "in-memory" defined operationally (single process, no replication, no persistence on restart) so no consumer mistakes it for a cache layer in front of something else? [Clarity, Spec §FR-005 / §Assumptions]
- [ ] CHK019 Is "the beads naming format" (`bd-` + first 4 hex chars of UUIDv4) specified as a wire-format invariant or as a generation strategy? Either is fine; the spec must pick one. [Clarity, Spec §FR-007]
- [ ] CHK020 Is "matching seed data count" in SC-002 stated as a hard invariant or merely an indicator? If the seed grows in a future M0 revision, does SC-002 follow? [Clarity, Spec §SC-002]
- [ ] CHK021 Are timing units in SC-001/SC-004/SC-005 specified at a percentile (p50? p95?), or are they intended as absolute upper bounds for a single sample? [Clarity, Spec §SC-001..SC-005]
- [ ] CHK022 Is the term "broadcasts only" in FR-012 unambiguous about per-client filtering (none) and per-bead filtering (none)? [Clarity, Spec §FR-012]

## Requirement Consistency

- [ ] CHK023 Does the WS event-type list in FR-012 (`hello`, `bead.created`, `bead.updated`, `bead.moved`, `bead.deleted`, `comment.added`, `pong`) match `contracts/ws-events.md` (which only documents `bead.created`, `bead.updated`, `bead.deleted`)? **Known conflict.** [Conflict, Spec §FR-012 vs contracts/ws-events.md]
- [ ] CHK024 Does US7 scenario 1 (`hello` within 1 s of WS connection) appear in the WS contract document? **Known gap.** [Conflict, Spec §US7 / §FR-013 vs contracts/ws-events.md]
- [ ] CHK025 Does FR-014 (ping → pong) reconcile with `contracts/ws-events.md` § "Client → Server Frames" which states "the server ignores all application frames"? **Known conflict.** [Conflict, Spec §FR-014 vs contracts/ws-events.md]
- [ ] CHK026 Does US6 request body (`{toColumn, beforeID?}`) match `contracts/rest-api.md` move body (`{column}`)? **Known conflict.** [Conflict, Spec §US6 vs contracts/rest-api.md]
- [ ] CHK027 Is the `beforeID?` positional-insertion requirement in US6 either specified in the contract or explicitly deferred? **Known gap.** [Conflict, Spec §US6 vs contracts/rest-api.md]
- [ ] CHK028 Does US9 (POST /comments returns `201`) match `contracts/rest-api.md` (returns `200`)? **Known conflict.** [Conflict, Spec §US9 vs contracts/rest-api.md]
- [ ] CHK029 Does US9 (`comment.added` WS event) match `contracts/ws-events.md` (only `bead.updated` is emitted for comments)? **Known conflict.** [Conflict, Spec §US9 vs contracts/ws-events.md]
- [ ] CHK030 Does US8 (dispatch transitions column to `running`, appends `claimed`+`started` events, and returns `400 INVALID_STATE` from non-`scheduled` columns) match `contracts/rest-api.md` (dispatch only appends a step + `claimed` event; no column transition, no `INVALID_STATE`)? **Known conflict** — appears as multiple discrepancies. [Conflict, Spec §US8/§FR-010 vs contracts/rest-api.md]
- [ ] CHK031 Does the spec's clarification ("/move is unrestricted in M0") imply dispatch is also unrestricted, or do US8/FR-010 still impose `scheduled→running` ordering as the only valid path? Pick one. [Conflict, Spec §Clarifications vs §US8/§FR-010]
- [ ] CHK032 Does the spec error-code table (`BEAD_NOT_FOUND`, `INVALID_STATE`, `INVALID_REQUEST`, `INTERNAL`) match the contract error code matrix (`BEAD_NOT_FOUND`, `INVALID_REQUEST`, `METHOD_NOT_ALLOWED`, `NOT_FOUND`, `INTERNAL_ERROR`)? Note `INTERNAL` vs `INTERNAL_ERROR` naming. [Conflict, Spec §Error Codes vs contracts/rest-api.md]
- [ ] CHK033 Does `/orchestrator/status` shape in US10 (`{build, schemaVersion, beadsVersion, online, serverTime, dolt}`) match data-model.md (`{running, mode}`)? **Known conflict.** [Conflict, Spec §US10 vs data-model.md]
- [ ] CHK034 Does the startup banner format in Assumptions (`muster listening on http://127.0.0.1:7766 (build=dev schemaVersion=1)`) match the quickstart example (`muster build=dev schemaVersion=1 addr=127.0.0.1:7766`)? [Conflict, Spec §Assumptions vs quickstart.md]
- [ ] CHK035 Do the coverage targets in SC-006/SC-007 (80% core, 70% api) match the plan's per-package targets (80% core / 80% services / 75% ws / 90% render+middleware / 70% api handlers)? [Conflict, Spec §SC-006..SC-007 vs plan.md]
- [ ] CHK036 Is the `internal/api/` module layout in spec §Technical Context (a single package) consistent with the plan's resource-subpackage layout (`internal/api/beads/`, `internal/api/stream/`, etc.)? [Conflict, Spec §Technical Context vs plan.md]
- [ ] CHK037 Is `internal/services/` in plan.md reflected in spec §Technical Context module layout? **Known gap.** [Conflict, Spec §Technical Context vs plan.md]
- [ ] CHK038 Are the WS event names consistent across documents — e.g., `bead.moved` (spec) vs no such event (contracts), and `comment.added` (spec) vs no such event (contracts)? [Conflict, Spec §FR-012 / §US6 / §US9 vs contracts/ws-events.md]

## Acceptance Criteria Quality

- [ ] CHK039 Is "renders identically to serving the files via `python3 -m http.server`" (US1 scenario 3) objectively verifiable, or is it a visual/subjective check? Quantify the test or restate. [Measurability, Spec §US1]
- [ ] CHK040 Is "<50ms latency" in SC-002 specified at a percentile, sample size, and warm/cold state? [Measurability, Spec §SC-002]
- [ ] CHK041 Is the SC-005 "<100ms" deadline measured (a) from REST handler return, (b) from store commit, (c) from request receipt? [Measurability, Spec §SC-005]
- [ ] CHK042 Are the acceptance scenarios for FR-019 (`?column=` filter) specified, including what an invalid column value returns (400 vs empty list)? [Completeness, Spec §FR-019]
- [ ] CHK043 Is the acceptance criterion for FR-017 (X-Request-ID on **all** responses, including 4xx/5xx and WS upgrade responses) testable? [Measurability, Spec §FR-017]
- [ ] CHK044 Is "build clean" or "tests pass" defined as a measurable success criterion, separate from coverage? [Gap]
- [ ] CHK045 Is there a measurable acceptance criterion for "the binary is self-contained" (e.g., `file ./muster | grep -v dynamic`, or `ldd ./muster` returns expected results)? [Measurability, Spec §FR-001]

## Scenario Coverage

- [ ] CHK046 Are alternate-flow requirements covered for `POST /beads` (e.g., creating with all optional fields supplied vs all defaulted)? [Coverage, Spec §US4]
- [ ] CHK047 Are exception-flow requirements covered for every endpoint? Every endpoint's "what fails" rows from the error matrix need a corresponding acceptance scenario in the spec. [Coverage, Gap]
- [ ] CHK048 Are recovery-flow requirements covered for WS disconnect/reconnect? E.g., "the client re-fetches state via GET /beads after reconnect" is documented in contracts but not in spec acceptance scenarios. [Coverage, Gap]
- [ ] CHK049 Are non-functional scenarios covered (cold start, concurrent client load, memory growth under sustained mutation)? [Coverage, Gap]
- [ ] CHK050 Are concurrent-client scenarios specified (e.g., "two WS clients connected, one mutates via REST, both receive the event")? US7 says "all receive the same events" — is there an acceptance scenario that pins this with N=2 clients? [Coverage, Spec §Edge Cases]
- [ ] CHK051 Are the requirements for behaviour when a WS client connects mid-broadcast specified? E.g., does it miss the event in flight, or is registration synchronous w.r.t. broadcast? [Coverage, Gap]

## Edge Case Coverage

- [ ] CHK052 Is the behaviour specified when `GET /beads?column=running&column=review` (repeated query param) is supplied? [Edge Case, Gap]
- [ ] CHK053 Is the behaviour specified when a UTF-8 multi-byte string in `title` (e.g., emoji, RTL text) is supplied? [Edge Case, Gap]
- [ ] CHK054 Is the behaviour specified when an integer field overflows `int` (e.g., `priority: 99999999999999`)? [Edge Case, Gap]
- [ ] CHK055 Is the behaviour specified when the same bead is mutated concurrently by two PATCH requests (last-write-wins? optimistic concurrency?)? [Edge Case, Gap]
- [ ] CHK056 Is the behaviour specified when a WS client sends an oversized frame or non-text (binary) frame? [Edge Case, Gap]
- [ ] CHK057 Is the behaviour specified when the embedded `ui/` directory is empty at runtime (e.g., the developer skipped `make ui-copy`)? Build-time vs runtime detection? [Edge Case, Gap]
- [ ] CHK058 Is the behaviour specified when `X-Request-ID` is supplied with control characters or excessive length? [Edge Case, Gap]
- [ ] CHK059 Are requirements specified for the `/orchestrator/status` response when DOLT is not configured (M0)? [Edge Case, Spec §US10]

## Non-Functional Requirements

- [ ] CHK060 Is a memory-budget requirement specified (e.g., "M0 holds 14 beads in <10 MB RSS")? [Gap]
- [ ] CHK061 Is a CPU-budget requirement specified for idle vs broadcast load? [Gap]
- [ ] CHK062 Are the security requirements documented (e.g., "M0 binds to 127.0.0.1 only, no external auth, no TLS")? Currently scattered between Assumptions and FR-002. [Clarity, Spec §FR-002 / §Assumptions]
- [ ] CHK063 Are logging-level requirements specified (DEBUG vs INFO vs WARN), or is the level fixed to INFO in M0? [Gap, Spec §Assumptions]
- [ ] CHK064 Are log-format requirements pinned (slog text handler) such that downstream parsers can rely on the format, or is it free-form? [Clarity, Spec §Assumptions]
- [ ] CHK065 Is the behaviour required when slog can't write to stderr (e.g., closed FD)? [Edge Case, Gap]
- [ ] CHK066 Is a request-rate ceiling specified (or explicitly disclaimed) for M0? [Gap]

## Dependencies & Assumptions

- [ ] CHK067 Is FR-020 ("System MUST use `go-chi/chi` and `coder/websocket`") an implementation choice that belongs in `plan.md`/`research.md` rather than the requirements section? [Anti-pattern, Spec §FR-020]
- [ ] CHK068 Is the version pinning of dependencies (chi v5.2.1, coder/websocket v1.8.13, uuid v1.6.0, testify v1.10.0) in scope for requirements, or is "any compatible version" acceptable? [Clarity, Spec §Technical Context]
- [ ] CHK069 Is the Go 1.26+ assumption validated against features actually used (e.g., does the code rely on a 1.26-specific feature)? [Assumption]
- [ ] CHK070 Is the assumption "Babel-in-browser, no UI build step" still valid given the embed surface and CSP considerations? [Assumption, Spec §Assumptions]
- [ ] CHK071 Are the cross-document references (`spec.md §3.1`, `DESIGN.md §1.11`, `data.jsx` TASKS) pinned to a specific commit or treated as living references? [Traceability, Spec §FR-004, §Key Entities]

## Ambiguities & Conflicts (summary)

- [ ] CHK072 Resolve the WS event-name conflict: standardise on either `bead.moved`/`comment.added` (spec) or fold them into `bead.updated` (contracts). [Conflict — see CHK023, CHK029, CHK038]
- [ ] CHK073 Resolve the dispatch semantics conflict: append-a-step (contracts) vs scheduled→running transition (spec/US8). [Conflict — see CHK030, CHK031]
- [ ] CHK074 Resolve the move request-shape conflict: `{toColumn}` (spec) vs `{column}` (contracts). [Conflict — see CHK026]
- [ ] CHK075 Resolve the orchestrator/status payload conflict: full daemon metadata (spec) vs `{running, mode}` stub (data-model). [Conflict — see CHK033]
- [ ] CHK076 Resolve the hello+ping/pong gap: either add them to the WS contract, or remove FR-013/FR-014 from the spec. [Conflict — see CHK024, CHK025]
- [ ] CHK077 Resolve the module-layout drift: spec §Technical Context says `internal/api/` flat; plan.md says resource subpackages + `internal/services/`. Pick one and update both. [Conflict — see CHK036, CHK037]
- [ ] CHK078 Resolve coverage-target drift between SC-006/SC-007 and plan.md's per-package matrix. [Conflict — see CHK035]

## Traceability

- [ ] CHK079 Is a requirement & acceptance criteria ID scheme established such that every test can reference the FR / SC / US it satisfies? [Traceability]
- [ ] CHK080 Are the cross-references between `spec.md`, `data-model.md`, `contracts/rest-api.md`, and `contracts/ws-events.md` maintained bidirectionally so a change in one surfaces a needed change in the others? [Traceability]
