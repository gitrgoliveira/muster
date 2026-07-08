---
description: "Task list for M6 — Skills & Constitution"
---

# Tasks: M6 — Skills & Constitution

**Input**: Design documents from `specs/007-m6-skills-constitution/` (spec.md, plan.md, research.md, data-model.md, contracts/, quickstart.md)
**Feature dir / bead**: `specs/007-m6-skills-constitution` · `muster-ep0`

**Tests**: TDD is **mandatory** (Constitution IV, NON-NEGOTIABLE). Every implementation task is preceded by a failing-test task — including across parallel groups (a test task is never in the same group as its impl). Every external-tool (`bd`) and network (URL import) integration is pinned by a **spike** (verify-before-building), then covered by a fake-on-`$PATH`/fake-HTTP unit test **and** a skip-gated real-binary/real-network integration test.

**Revisions**: post-`speckit.analyze` (C1/M1/M2/L1–L3 + security hardening), then post-**cross-model review** (Sonnet 5): closes F1 (TDD ordering), F2 (service-before-handler), F3 (claude MCP-config spike), F4 (`bd` label-read spike), F5/F9 (primed-memories persistence), F6 (summary-extraction algorithm), F7 (union wording), F8 (memory-key derivation), F10 (`defaultPromptFor` source).

## Format: `[ID] [P?] [Story] Description with file path`

- **[P]** = parallelizable (different files, no dependency on an incomplete task, and never a test paired with its own impl). Fleet fans out ≤3 `[P]` per `<!-- parallel-group -->`.
- **[Story]** label on user-story-phase tasks only.
- `<!-- sequential -->` precedes tasks that MUST run in order — shared-file edits (`orchestrator.go`, `assemble.go`, `router.go`, `main.go`, `event.go`, `errors.go`, `mapper.go`, `registry.go`, `Makefile`, `go.mod`) AND test→impl and service→handler dependencies.

## Conventions

- Go single binary. Layered `core → store → services → api` + `orchestrator`. New persistent state = local files under `<musterDir>` (default `~/.muster`); memories via `bd`; nothing new in Dolt/around `bd`.
- Keep M0–M5 REST/WS surface + suites green (Principle V). Only additive routes/events/fields.

---

## Phase 1: Setup — deps, scaffolding & verify-before-building spikes

**Purpose**: pin every external contract against reality *before* code is written (Constitution "verify assumptions empirically").

<!-- sequential --> (go.mod; spikes write stub comment-blocks)
- [ ] T001 Promote `gopkg.in/yaml.v3` from indirect → direct dependency (`go.mod`; `go mod tidy`).
- [ ] T002 [US5] SPIKE `bd` memories: run `bd remember [--key K] "v"`, `bd recall <k>`, `bd forget <k>`, `bd memories [q] [--json]` against a scratch beads dir; record exact flags/args/JSON shapes **and whether keyless `bd remember` returns a stable retrievable key** (F8) in a comment block atop `internal/store/bdshell/memories.go` (stub).
- [ ] T003 [US4] SPIKE `bd` labels: run `bd show <id> --json` (and/or `bd label` subcommands) against a scratch beads dir; confirm labels appear, record the JSON key + command shape, and note any N+1 concern for dispatch-time reads; write it in a comment block atop `internal/store/bdshell/labels.go` (stub). **[F4]**
- [ ] T004 [US4] SPIKE claude MCP config: determine how to locate the `claude` CLI's own MCP config (try `claude config ...` / known paths); record the resolved path + the JSON schema for MCP-server entries in a comment block atop `internal/orchestrator/mcpcheck.go` (stub). If not stably discoverable, note the `--claude-config-path` fallback. **[F3]**

<!-- parallel-group: 1 (max 3 concurrent) -->
- [ ] T005 [P] Scaffold package dirs with a `doc.go` each: `internal/skills/`, `internal/api/constitution/`, `internal/api/skills/`, `internal/api/memories/`.
- [ ] T006 [P] Add placeholder coverage-gate entries to the Makefile `thresholds` map for the new packages (tighten in Polish).

**Checkpoint**: deps + scaffolding + all three external contracts pinned.

---

## Phase 2: Foundational (Blocking Prerequisites)

<!-- sequential --> (shared files: errors.go, event.go)
- [ ] T007 Add error codes `SKILL_READONLY` (403), `SKILL_ID_CONFLICT` (409), `SKILL_INVALID_ID` (400), `BD_UNAVAILABLE` (502) to `internal/api/render/errors.go` (+ `httpStatusForCode`).
- [ ] T008 Add WS event types `constitution.changed` + `runlog.warning` to `internal/ws/event.go`; add additive `omitempty` `Version *int` to `Frame`. (No existing type/field changed.)

<!-- parallel-group: 2 (max 3 concurrent) — tests only, distinct new files -->
- [ ] T009 [P] Test: `<musterDir>` resolution (default `~/.muster`, `--muster-dir`/`MUSTER_DIR` override, normalization) — `internal/config/muster_dir_test.go`.
- [ ] T010 [P] Test: `skills.ValidateID` — accepts `^[a-z0-9][a-z0-9._-]*$`; REJECTS empty, separators, `.`/`..`, traversal — `internal/skills/id_test.go`. **[Security — path traversal]**
- [ ] T011 [P] Test: `ConstitutionProvider`/`SkillProvider` interfaces are nil-safe — `internal/orchestrator/providers_test.go`.

<!-- sequential --> (impls; T014 edits shared orchestrator.go)
- [ ] T012 Implement `<musterDir>` in `internal/config/muster_dir.go` (mirrors `DefaultWorktreesDir`, `internal/config/orchestrator.go:105-111`); green.
- [ ] T013 Implement `skills.ValidateID` in `internal/skills/id.go` (sole gate for any id → path or label resolution); green.
- [ ] T014 Define `ConstitutionProvider` (`Snapshot() (md string, v int)`) + `SkillProvider` (`Resolve(ids []string) ([]skills.Skill, []string)`) + nil-safe defaults on `orchestrator.Config` (`orchestrator.go:229`); green.
- [ ] T015 Transcribe the per-mode default step prompts from `prototype/handoff/spec.md §9/§6.1` into a Go table in `internal/orchestrator/prompts.go` (`defaultPromptFor(mode)` source of truth — NOT a "UI table"); exported for assembly. **[F10]**

**Checkpoint**: error codes, WS events, musterDir, id-guard, provider seams, and the prompt table exist.

---

## Phase 3: User Story 1 — Prompt assembly replaces the placeholder (P1) 🎯 MVP

**Goal / Independent test**: dispatch with a non-empty constitution + fake skills + a 2-step chain; `.muster-prompt-<idx>.txt` matches the template byte-for-byte (SC-001); step 1 carries a one-line summary of step 0; launch/session/advance/loopback/capacity/idempotency/quota UNCHANGED.

<!-- sequential --> (all touch orchestrator.go / runlog.go / assemble.go)
- [ ] T016 [US1] Test: per-step output retention — tail bounded to **last 8 KiB / 100 lines** captured into `run.stepSummaries[idx]` (run-mutex guarded); failed steps retained + labelled — `internal/orchestrator/runlog_retention_test.go`. **[F6 — bounds pinned]**
- [ ] T017 [US1] Implement per-step tail capture: add `stepSummaries map[int]string` to `Run`, tee the tail in `runlogStreamer` (`runlog.go:29-59`); green.
- [ ] T018 [US1] Test: `defaultPromptFor(mode)` returns the pinned table string per mode (from T015); `resolvePrompt(PromptRef, mode)` returns stored override else default — `internal/orchestrator/assemble_prompt_test.go`.
- [ ] T019 [US1] Test: `assemblePrompt` produces the exact handoff-§9 template byte-for-byte, incl. degenerate case (FR-005) + synthesized-stage prefix (§6.1). The **one-line summary rule is pinned**: the last non-blank line of `stepSummaries[k]`, truncated to 200 chars — `internal/orchestrator/assemble_test.go`. **[F6]**
- [ ] T020 [US1] Test: a `plan→build` chain — step 1's prompt contains the pinned one-line summary of a completed step 0; a *failed* step 0 is included + labelled — `internal/orchestrator/assemble_multistep_test.go`.
- [ ] T021 [US1] Implement `internal/orchestrator/assemble.go` (`assemblePrompt` + `resolvePrompt`, using `prompts.go` + providers); green (T018–T020).
- [ ] T022 [US1] Replace `buildPrompt(...)` at `orchestrator.go:861` with `assemblePrompt(...)`; delete `buildPrompt` (`:1252-1259`). All launches funnel through `doLaunch` — one swap.
- [ ] T023 [US1] Update M2/M4 suites to the new prompt shape while asserting non-prompt behavior (launch/session/advance/loopback/capacity/idempotency/quota) unchanged (`internal/orchestrator/*_test.go`).

**Checkpoint**: US1 MVP.

---

## Phase 4: User Story 2 — Constitution CRUD, versioned & merged (P1)

**Goal / Independent test**: `PUT` → `GET` shows bumped version → next dispatch header is new markdown+version; running step unaffected; survives restart.

<!-- sequential --> (service test → impl → wiring; F1: test precedes impl)
- [ ] T024 [US2] Test: `ConstitutionService` — fresh ⇒ `{markdown:"",version:0}`; `Set` writes `<musterDir>/constitution.md` **atomically (temp+rename)**, bumps 0→1→…, emits `constitution.changed`; `Snapshot()` atomic under concurrent `Set` (`-race`); startup reload; corrupt file ⇒ empty-default + warning — `internal/services/constitution_test.go`. **[Hardening]**
- [ ] T025 [US2] Implement `internal/services/constitution.go` (RWMutex, atomic temp+rename, monotonic version, publisher, corrupt-file fallback); green.
- [ ] T026 [US2] Test: `internal/api/constitution` handlers — `GET` (fresh ⇒ version 0, not 404); `PUT` validate + body-limit + bumped version; oversize/malformed ⇒ 400 — `internal/api/constitution/handlers_test.go`. **[F1 — test before impl]**
- [ ] T027 [US2] Implement `internal/api/constitution/{dto.go,handlers.go}` (thin; `middleware.BodyLimit` on PUT); green.

<!-- sequential --> (shared: main.go, router.go)
- [ ] T028 [US2] Wire `ConstitutionService` as orchestrator `ConstitutionProvider` (`cmd/muster/main.go` → `orchestrator.Config`).
- [ ] T029 [US2] Register `GET`/`PUT /api/v1/constitution` in `internal/api/router.go`.
- [ ] T030 [US2] Integration test: dispatch after `PUT` ⇒ header = new markdown+version; step running at PUT unaffected (FR-009) — `internal/orchestrator/assemble_constitution_test.go`.

**Checkpoint**: US1+US2 = constitution merged into real dispatches.

---

## Phase 5: User Story 3 — Skill registry: built-ins + import/remove (P2)

**Goal / Independent test**: `GET /skills` returns built-ins with zero imports; `POST {url}` adds one (survives restart); `DELETE` imported works; delete built-in ⇒ 403.

<!-- parallel-group: 3 (max 3) — tests only, distinct files -->
- [ ] T031 [P] [US3] Test: `Skill` type + front-matter parse (valid, missing fields, empty PromptStub; id via `ValidateID`) — `internal/skills/skill_test.go`.
- [ ] T032 [P] [US3] Test: imported file store CRUD over `<musterDir>/skills/<id>.md` (create/list/delete, reload; rejects traversal id before FS touch; concurrent same-id CRUD serialized, no half/orphan file) — `internal/skills/store_test.go`. **[Hardening]**
- [ ] T033 [P] [US3] Test: URL import — `https`-only (+ loopback `http`), 10 s timeout, 1 MiB cap, malformed/oversize ⇒ error + no partial reg — `internal/skills/import_test.go`.

<!-- parallel-group: 4 (max 3) — impls in distinct files -->
- [ ] T034 [P] [US3] Implement `internal/skills/skill.go` (`Skill` §3.3 + front-matter; id via `ValidateID`); green T031.
- [ ] T035 [P] [US3] Implement `internal/skills/builtin.go` (`//go:embed builtin` FS + parse) + `internal/skills/builtin/` seed catalog. **[M1 — separate from registry.go]**
- [ ] T036 [P] [US3] Implement `internal/skills/import.go` (bounded, safe fetch); green T033.

<!-- sequential --> (store.go + registry.go)
- [ ] T037 [US3] Implement `internal/skills/store.go` (imported-file CRUD, `ValidateID` gate, atomic write + per-id serialization, fsnotify reload); green T032.
- [ ] T038 [US3] Implement `internal/skills/registry.go` (built-in ∪ imported, categories, CRUD, `SKILL_ID_CONFLICT` on built-in-id, upsert on imported-id); green.
- [ ] T039 [US3] Test: SSRF/abuse hardening for URL import — reject `file://`, non-loopback `http://`, link-local/metadata (`169.254.169.254`), redirect landing on disallowed scheme/host — `internal/skills/import_ssrf_test.go`. **[Hardening]**
- [ ] T040 [US3] Skip-gated real-network integration test for URL import — `internal/skills/import_integration_test.go`.

<!-- sequential --> (F2: service before handler; then shared router/main)
- [ ] T041 [US3] Test + impl: `internal/services/skills.go` (`SkillRegistry` service; `SKILL_READONLY`/`SKILL_ID_CONFLICT`/`SKILL_INVALID_ID` → `*ServiceError`) — `skills_test.go` + `skills.go`. **[F2 — precedes handler]**
- [ ] T042 [US3] Test + impl: `internal/api/skills/{dto.go,handlers.go}` — `GET /skills`, `GET /skills/categories`, `POST /skills` (body-limit), `DELETE /skills/{id}` (403 built-in / 404 unknown / 400 invalid) — imports the T041 service — `handlers_test.go` + handlers.
- [ ] T043 [US3] Register skills routes in `internal/api/router.go`; construct `SkillRegistry` in `cmd/muster/main.go`.

**Checkpoint**: registry browsable, mutable, traversal-safe.

---

## Phase 6: User Story 4 — Skill loadout in the prompt + MCP verify (P2)

**Goal / Independent test**: label a bead `skill:repo-grep` → prompt lists it; `skill:does-not-exist` → dispatch succeeds + `runlog.warning`; MCP server absent → warning, dispatch proceeds.

<!-- sequential --> (label read path — uses T003 spike; C1 closure)
- [ ] T044 [US4] Test: `bdshell` label-read verb vs **fake `bd` on `$PATH`** (argv per T003 spike; flags before `--`; non-zero ⇒ `*CLIError`) — `internal/store/bdshell/labels_test.go`. **[C1]**
- [ ] T045 [US4] Implement `internal/store/bdshell/labels.go` (per T003) + add `Labels []string` to `internal/store/issue.go`; green T044.
- [ ] T046 [US4] Skip-gated **real-`bd`** integration test for label read — `internal/store/bdshell/labels_integration_test.go`. **[C1]**
- [ ] T047 [US4] Test: `IssueToBead` splits labels → `Bead.Labels` vs `skill:*` → `Bead.Skills` (prefix stripped; malformed/traversal ids ignored+warned via `ValidateID`) — `internal/services/mapper_test.go`.
- [ ] T048 [US4] Implement the split in `internal/services/mapper.go:29-30`; populate labels at dispatch/assembly time; green T047.

<!-- sequential --> (DispatchRequest + assembly, shared orchestrator files)
- [ ] T049 [US4] Test: loadout = de-duplicated **union** `Bead.Skills ∪ Step.Skills` — a bead `skill:a` + step `["b"]` ⇒ `["a","b"]` (NOT `["b"]`); "Skills loaded" lists each name + PromptStub first line — `internal/orchestrator/assemble_skills_test.go`. **[F7 — asserts union, not override]**
- [ ] T050 [US4] Thread `Step.Skills` via optional `DispatchRequest.Skills`; resolve union via `SkillProvider` in `assemblePrompt`; wire real `SkillRegistry` as `orchestrator.Config.SkillProvider` in `cmd/muster/main.go`; green T049.

<!-- parallel-group: 5 (max 3) — warning tests, distinct files -->
- [ ] T051 [P] [US4] Test: unresolvable skill id ⇒ skipped + `runlog.warning` (never blocks, never silent) — `internal/orchestrator/warn_skill_test.go`.
- [ ] T052 [P] [US4] Test: MCP verify (per T004 spike) — a skill `MCPServers` entry absent from a fake claude MCP config ⇒ `runlog.warning`, dispatch proceeds; unreadable/absent config ⇒ "not found" warning — `internal/orchestrator/warn_mcp_test.go`.

<!-- sequential --> (shared warning path; mcpcheck.go from T004)
- [ ] T053 [US4] Implement `runlog.warning` for unresolvable skills; green T051.
- [ ] T054 [US4] Implement `internal/orchestrator/mcpcheck.go` best-effort MCP probe (per T004) + warning; MUST NOT spawn/manage MCP servers (FR-022); green T052.

**Checkpoint**: skills wired end-to-end.

---

## Phase 7: User Story 5 — Memories CRUD as a thin `bd` wrapper (P3)

**Goal / Independent test**: `POST`→`GET ?q=`→`DELETE` (fake-`bd`) + skip-gated real-`bd`; `POST /prime` ⇒ next prompt has "Primed memories" (survives restart); any `bd` failure ⇒ typed error, not empty-list.

<!-- sequential --> (bdshell/memories.go from T002 spike)
- [ ] T055 [US5] Test: `bdshell` `Remember/Recall/Forget/Memories` vs fake `bd` (argv+JSON per T002; key/value with leading `-` passed after `--`; auto-key behavior per T002/F8; non-zero ⇒ `*CLIError`) — `internal/store/bdshell/memories_test.go`. **[Hardening + F8]**
- [ ] T056 [US5] Implement `internal/store/bdshell/memories.go` (per T002; `--` before positionals); green T055.
- [ ] T057 [US5] Skip-gated real-`bd` integration test — `internal/store/bdshell/memories_integration_test.go`.

<!-- sequential --> (F2: service before handler)
- [ ] T058 [US5] Test + impl: `internal/services/memories.go` (`MemoriesService`; any `bd` failure ⇒ `BD_UNAVAILABLE` `*ServiceError`, never empty-list) — `memories_test.go` + `memories.go`. **[F2 — precedes handler]**
- [ ] T059 [US5] Test + impl: `internal/api/memories/{dto.go,handlers.go}` — `GET`(`?q=`)/`POST`(`{key?,value}`)/`DELETE /{key}` + `POST /prime {beadID}`; imports T058 service — `handlers_test.go` + handlers.

<!-- sequential --> (assemble.go + router + main)
- [ ] T060 [US5] Test + impl: **persist** primed snapshot to `<musterDir>/primed/<beadID>.json` (survives restart, disposable) and fold it into `assemblePrompt` as a "Primed memories" section; prime-before-dispatch and prime-after-dispatch both apply at the *next* launch — `internal/orchestrator/assemble_memories_test.go` + `internal/services/memories.go` + assemble.go. **[F5/F9 — persisted, not in-memory]**
- [ ] T061 [US5] Register memories routes in `internal/api/router.go`; construct `MemoriesService` in `cmd/muster/main.go`.

**Checkpoint**: all 5 user stories complete.

---

## Phase 8: Constitution-Compliance Verification & Polish

<!-- parallel-group: 6 (max 3) — distinct assertion files -->
- [ ] T062 [P] Additive-surface regression test: full M0–M5 route + WS event set unchanged; only M6 additions present (SC-008, FR-027) — `internal/api/router_additive_test.go`.
- [ ] T063 [P] Storage-boundary assertion (FR-028): no new Dolt table/schema; skills/constitution never call the bdshell writer (only memories do) — `internal/skills/no_dolt_test.go` / Makefile guard.
- [ ] T064 [P] Thin-handler assertion (FR-030): new handlers delegate to services, no business logic — test + reviewer checklist.

<!-- sequential -->
- [ ] T065 Set real per-package coverage thresholds in the Makefile `thresholds` map for all new packages (SC-007); `make cover-check`.
- [ ] T066 Update README Flags/API/WS tables with `--muster-dir`, new routes, `constitution.changed`/`runlog.warning` (additive docs).
- [ ] T067 Run `quickstart.md` end-to-end vs a built binary; fix drift.
- [ ] T068 Full gate: `make test` (`-race`) green, `make lint` clean, `make cover-check` passes; M0–M5 suites green.

---

## Dependencies & Story Completion Order

- **Setup (spikes T002/T003/T004) → Foundational** block everything. `ValidateID` (T010/T013) is foundational for US3 store + US4 label parsing. The prompt table (T015) is foundational for US1 assembly.
- **US1** → **US2** (fills `ConstitutionProvider`) → **US3** → **US4** (needs US1 assembly + US3 registry + T003 label spike + T004 MCP spike) → **US5** (needs T002 spike + US1 assembly).
- Every service task precedes its handler task (T041→T042, T058→T059). Every test precedes its impl (no test+impl in one group). Shared-file tasks are sequential.

## Parallel Execution Opportunities

- **Group 1** (T005–T006): scaffolding.
- **Group 2** (T009–T011): foundational *tests* (distinct new files).
- **Group 3** (T031–T033) → **Group 4** (T034–T036): skill tests, then skill impls (skill.go/builtin.go/import.go).
- **Group 5** (T051–T052): the two warning tests.
- **Group 6** (T062–T064): compliance assertions.

## Implementation Strategy

- **MVP = Phase 1–3 (US1)**; **Increment 2 = US2**; **Increment 3 = US3+US4**; **Increment 4 = US5**. Each independently testable.

**Total: 68 tasks** — Setup 6 · Foundational 9 · US1 8 · US2 7 · US3 13 · US4 11 · US5 7 · Compliance/Polish 7.

## Security-Hardening & Spike Summary

| Concern | Guard / Spike | Tasks |
|---|---|---|
| Path traversal via skill `id` | `skills.ValidateID` before any id→path/label | T010, T013, T032, T037, T047 |
| SSRF via URL import | scheme allowlist + loopback-only http + link-local reject + redirect re-check | T033, T036, T039 |
| Torn/crash-torn constitution file | atomic temp+rename; RWMutex snapshot; corrupt fallback | T024, T025 |
| `bd` arg injection (leading `-`) | `--` separator | T055, T056 |
| Concurrent skill CRUD | per-id serialization + atomic write | T032, T037 |
| Primed memories lost on restart (F5) | persist to `<musterDir>/primed/<beadID>.json` | T060 |
| `bd` label/memory shape unverified (F4/F8) | spikes before wrapper code | T002, T003 |
| claude MCP-config path unknown (F3) | spike before probe code | T004 |
