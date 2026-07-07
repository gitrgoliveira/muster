---
description: "Task list for M6 — Skills & Constitution"
---

# Tasks: M6 — Skills & Constitution

**Input**: Design documents from `specs/007-m6-skills-constitution/` (spec.md, plan.md, research.md, data-model.md, contracts/, quickstart.md)
**Feature dir / bead**: `specs/007-m6-skills-constitution` · `muster-ep0`

**Tests**: TDD is **mandatory** (Constitution IV, NON-NEGOTIABLE). Every implementation task is preceded by a failing-test task. External-tool/URL paths get a fake-on-`$PATH` / fake-HTTP unit test **and** a skip-gated real-binary/real-network integration test.

## Format: `[ID] [P?] [Story] Description with file path`

- **[P]** = parallelizable (different files, no dependency on an incomplete task). The fleet orchestrator may fan out up to **3** `[P]` tasks per `<!-- parallel-group -->` concurrently.
- **[Story]** label on user-story-phase tasks only.
- `<!-- sequential -->` precedes tasks that MUST run in order — notably any tasks editing a **shared file** (`internal/orchestrator/orchestrator.go`, `internal/api/router.go`, `cmd/muster/main.go`, `internal/ws/event.go`, `internal/api/render/errors.go`, `internal/services/mapper.go`, `Makefile`, `go.mod`).

## Conventions

- Go single binary. Layered `core → store → services → api` + `orchestrator`. New persistent state = local files under `<musterDir>` (default `~/.muster`); memories via `bd`; nothing new in Dolt/around `bd`.
- Keep M0–M5 REST/WS surface + suites green (Principle V). Only additive routes/events/fields.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Dependency + scaffolding prerequisites. The `bd` spike lands here so US5's wrapper is built against verified reality (Constitution "verify before building").

<!-- sequential -->
- [ ] T001 Promote `gopkg.in/yaml.v3` from indirect → direct dependency (`go.mod`; `go mod tidy`) for skill front-matter parsing.
- [ ] T002 [US5] SPIKE: run real `bd remember`/`bd recall`/`bd forget`/`bd memories [--json]` against a scratch beads dir; record exact flags, args, and JSON output shapes in a comment block at the top of `internal/store/bdshell/memories.go` (new file, stub) — pins the wrapper contract before implementation.

<!-- parallel-group: 1 (max 3 concurrent) -->
- [ ] T003 [P] Scaffold package dirs with a `doc.go` each: `internal/skills/`, `internal/api/constitution/`, `internal/api/skills/`, `internal/api/memories/`.
- [ ] T004 [P] Add placeholder coverage-gate entries to the Makefile `thresholds` map for `internal/skills`, `internal/api/constitution`, `internal/api/skills`, `internal/api/memories` (tighten to real values in Polish).

**Checkpoint**: deps + scaffolding + `bd` contract ready.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: shared, additive infrastructure every story depends on. **No user-story work starts until this completes.**

<!-- sequential --> (shared files: errors.go, event.go)
- [ ] T005 Add error codes `CodeSkillReadonly = "SKILL_READONLY"`, `CodeSkillIDConflict = "SKILL_ID_CONFLICT"`, `CodeBDUnavailable = "BD_UNAVAILABLE"` to `internal/api/render/errors.go` (+ their `httpStatusForCode` mappings: 403 / 409 / 502).
- [ ] T006 Add WS event types `EventConstitutionChanged = "constitution.changed"` and `EventRunlogWarning = "runlog.warning"` to `internal/ws/event.go`; add additive `omitempty` `Version *int` to `Frame` for the constitution event. (No existing type/field changed.)

- [ ] T007 [P] Test: `<musterDir>` resolution — default `~/.muster`, `--muster-dir`/`MUSTER_DIR` override, path normalization — in `internal/config/muster_dir_test.go`.
- [ ] T008 Implement `<musterDir>` resolution in `internal/config/muster_dir.go` (mirrors `DefaultWorktreesDir` at `internal/config/orchestrator.go:105-111`); make it green.

<!-- sequential --> (orchestrator.Config is a shared file with US1)
- [ ] T009 [P] Test: `ConstitutionProvider` + `SkillProvider` interfaces are nil-safe (a nil/absent provider yields empty constitution / empty loadout without panic) in `internal/orchestrator/providers_test.go`.
- [ ] T010 Define `ConstitutionProvider` (`Snapshot() (markdown string, version int)`) and `SkillProvider` (`Resolve(ids []string) ([]skills.Skill, []string /*unresolved*/)`) interfaces + nil-safe defaults on `orchestrator.Config` (`internal/orchestrator/orchestrator.go:229`); make green.

**Checkpoint**: error codes, WS events, musterDir, and assembly provider seams exist — stories can begin.

---

## Phase 3: User Story 1 — Prompt assembly replaces the placeholder (P1) 🎯 MVP

**Goal**: every dispatched step's prompt is the full assembled template (constitution header, step/provider framing, skills, bead ticket, earlier-step summaries, resolved step prompt) instead of the two-line `buildPrompt`.

**Independent test**: dispatch with a non-empty constitution + fake skills + a 2-step chain; assert `.muster-prompt-<idx>.txt` matches the template byte-for-byte (SC-001), and step 1 carries a one-line summary of step 0. Assert launch/session/advance/loopback/capacity/idempotency/quota UNCHANGED.

<!-- sequential --> (all touch orchestrator.go / runlog.go)
- [ ] T011 [P] [US1] Test: per-step output retention — a bounded tail of each step's pane stream is captured into `run.stepSummaries[idx]`, guarded by the run mutex; failed steps are retained + labelled — `internal/orchestrator/runlog_retention_test.go`.
- [ ] T012 [US1] Implement per-step tail capture: add `stepSummaries map[int]string` to `Run` and tee the tail in `runlogStreamer` (`internal/orchestrator/runlog.go:29-59`, `orchestrator.go` Run struct); make green.
- [ ] T013 [US1] Test: `defaultPromptFor(mode)` returns a non-empty default per supported mode; `resolvePrompt(PromptRef, mode)` returns a stored override when present, else the default — `internal/orchestrator/assemble_prompt_test.go`.
- [ ] T014 [US1] Test: `assemblePrompt` produces the exact handoff-§9 template (byte-verifiable), incl. degenerate case (empty constitution/skills/history ⇒ well-formed, FR-005) and the synthesized-stage prefix layering (FR/§6.1) — `internal/orchestrator/assemble_test.go`.
- [ ] T015 [US1] Implement `internal/orchestrator/assemble.go`: `assemblePrompt(...)` + `defaultPromptFor` table + `resolvePrompt`; reads providers from `orchestrator.Config`; make T013/T014 green.
- [ ] T016 [US1] Replace the `buildPrompt(req.BeadTitle, req.BeadDesc)` call at `internal/orchestrator/orchestrator.go:861` with `assemblePrompt(...)`; delete `buildPrompt` (`:1252-1259`). All launches (step 0, admission, advance, loopback) funnel through `doLaunch` — one swap.
- [ ] T017 [US1] Update M2/M4 suites to assert the new assembled-prompt shape while asserting every non-prompt-content behavior (launch, session naming, advance/loopback, capacity, idempotency, quota) is unchanged (`internal/orchestrator/*_test.go`).

**Checkpoint**: US1 is a shippable MVP — assembly works with empty/fake providers; real constitution/skills layer in next.

---

## Phase 4: User Story 2 — Constitution CRUD, versioned & merged (P1)

**Goal**: `GET`/`PUT /api/v1/constitution` round-trips a single versioned markdown file merged into every subsequent dispatch.

**Independent test**: `PUT` new markdown → `GET` shows incremented version → next dispatch's prompt header is the new markdown+version; a step already running is unaffected; survives restart.

<!-- sequential --> (constitution service + its wiring)
- [ ] T018 [P] [US2] Test: `ConstitutionService` — fresh install ⇒ `{markdown:"", version:0}`; `Set` overwrites `<musterDir>/constitution.md`, bumps version (0→1→…), updates `UpdatedAt`, emits `constitution.changed`; `Snapshot()` is atomic (markdown+version together) under concurrent `Set`; startup reload from disk — `internal/services/constitution_test.go`.
- [ ] T019 [US2] Implement `internal/services/constitution.go`: `RWMutex`-guarded struct, file persistence, monotonic version, `Publisher` for `constitution.changed`; make green.
- [ ] T020 [US2] Wire `ConstitutionService` as the orchestrator's `ConstitutionProvider` (construct in `cmd/muster/main.go`, pass into `orchestrator.Config` at `main.go:355-366`).

<!-- parallel-group: 2 (max 3 concurrent) -->
- [ ] T021 [P] [US2] Test: `internal/api/constitution` handlers — `GET` returns `{markdown,version,updatedAt}` (fresh ⇒ version 0, not 404); `PUT` validates + body-limit + returns bumped version; oversize/malformed ⇒ `400 INVALID_REQUEST` — `internal/api/constitution/handlers_test.go`.
- [ ] T022 [P] [US2] Implement `internal/api/constitution/{dto.go,handlers.go}` (thin: validate→service→render), using `middleware.BodyLimit` on `PUT`.

<!-- sequential --> (router.go shared)
- [ ] T023 [US2] Register constitution routes in `internal/api/router.go` (`GET`/`PUT /api/v1/constitution`); thread the service via `NewRouter`/config-struct.
- [ ] T024 [US2] Integration test: dispatch after a `PUT` ⇒ assembled prompt header = new markdown + version; a step running at PUT time is unaffected (FR-009) — `internal/orchestrator/assemble_constitution_test.go`.

**Checkpoint**: US1+US2 = constitution merged into real dispatches (the spec's core value).

---

## Phase 5: User Story 3 — Skill registry: built-ins + import/remove (P2)

**Goal**: list built-in + imported skills, import from URL, delete imported (built-ins read-only).

**Independent test**: `GET /skills` returns built-ins with zero imports; `POST /skills {url}` adds one (survives restart); `DELETE` an imported id works; deleting a built-in ⇒ `403 SKILL_READONLY`.

<!-- parallel-group: 3 (max 3 concurrent) -->
- [ ] T025 [P] [US3] Test: `Skill` type + front-matter parse (valid, missing fields, empty PromptStub) — `internal/skills/skill_test.go`.
- [ ] T026 [P] [US3] Test: imported file store CRUD over `<musterDir>/skills/<id>.md` (create/list/delete, reload on change) — `internal/skills/store_test.go`.
- [ ] T027 [P] [US3] Test: URL import — `https`-only (+ `http` loopback), 10 s timeout, 1 MiB cap, malformed/oversize ⇒ error + no partial registration (fake-HTTP) — `internal/skills/import_test.go`.

<!-- parallel-group: 4 (max 3 concurrent) -->
- [ ] T028 [P] [US3] Implement `internal/skills/skill.go` (`Skill` struct §3.3 + YAML front-matter parse); make T025 green.
- [ ] T029 [P] [US3] Implement `internal/skills/builtin/` seed catalog (≥ a few read-only `.md` skills) + `//go:embed builtin` FS loader in `internal/skills/registry.go`.
- [ ] T030 [P] [US3] Implement `internal/skills/import.go` (bounded, safe URL fetch); make T027 green.

<!-- sequential --> (registry.go builds on the above)
- [ ] T031 [US3] Implement `internal/skills/registry.go`: built-in ∪ imported union, categories, CRUD, `SKILL_ID_CONFLICT` on built-in-id collision, upsert on imported-id match; make T026 green.
- [ ] T032 [US3] Skip-gated real-network integration test for URL import (skips when offline) — `internal/skills/import_integration_test.go`.

<!-- parallel-group: 5 (max 3 concurrent) -->
- [ ] T033 [P] [US3] Test + impl: `internal/services/skills.go` (`SkillRegistry` service wrapping `internal/skills`; `SKILL_READONLY`/`SKILL_ID_CONFLICT` → `*ServiceError`) — `skills_test.go` + `skills.go`.
- [ ] T034 [P] [US3] Test + impl: `internal/api/skills/{dto.go,handlers.go}` — `GET /skills`, `GET /skills/categories`, `POST /skills` (body-limit), `DELETE /skills/{id}` (403 built-in / 404 unknown) — `handlers_test.go` + handlers.

<!-- sequential --> (router.go + main.go shared)
- [ ] T035 [US3] Register skills routes in `internal/api/router.go`; construct `SkillRegistry` service in `cmd/muster/main.go`.

**Checkpoint**: registry is browsable/mutable; not yet threaded into assembly (US4).

---

## Phase 6: User Story 4 — Skill loadout in the prompt + MCP verify (P2)

**Goal**: a bead/step's `skill:<id>` selection resolves into the assembled prompt's "Skills loaded" section; unresolvable skills / missing MCP servers warn, never block.

**Independent test**: label a bead `skill:repo-grep`; dispatch ⇒ prompt lists it with its PromptStub first line; a `skill:does-not-exist` label ⇒ dispatch succeeds + `runlog.warning`; an MCP server absent from agent config ⇒ warning, dispatch proceeds.

<!-- sequential --> (issue.go, mapper.go, bdshell — label read path)
- [ ] T036 [US4] Test: `store.Issue.Labels` populated + `IssueToBead` splits labels into `Bead.Labels` vs `skill:*` → `Bead.Skills` (prefix stripped; malformed `skill:`/`skill:a:b` ignored+warn) — `internal/services/mapper_test.go`.
- [ ] T037 [US4] Add `Labels []string` to `internal/store/issue.go`; add a `bdshell` label-read verb (`bd`-CLI read of a bead's labels); populate at dispatch/assembly time.
- [ ] T038 [US4] Implement the label split in `internal/services/mapper.go:29-30`; make T036 green.

<!-- sequential --> (DispatchRequest + assembly, shared orchestrator files)
- [ ] T039 [US4] Test: effective loadout = de-duplicated union `Bead.Skills ∪ Step.Skills`; "Skills loaded" lists each resolved skill's name + PromptStub first line — `internal/orchestrator/assemble_skills_test.go`.
- [ ] T040 [US4] Thread `Step.Skills` via an optional `DispatchRequest.Skills` field; resolve the union through the `SkillProvider` in `assemblePrompt`; wire the real `SkillRegistry` as `orchestrator.Config.SkillProvider` in `cmd/muster/main.go`; make T039 green.

<!-- parallel-group: 6 (max 3 concurrent) -->
- [ ] T041 [P] [US4] Test: unresolvable skill id ⇒ skipped + `runlog.warning` (never blocks dispatch, never silent) — `internal/orchestrator/warn_skill_test.go`.
- [ ] T042 [P] [US4] Test: best-effort MCP verification — a skill's `MCPServers` entry absent from a fake agent MCP config ⇒ `runlog.warning`, dispatch proceeds; unreadable config treated as "not found" — `internal/orchestrator/warn_mcp_test.go`.

<!-- sequential --> (both emit via the shared warning path in orchestrator)
- [ ] T043 [US4] Implement `runlog.warning` emission for unresolvable skills (make T041 green).
- [ ] T044 [US4] Implement best-effort, non-blocking MCP-config probe + warning (make T042 green); MUST NOT spawn/manage MCP servers (FR-022).

**Checkpoint**: skills fully wired end-to-end into dispatch.

---

## Phase 7: User Story 5 — Memories CRUD as a thin `bd` wrapper (P3)

**Goal**: list/search/upsert/delete memories + prime a bead — all via `bd` (contract pinned by T002 spike).

**Independent test**: `POST` → appears in `GET ?q=` → `DELETE` removes it (fake-`bd` round-trip) + a skip-gated real-`bd` run; `POST /memories/prime` ⇒ that bead's next prompt has a "Primed memories" section; any `bd` failure ⇒ typed error, not empty-list.

<!-- sequential --> (bdshell/memories.go from the T002 spike)
- [ ] T045 [US5] Test: `bdshell` verbs `Remember/Recall/Forget/Memories` against a fake `bd` on `$PATH` (argv + JSON per T002); non-zero exit ⇒ `*CLIError` — `internal/store/bdshell/memories_test.go`.
- [ ] T046 [US5] Implement `internal/store/bdshell/memories.go` verbs (following `Create`/`AppendNote` + `Run`/`RunJSON` patterns); make green.
- [ ] T047 [US5] Skip-gated real-`bd` integration test for the memories round-trip — `internal/store/bdshell/memories_integration_test.go`.

<!-- parallel-group: 7 (max 3 concurrent) -->
- [ ] T048 [P] [US5] Test + impl: `internal/services/memories.go` (`MemoriesService`; any `bd` failure ⇒ `BD_UNAVAILABLE` `*ServiceError`, never empty-list masquerade) — `memories_test.go` + `memories.go`.
- [ ] T049 [P] [US5] Test + impl: `internal/api/memories/{dto.go,handlers.go}` — `GET`(`?q=`)/`POST`(`{key?,value}`)/`DELETE /{key}` + `POST /prime {beadID}` — `handlers_test.go` + handlers.

<!-- sequential --> (assembly + router + main shared)
- [ ] T050 [US5] Test + impl: per-bead primed-memories snapshot folded into `assemblePrompt` as a "Primed memories" section (snapshot at prime time) — `internal/orchestrator/assemble_memories_test.go` + assemble.go.
- [ ] T051 [US5] Register memories routes in `internal/api/router.go`; construct `MemoriesService` in `cmd/muster/main.go`.

**Checkpoint**: all 5 user stories complete.

---

## Phase 8: Polish & Cross-Cutting Concerns

<!-- parallel-group: 8 (max 3 concurrent) -->
- [ ] T052 [P] Set real per-package coverage thresholds in the Makefile `thresholds` map for all new packages (SC-007); run `make cover-check`.
- [ ] T053 [P] Additive-surface regression test: assert the full M0–M5 route set + WS event set are unchanged and only the M6 additions are present (SC-008) — `internal/api/router_additive_test.go`.
- [ ] T054 [P] Update README Flags table + API/WS tables with `--muster-dir` and the new `/constitution`, `/skills*`, `/memories*` routes + `constitution.changed`/`runlog.warning` events (additive docs).

<!-- sequential -->
- [ ] T055 Run `quickstart.md` end-to-end against a built binary; fix any drift.
- [ ] T056 Full gate: `make test` (`go test -race ./...`) green, `make lint` clean, `make cover-check` passes.

---

## Dependencies & Story Completion Order

- **Setup (P1) → Foundational (P2)** block everything.
- **US1 (P1)** depends on Foundational (provider seams, retention). Ships as MVP with empty/fake providers.
- **US2 (P1)** depends on US1's provider seam (fills `ConstitutionProvider`).
- **US3 (P2)** is largely independent (registry) but its service/handlers depend on Foundational error codes.
- **US4 (P2)** depends on US1 (assembly) + US3 (registry) — it wires skills into the prompt and adds label plumbing.
- **US5 (P3)** depends on the T002 spike + Foundational; its prime step depends on US1 assembly.
- Shared-file tasks (orchestrator.go, router.go, main.go, event.go, errors.go, mapper.go, Makefile) are marked `<!-- sequential -->` and must not be parallelized against each other.

## Parallel Execution Opportunities

- **Group 1** (T003, T004): scaffolding + Makefile placeholders.
- **Group 3** (T025–T027) then **Group 4** (T028–T030): skill tests, then skill impls (different files in `internal/skills`).
- **Group 5** (T033, T034): skills service + handlers (different packages).
- **Group 6** (T041, T042): the two warning tests.
- **Group 7** (T048, T049): memories service + handlers.
- **Group 8** (T052–T054): polish docs/gates.

## Implementation Strategy

- **MVP = Phase 1–3 (US1)**: assembly replaces `buildPrompt`, byte-verifiable, M2/M4 green. Demoable immediately.
- **Increment 2 = US2**: constitution merged into real dispatches (the milestone's headline).
- **Increment 3 = US3+US4**: skills browsable and wired into the prompt.
- **Increment 4 = US5**: memories facade + priming.
- Each increment is independently testable per its story's Independent Test.

**Total: 56 tasks** — Setup 4 · Foundational 6 · US1 7 · US2 7 · US3 11 · US4 9 · US5 7 · Polish 5.
