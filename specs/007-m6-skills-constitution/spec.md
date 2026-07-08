# Feature Specification: M6 — Skills & Constitution

**Feature Branch**: `007-m6-skills-constitution`

**Created**: 2026-07-06

**Status**: Draft

**Input**: Give muster's dispatch path a real prompt instead of the placeholder `buildPrompt(title, desc)` M2 shipped and M4 deliberately left untouched. Per the canonical roadmap (`prototype/handoff/spec.md` §20, milestone **M6 — Skills & constitution**): "Skill registry, prompt assembly, constitution merge. Memories CRUD." Deliver: (1) a **skill registry** (built-in + user/URL-imported skills) with CRUD over the mutable portion; (2) a **constitution** — a single versioned markdown document merged into every dispatched prompt; (3) **prompt assembly** — resolving `StepProfile.PromptRef` into the full constitution+bead+skills+history prompt template from handoff §9, replacing `buildPrompt`; (4) **Memories CRUD** — a thin muster HTTP surface, most likely wrapping `bd`'s own existing `remember`/`recall`/`forget`/`memories` primitives rather than inventing a parallel store. Directory numbered `007-*` to continue the `001–006` sequence (M0–M5); "M6" and `007-m6-skills-constitution` refer to the same milestone.

> **Naming note:** the roadmap labels this milestone **M6**. The spec directory is numbered `007-*` to continue the existing `001/002/.../006` sequence (M0/M1/M2/M3/M4/M5). "M6" and directory `007-m6-skills-constitution` refer to the same milestone throughout.

## Context & Prior-Milestone Boundary

M4 shipped `StepProfile{Name, PermissionMode, PromptRef}` as part of the step-chain mechanism, but explicitly stubbed prompt assembly: `internal/orchestrator/steps_types.go` documents `PromptRef` as "logical prompt identifier; resolved in M6," and `internal/orchestrator/steps.go`'s `resolveChain` doc comment states plainly "`PromptRef` is stored per `StepProfile` but resolves to the M2 bead prompt in M4 (real skill/constitution assembly is M6)." The M4 review (`specs/005-m4-dispatcher/review.md`, finding **L2**) and `specs/005-m4-dispatcher/tasks.md` **T043** both flag the same gap and the same boundary: *"do not build M6 assembly here."* Concretely, every step today calls `buildPrompt(req.BeadTitle, req.BeadDesc)` (`internal/orchestrator/orchestrator.go:861`) regardless of which step or which `PromptRef` is set — a fixed two-line prompt (`"# {title}\n\n{desc}\n"`) with no constitution, no skills, no earlier-step context.

M6 is the milestone where `PromptRef` stops being an inert string and prompt quality becomes a first-class, testable, versioned concern: a skill registry an operator can browse/extend, a constitution document that is merged into every dispatch and can be edited without a restart, and an assembly function that produces the exact multi-part prompt handoff §9 specifies — while staying inside the Constitution's boundaries (muster owns no durable *issue* state; `bd` is the sole writer of anything that is beads-authoritative).

## Clarifications

### Session 2026-07-06

- **Q: The vision doc (§3.6) imagines Skills/Constitution living in Dolt tables muster owns directly — does Constitution II (bd is the sole authoritative writer of issue state) forbid that?** → A: **No conflict, but not Dolt either.** Constitution II governs *issue* state (beads/steps/step_runs/etc.) — skills and the constitution are muster's own **operating configuration**, not issue data, the same category as muster's other operating config — the M3 `--default-vcs` knob and the `~/.muster/worktrees` root established in M2 (note: there is **no** `.muster/config.toml` today; muster config is flags + `MUSTER_*` env + `<beads-dir>/metadata.json`). They are stored as **local files under a resolved `.muster/` directory** (default `~/.muster`; see FR-008), not as new Dolt tables and not through `bd`. This keeps muster's durable-issue-state footprint at zero (Principle II) while keeping the binary self-contained (Principle I — no new embedded DB engine).
- **Q: Where do built-in skills live, and are they writable?** → A: **Embedded in the binary, read-only.** Built-in skills ship as `go:embed`-ed markdown/JSON under `internal/skills/builtin/` (or similar), matching Principle I ("single self-contained binary"). `DELETE /api/v1/skills/{id}` on a built-in ID MUST fail with a typed error (e.g. `SKILL_READONLY`), never silently no-op.
- **Q: Where do user/URL-imported skills persist across restarts?** → A: **One markdown file per imported skill** at `<musterDir>/skills/<id>.md` (YAML front-matter with `id/name/desc/category/icon/mcpServers` + markdown body = `PromptStub`), reloaded at startup / on file change and mutated in place by the CRUD handlers. Individual files (not a single `skills.json`) so per-skill add/delete is atomic and diffs cleanly. Not Dolt, not through `bd` (Clarification 1).
- **Q: Where does the constitution live, and how does an edit take effect?** → A: **`.muster/constitution.md`**, exactly as handoff §3.6 states ("single file; UI edits go here"). `PUT /api/v1/constitution` overwrites the file, increments an in-memory/on-disk `Version` int, and the *next* dispatch (not any already-running step) picks up the new markdown + version. No migration/history of prior versions is required in M6 (the file itself + `git`/backup is the only history, consistent with "muster owns no durable state of its own").
- **Q: Where does per-bead/per-step skill selection come from — a new muster-side field, or bd labels?** → A: **RESOLVED (Session 2026-07-06) — option (b): a reserved `bd label` prefix `skill:<id>`.** `core.Bead.Skills` and `core.Step.Skills` fields already exist in the type system (`internal/core/bead.go`, `internal/core/step.go`) but today are pure UI-mock seed data (`internal/store/seed.go`) never populated from `bd` (which has no `skills` concept) and never threaded into `orchestrator.DispatchRequest`, which has no `Skills` field at all. **Decision:** bead-level skill selection is carried as reserved `skill:<id>` **`bd` labels**, parsed at bead **read-time in the store/service layer** to populate the (previously dormant) `core.Bead.Skills` field — using `bd` as the authoritative reader of label state, so selection persists in beads (the source of truth) with **no new schema/column** and muster holds no durable state of its own. (Read-side plumbing is required: labels do not currently reach muster's read path — see the Session 2026-07-07 correction and Assumptions.) The **step-level** selection is an explicit per-dispatch set (`Step.Skills`, analogous to `StepProfile.PromptRef`) that is **unioned** (additively, never subtractively) with the bead-level set — a skill in either set is loaded exactly once (handoff §6.2: `bead.Skills ∪ step.Skills`); it does **not** replace the bead-level set. Considered and **rejected: (a)** config-derived/dispatch-override only (mirroring M2's prefix→repo map and M4's chain override) — rejected because per-bead selection would then have no persistent home in the SoT. The resolution is never a silent default — an unresolvable skill ID (including a `skill:<id>` label naming an unknown skill) is a warning, not a silent drop (mirrors the MCP-server verification below).
- **Q: Is Memories CRUD a new muster-owned store, or a wrapper over `bd`'s existing memory primitives?** → A: **A thin wrapper over `bd`.** Investigation into `bd --help` / `bd remember --help` / `bd memories --help` confirms `bd` already ships persistent, keyed memory: `bd remember "<insight>" [--key K]` (upsert), `bd recall <key>` (get), `bd forget <key>` (delete), `bd memories [search]` (list/search) — the exact CRUD shape handoff §4.1 asks for (`GET/POST/DELETE /api/v1/memories`). Building a parallel muster-owned memory store would be a second writer to conceptually the same durable-knowledge concern the Constitution already assigns to `bd` (Principle II's spirit, even though memories are not "issue state" in the narrowest sense) and would duplicate work `bd` already does well. M6's Memories CRUD is therefore a **thin `internal/api/memories` handler package that shells out to `bd remember/recall/forget/memories`** (through a small `store/bdshell`-style wrapper, following the existing authoritative-writer pattern), not a new muster-side table or file. `POST /api/v1/memories/prime` (load memories into a bead's next dispatch) is additive glue: it calls `bd memories`/`bd recall` and folds the results into that bead's next assembled prompt as an extra section — no new bd-side capability required.
- **Q: Does prompt assembly need to be adapter/provider-generic (post-M5) or can it be claude-only?** → A: **Provider-agnostic by construction, validated against claude only.** At spec-writing time `specs/006-m5-multi-provider/` exists as an empty directory (no spec drafted yet), so M6 cannot assume M5's adapter abstraction has landed. The assembly step (building the constitution+bead+skills+history prompt string in handoff §9) is adapter-agnostic already — it produces a plain string, and every adapter (present: claude; future: gemini/codex/opencode/direct-API) consumes an assembled prompt the same way (stdin/temp-file for CLIs, session input for SDKs, `messages` for direct-API, per handoff §6.2 "Agent input contract"). M6 implements and tests assembly against the one adapter that exists today (`claude`) and does not block on M5; if M5 lands first, M6 slots in without changes to the assembly function's signature.
- **Q: Does the constitution/skill assembly change the `<system role="muster-stage">` synthesized-mode prefix M2/M4 already inject for non-native modes?** → A: **No — additive layering, not replacement.** The synthesized-stage prefix (handoff §6.1, "This step is a code review...") is a *different* concern (workflow-stage framing for a mode with no native CLI flag) from the constitution/skill header (handoff §9, muster's operating rules + loaded skills). M6 assembly wraps/prepends the constitution+skills+bead block; if a step's mode is synthesized, the existing stage-prefix text is still included inside the assembled prompt's step-prompt section, not removed or duplicated.

### Session 2026-07-07 (checklist revision)

Requirements-quality pass (`checklists/requirements.md`) corrected two optimistic assumptions and pinned loose wording — **no design change**:

- Constitution starting `version` pinned to **0** (first `PUT` → `1`) — was "0 or 1". [FR-011, US2 AS1]
- FR-018 clarified: the loadout is the de-duplicated **union** (a skill is an opaque ID; the step-level set is additive, not a per-skill override).
- **Correction**: there is **no** `.muster/config.toml` precedent — muster config is flags + `MUSTER_*` env + `<beads-dir>/metadata.json`; the real local-config precedent is `~/.muster/worktrees` (M2). `<musterDir>` (default `~/.muster`, `--muster-dir`/`MUSTER_DIR` override) resolves the constitution + skills paths. [FR-008]
- **Correction**: `bd` labels are **not** already surfaced on muster's read path; M6 adds read-side label plumbing via `bd`. [Assumptions, FR-018]
- Added explicit URL-import safety (scheme allowlist + 10 s timeout + 1 MiB cap) and the imported-skill format (YAML front-matter + markdown body). [FR-014, FR-017, Edge Cases]
- Added edge cases: malformed `skill:` label; concurrent skill CRUD; pinned the built-in ID-collision rule (`SKILL_ID_CONFLICT`).

## User Scenarios & Testing *(mandatory)*

M6 is where the fixed two-line `buildPrompt` placeholder becomes a real, inspectable, versioned prompt: an operator can see and edit muster's "constitution" (its own operating rules for agents), attach named skills to a bead or step and see their `PromptStub`s actually appear in what the agent reads, and let earlier-step summaries flow forward in a chain — all while every prior milestone's dispatch behavior (M2 single-step, M4 multi-step chain, quota, idempotency) stays exactly as it was, because assembly only changes *what string gets written to the prompt file*, not the launch mechanics around it.

### User Story 1 - Prompt assembly replaces the placeholder (Priority: P1)

Every dispatched step's prompt is now the full assembled template — constitution header, step/provider framing, loaded skills, the bead's title/desc as acceptance criteria, earlier-done-step summaries, and the step's own prompt (default-for-mode or an explicit override) — instead of the fixed `"# {title}\n\n{desc}\n"` string.

**Why this priority**: This is the entire point of the milestone and the literal gap M4 left (`PromptRef` inert, `buildPrompt` ignoring it). Nothing else in M6 matters if assembly doesn't happen. It is independently testable and demonstrable the moment `.muster/constitution.md` exists (even with an empty skill registry and empty history), because the template's shape doesn't depend on skills or memories being populated.

**Independent Test**: Dispatch a bead with a non-empty `.muster/constitution.md`; read the written `.muster-prompt-<idx>.txt` for step 0 and assert it contains the constitution markdown, the bead title/desc under an "Acceptance criteria" heading, and a step-prompt section — byte-verifiable, no agent execution required. Dispatch a 2-step chain where step 0 completes; assert step 1's assembled prompt contains a one-line summary of step 0's outcome.

**Acceptance Scenarios**:

1. **Given** `.muster/constitution.md` contains markdown text, **When** any step is dispatched, **Then** the written prompt file's header section is exactly that markdown (versioned — see US2).
2. **Given** a step whose `StepProfile.PromptRef` names a known step-prompt (default-for-mode or a stored override), **When** the step is assembled, **Then** the assembled prompt's user-turn section is that resolved text, not the M4 two-line placeholder.
3. **Given** a chain where step 0 (`plan`) has completed with runlog output, **When** step 1 (`build`) is assembled, **Then** the assembled prompt includes a one-line summary of step 0 drawn from the runlog (per handoff §9 "Earlier-step summaries").
4. **Given** no `.muster/constitution.md` file exists yet (fresh install), **When** a step is dispatched, **Then** assembly MUST NOT fail dispatch — it uses a documented empty/default constitution rather than erroring (no silent behavior change to launch success/failure).
5. **Given** the existing M2/M4 test suites (single-step `buildPrompt` behavior, multi-step chain mechanics, quota, idempotency), **When** M6 assembly replaces `buildPrompt`, **Then** those suites are updated to assert against the new assembled-prompt shape but every non-prompt-content behavior (launch, session naming, advance/loopback, capacity, idempotency) is unchanged.

---

### User Story 2 - Constitution CRUD, versioned and merged into every dispatch (Priority: P1)

An operator can read and replace muster's constitution — a single markdown document describing its own agents' operating rules — through `GET`/`PUT /api/v1/constitution`, and every subsequent dispatch's assembled prompt carries the current version.

**Why this priority**: The constitution is the first, always-present element of the assembled prompt (US1 depends on it existing) and is the simplest possible "config that affects prompts" — a single file, no registry, no merge logic — making it the right second-priority slice after raw assembly plumbing.

**Independent Test**: `PUT /api/v1/constitution` with new markdown; assert `GET /api/v1/constitution` returns the new markdown with an incremented `version`; dispatch a bead and assert the assembled prompt's header is the new markdown and includes the new version number; assert a step whose agent was already running when the `PUT` happened is unaffected (only the *next* dispatch/step picks up the change).

**Acceptance Scenarios**:

1. **Given** no constitution has ever been set, **When** `GET /api/v1/constitution` is called, **Then** it returns a well-defined empty/default document with `version: 0` (the never-set/empty default; the first successful `PUT` increments to `1`) rather than a 404.
2. **Given** a `PUT /api/v1/constitution { markdown }` call, **When** it succeeds, **Then** `.muster/constitution.md` is overwritten, the version increments monotonically, and a `constitution.changed` WS event is emitted (additive event type).
3. **Given** an updated constitution, **When** a new dispatch is assembled, **Then** its prompt header is the updated markdown and the version is embedded/referenced in the assembled prompt (handoff §3.4: "included in every dispatch").
4. **Given** muster restarts, **When** it starts back up, **Then** it reloads `.muster/constitution.md` from disk and reports the same markdown/version as before the restart (muster holds no durable state of its own — the file is the persistence).
5. **Given** an oversized or malformed `PUT` body, **When** submitted, **Then** it is rejected with a clear validation error (body-limit middleware, consistent with existing write-endpoint conventions), not silently truncated.

---

### User Story 3 - Skill registry: browse built-ins, import/remove user skills (Priority: P2)

An operator can list the full skill registry (built-in + imported), see it grouped by category, import a new skill from a URL, and delete a previously-imported (non-built-in) skill. Built-in skills are always present and read-only.

**Why this priority**: Skills are the second ingredient of assembly (after the constitution) but are additive/optional — a dispatch with zero skills loaded still produces a valid, useful assembled prompt (US1 covers that case). Skill CRUD is real, testable value but ranks below "assembly happens at all" and "constitution merges."

**Independent Test**: `GET /api/v1/skills` returns the built-in set at minimum, even with zero imports; `POST /api/v1/skills { url }` imports a new skill and it subsequently appears in the list; `DELETE /api/v1/skills/{id}` removes a previously-imported skill but returns a typed `SKILL_READONLY` error (not a 404, not a silent no-op) for any built-in ID; restarting muster preserves imported skills (loaded from `.muster/skills/`) and still surfaces all built-ins (loaded from the embedded FS).

**Acceptance Scenarios**:

1. **Given** a fresh install with no imports, **When** `GET /api/v1/skills` is called, **Then** it returns the full built-in catalog (each with `ID, Name, Desc, Category, Icon, PromptStub, MCPServers`), sourced from the embedded read-only FS.
2. **Given** `GET /api/v1/skills/categories`, **When** called, **Then** it returns the distinct set of categories present across built-in + imported skills.
3. **Given** `POST /api/v1/skills { url }`, **When** the URL resolves to a well-formed skill definition, **Then** the skill is persisted under `.muster/skills/` and immediately appears in subsequent `GET /api/v1/skills` calls, including after a restart.
4. **Given** a malformed or unreachable `url`, **When** `POST /api/v1/skills` is called, **Then** it fails with a clear, typed error — no partial/corrupt skill is registered.
5. **Given** a built-in skill ID, **When** `DELETE /api/v1/skills/{id}` is called, **Then** it is rejected with `SKILL_READONLY` (never silently succeeds, never 404s as if the ID didn't exist).
6. **Given** a previously-imported skill ID, **When** `DELETE /api/v1/skills/{id}` is called, **Then** it is removed from `.muster/skills/` and absent from subsequent listings.

---

### User Story 4 - Skill loadout resolves into the assembled prompt, with best-effort MCP verification (Priority: P2)

A bead or step's selected skills actually show up in the assembled prompt (a "Skills loaded" section listing each skill's name and the first line of its `PromptStub`), and any MCP servers a skill expects are verified against the agent's own config — if one is missing, muster emits a non-blocking `runlog.warning` rather than failing or silently omitting. (The MCP-server list is a verification input, not part of the rendered prompt.)

**Why this priority**: This is where the skill registry (US3) actually connects to assembly (US1) — without it, US3 is a CRUD feature with no effect on dispatch. It's P2 rather than P1 because assembly and the constitution are meaningful and shippable without skill resolution (a bead with no skills selected still gets a correct, useful prompt).

**Independent Test**: Dispatch a bead/step with one or more resolved skill IDs; assert the assembled prompt's "Skills loaded" section lists each skill's name and the first line of its `PromptStub` (handoff §9 exact template); configure a skill with an `MCPServers` entry not present in a fake agent MCP config; assert dispatch still succeeds and a warning appears in the runlog (`runlog.line` with a warning-shaped `Kind`), never a blocked/failed dispatch.

**Acceptance Scenarios**:

1. **Given** a resolved skill loadout of `["repo-grep", "run-tests"]` for a step, **When** the prompt is assembled, **Then** the "Skills loaded" section names both, each with its `PromptStub`'s first line (handoff §9 template line: `${for each merged skill: name — promptStub.firstLine}`).
2. **Given** an unresolvable skill ID (typo, deleted skill) in the loadout, **When** assembly runs, **Then** dispatch is **not blocked** — the unknown ID is skipped with a warning (never a silent drop with no signal, per the "no silent defaults" posture applied to warnings rather than defaults).
3. **Given** a skill whose `MCPServers` lists a server name, **When** assembly runs and muster best-effort-checks the agent's own MCP config for that name, **Then**: if present, nothing extra happens (or an informational note); if absent, dispatch proceeds and a runlog warning names the missing server — dispatch is **never blocked** on this check (handoff §3.3, explicit design constraint).
4. **Given** `bead.Skills ∪ step.Skills` per handoff §6.2 (bead-level parsed from `skill:<id>` `bd` labels, step-level from the per-dispatch `Step.Skills` override — see Clarifications), **When** both are non-empty and overlapping, **Then** the resolved loadout is the de-duplicated union, not a plain concatenation with duplicates.
5. **Given** muster verifying MCP servers, **When** it does so, **Then** it MUST NOT spawn, manage, proxy, or configure any MCP server itself — verification reads the agent's existing config only (handoff §3.3: "these must already be registered in the CLI/agent's own config; Muster does not spawn or manage MCP servers").

---

### User Story 5 - Memories CRUD as a thin wrapper over `bd` (Priority: P3)

An operator (or a client) can list, search, create/update, and delete persistent memories, and prime a specific bead's next dispatch with relevant memories folded into its assembled prompt — all backed by `bd`'s existing `remember`/`recall`/`forget`/`memories` commands, not a new muster-owned store.

**Why this priority**: Memories are the least load-bearing of the four M6 deliverables for prompt quality (assembly, constitution, and skills already produce a materially better prompt without them) and `bd` already implements the hard part (persistence, search) — this story is a thin REST facade plus one piece of assembly glue (`/memories/prime`). Ranked P3 as genuinely nice-to-have polish on top of P1/P2.

**Independent Test**: `POST /api/v1/memories { key?, value }` upserts a memory (shelling to `bd remember`); `GET /api/v1/memories?q=<term>` returns matches (shelling to `bd memories <term>`); `DELETE /api/v1/memories/{key}` removes it (shelling to `bd forget <key>`); `POST /api/v1/memories/prime { beadID }` causes that bead's next dispatch's assembled prompt to include a "Primed memories" section sourced from the current memory set.

**Acceptance Scenarios**:

1. **Given** `POST /api/v1/memories { value: "always run tests with -race" }` with no explicit key, **When** it succeeds, **Then** a memory is stored (via `bd remember`) with an auto-derived key, returned in the response.
2. **Given** `POST /api/v1/memories { key: "dolt-phantoms", value: "..." }` against an existing key, **When** it succeeds, **Then** the memory is updated in place (via `bd remember --key`), not duplicated.
3. **Given** `GET /api/v1/memories?q=dolt`, **When** called, **Then** it returns memories matching the query (via `bd memories dolt`), and `GET /api/v1/memories` with no query returns the full list.
4. **Given** `DELETE /api/v1/memories/{key}` for an existing key, **When** it succeeds, **Then** the memory is gone from subsequent listings (via `bd forget`); for a non-existent key, it returns a clear not-found error rather than a false success.
5. **Given** `POST /api/v1/memories/prime { beadID }`, **When** the named bead is next dispatched, **Then** the assembled prompt includes an additional "Primed memories" section reflecting the current memory set at prime time (not re-queried live at dispatch, unless the plan phase decides otherwise — the ordering guarantee is pinned in planning, not here).
6. **Given** the `bd` binary is unavailable or errors, **When** any `/memories` endpoint is called, **Then** it fails with a clear, typed error surfaced to the client — never a silent empty-list success that masks a broken backend (mirrors the existing `store/bdshell` error-surfacing convention).

---

### Edge Cases

- **Constitution file present but unreadable/corrupt** (permissions error, invalid UTF-8): assembly MUST NOT crash the dispatch path; it falls back to the documented empty/default constitution and surfaces a startup/reload warning (log + status field), never a silent swallow with no signal.
- **Skill `PromptStub` is empty or missing**: the skill still appears in "Skills loaded" by name; an empty stub contributes no text, not an error.
- **An imported skill shares an `ID` with a built-in**: the import MUST be rejected with a typed error (`SKILL_ID_CONFLICT`), never silently shadowing the read-only built-in — this keeps `skill:<id>` label resolution unambiguous. An import whose `ID` matches an existing *imported* skill is an explicit upsert (overwrite in place).
- **Constitution `PUT` racing a live dispatch's assembly read**: assembly MUST read a consistent snapshot (whole file + version together), never a torn read mixing old markdown with a new version number or vice versa.
- **A step's `PromptRef` is empty/unset**: resolves to the documented default-for-mode prompt (handoff §9 `defaultPromptFor(mode, specSkill)`), not an empty user turn.
- **Earlier-step summaries when a prior step `failed`, not `done`**: the summary section still includes it (labelled as failed), so the next step's agent knows a prior attempt did not succeed — silently omitting failed-step context would hide relevant history from the agent.
- **`bd remember`/`bd recall`/`bd forget`/`bd memories` invoked when `--readonly` or sandbox mode is active on the `bd` side**: muster's memories handlers surface whatever error `bd` returns rather than reinterpreting or masking it.
- **Skill import URL points at a huge, slow, or hostile resource**: the server-side fetch MUST be bounded and safe — a scheme allowlist (`https`, plus `http` only for loopback), a request timeout (10 s), and a response size cap (1 MiB, matching the body-limit middleware). An SSRF-shaped, disallowed-scheme, oversize, or slow fetch MUST fail with a typed error and register no partial skill.
- **A `skill:<id>` label or imported skill `id` is malformed or hostile** (empty, extra colons like `skill:a:b`, path separators, or `.`/`..` traversal such as `skill:../../etc/x`): it MUST be rejected by a single `ValidateID` gate — *ignored-with-warning* for a label, a typed `SKILL_INVALID_ID` (400) for an import — and MUST NOT crash assembly, resolve to a bogus skill, or escape `<musterDir>/skills/` on any filesystem operation.
- **Concurrent skill import/delete of the same id**: CRUD over `<musterDir>/skills/` MUST be serialized (or use atomic file ops) so two concurrent writers cannot leave a half-written or orphaned skill file.
- **MCP-server verification when the agent's own config is itself unreadable** (e.g. the CLI isn't installed, or its config path can't be probed): treated the same as "server not found" — a warning, never a blocking error, per US4 AS3's explicit non-blocking design.

## Requirements *(mandatory)*

### Functional Requirements

**Prompt assembly (US1)**

- **FR-001**: The dispatcher MUST replace `buildPrompt(title, desc)` with an assembly function that produces the full template from handoff §9: a `<system role="muster">` block containing the constitution markdown, step/provider framing (`Step ${i+1} of ${n}`, provider name), the resolved skill loadout, the bead's title/desc as acceptance criteria, and earlier-done-step summaries, followed by a `<user>` block containing the resolved step prompt.
- **FR-002**: `StepProfile.PromptRef` MUST be resolved at assembly time into either a stored explicit prompt override or a documented default-for-mode prompt (`defaultPromptFor(mode, ...)`) — it MUST NOT remain an inert, unused string as it is through M4.
- **FR-003**: Assembly MUST run once per step launch (including on `Advance`/`LoopBack` relaunch), producing a fresh prompt file per step index exactly as the existing `.muster-prompt-<stepIdx>.txt` per-step-file convention already requires (no change to that file-naming contract).
- **FR-004**: Assembly MUST include a **one-line** summary of each prior `done` or `failed` step in the current run's chain, before the current step's own prompt content. The summary is derived deterministically so assembly stays byte-verifiable (SC-001): muster retains a bounded tail of each step's runlog (last **8 KiB / 100 lines**), and the one-line summary is the **last non-blank line** of that tail, truncated to **200 chars** (a `failed` step is included and labelled, never omitted).
- **FR-005**: Assembly MUST succeed (producing a valid, if minimal, prompt) even when the constitution is absent/empty, the skill loadout is empty, and there are no earlier steps — the single-step M2-equivalent case MUST still produce a well-formed prompt, not an error.
- **FR-006**: Assembly MUST NOT alter any non-prompt-content dispatch behavior — session naming, worktree creation, capacity gating, idempotency, quota parsing, and step-pointer mechanics from M2/M4 are unchanged; only the *contents written to the prompt file* change.

**Constitution (US2)**

- **FR-007**: Muster MUST expose `GET /api/v1/constitution` returning `{ markdown, updatedAt, version }` and `PUT /api/v1/constitution { markdown }` that overwrites `.muster/constitution.md`, increments `version` monotonically, updates `updatedAt`, and emits an additive `constitution.changed` WS event.
- **FR-008**: The constitution MUST be stored as a single local file at `<musterDir>/constitution.md`, where `<musterDir>` is a resolved directory (default `~/.muster`, overridable via a `--muster-dir`/`MUSTER_DIR` flag) — not a new Dolt table, not routed through `bd` — consistent with Constitution II governing *issue* state only, and with the `~/.muster` operational-config precedent (the M2 worktrees root).
- **FR-009**: A constitution update MUST take effect for the *next* dispatch/step assembly; it MUST NOT retroactively alter a prompt already written to disk for a currently-running step.
- **FR-010**: On startup (and on `POST /api/v1/orchestrator/reload`, if applicable), muster MUST reload the constitution from disk so that muster restarts do not lose an operator's edit (muster itself holds no durable state — the file is the sole persistence).
- **FR-011**: A missing constitution file MUST NOT be an error at startup or at assembly time; it MUST resolve to a documented empty/default document with starting `version: 0` (the first successful `PUT` increments to `1`).

**Skill registry (US3)**

- **FR-012**: Muster MUST ship a built-in skill catalog embedded in the binary (`go:embed`), read-only at runtime, matching the `Skill{ID, Name, Desc, Category, Icon, PromptStub, MCPServers}` shape from handoff §3.3.
- **FR-013**: Muster MUST expose `GET /api/v1/skills` (full registry: built-in + imported) and `GET /api/v1/skills/categories` (distinct categories across both).
- **FR-014**: Muster MUST expose `POST /api/v1/skills { url }` to import a skill definition from a URL. The imported resource MUST be a markdown document with a YAML front-matter header (`id/name/desc/category/icon/mcpServers`) and a markdown body (the `PromptStub`) — the same on-disk format as built-in skills. On success it is persisted at `<musterDir>/skills/<id>.md`, survives restart, and is immediately visible in subsequent `GET /api/v1/skills` calls.
- **FR-015**: Muster MUST expose `DELETE /api/v1/skills/{id}`, which MUST succeed only for a previously-imported (non-built-in) skill; attempting to delete a built-in ID MUST fail with a typed `SKILL_READONLY` error (never a silent no-op, never a false success).
- **FR-016**: Imported skills MUST be stored as local file(s) under `<musterDir>/skills/`, not a new Dolt table and not through `bd` — same storage-class rationale as FR-008. A skill `id` (from import front-matter or a `skill:<id>` label) MUST pass a validation gate (`^[a-z0-9][a-z0-9._-]*$`; no separators, no `.`/`..`) before it is used in any filesystem path or label resolution — an invalid id is a typed `SKILL_INVALID_ID` on import and ignored-with-warning on a label, never a path escape.
- **FR-017**: A skill import from a malformed, unreachable, disallowed-scheme, oversize, or timed-out URL — or one whose payload is not a valid skill document — MUST fail explicitly (typed error) and MUST NOT register a partial/corrupt skill entry. The fetch MUST enforce the scheme allowlist, timeout, and size cap defined in Edge Cases.

**Skill loadout resolution (US4)**

- **FR-018**: Assembly MUST resolve a step's effective skill loadout as the **de-duplicated union** `bead.Skills ∪ step.Skills` (handoff §6.2). Because a skill is an opaque ID (no per-skill settings), the union is the whole rule — a skill present in either set is loaded exactly once, and the step-level set is purely additive to the bead-level set (never subtractive). The selection mechanism is **resolved** (see Clarifications, Session 2026-07-06): the **bead-level** set is derived from reserved `skill:<id>` **`bd` labels** parsed in the store/service layer into `core.Bead.Skills`; the **step-level** set is the per-dispatch `Step.Skills` override carried on `DispatchRequest`.
- **FR-019**: For each resolved skill in the loadout, assembly MUST append that skill's name and the first line of its `PromptStub` into the "Skills loaded" section of the assembled prompt (handoff §9 template).
- **FR-020**: An unresolvable skill ID in the loadout MUST NOT block or fail dispatch — it MUST be skipped with a visible warning (runlog line and/or status field), never a silent, signal-free drop.
- **FR-021**: When a resolved skill lists `MCPServers`, muster MUST perform a **best-effort, non-blocking** check that each named server appears in the agent's own existing MCP configuration (located per the agent/provider — for `claude`, its own config file), and MUST emit a warning (not an error, never a dispatch failure) when a named server is not found. If the agent's MCP config is itself unreadable or absent, that is treated as "server not found" — a warning, never a block.
- **FR-022**: Muster MUST NOT spawn, register, proxy, or otherwise manage the lifecycle of any MCP server as part of skill resolution or verification — verification is read-only inspection of the agent's own config.

**Memories CRUD (US5)**

- **FR-023**: Muster MUST expose `GET /api/v1/memories` (list, optional `?q=` search) `POST /api/v1/memories` (`{key?, value}` upsert) and `DELETE /api/v1/memories/{key}`, each implemented as a thin handler that shells out to `bd memories`/`bd remember`/`bd forget` respectively — no new muster-owned memory store.
- **FR-024**: Muster MUST expose `POST /api/v1/memories/prime { beadID }`, which associates a snapshot of current memories with the named bead such that its *next* dispatch's assembled prompt includes a "Primed memories" section. The snapshot MUST be **persisted** (e.g. `<musterDir>/primed/<beadID>.json`) so it survives a muster restart between the `prime` call and the next dispatch (disposable, per-bead, reconstructable — not authoritative issue state); an in-memory-only snapshot would silently break the "next dispatch" guarantee across a restart.
- **FR-025**: Any failure from the underlying `bd` invocation (binary missing, non-zero exit, unparseable output) on a `/memories*` endpoint MUST be surfaced to the client as a clear, typed error — never masked as an empty-list success.
- **FR-026**: Memories writes MUST go through the `bd` CLI exactly as beads issue-state writes do (`store/bdshell` pattern) — muster introduces no direct memory persistence of its own, keeping a single writer to this durable-knowledge concern.

**Cross-cutting (Constitution)**

- **FR-027**: All new REST routes (`/constitution`, `/skills*`, `/memories*`), DTO fields, and WS event types (`constitution.changed`, any skill/memory events) MUST be additive; no M0–M5 route/shape/event is changed or removed, and the prior-milestone suites MUST stay green.
- **FR-028**: All persistent state M6 introduces (constitution file, imported-skill files) MUST be local files under `.muster/`, never new Dolt tables and never routed through `bd`; all memory persistence MUST go through `bd` (FR-026) — no muster-side database of its own (Constitution I & II).
- **FR-029**: Every new capability MUST be delivered test-first with per-layer coverage and MUST pass `go test -race ./...`; the skill-URL-import path and the `bd`-shelling memories path each get a fake-on-`$PATH`/fake-HTTP unit test **and**, where a real external dependency exists, a skip-gated real-binary/real-network integration test.
- **FR-030**: Handlers for `/constitution`, `/skills*`, and `/memories*` MUST remain thin (validate input → call a service → render); assembly, skill-registry management, and the `bd`-shelling logic live in the service/orchestrator layer, not in `internal/api`.

### Key Entities

- **Constitution**: `{Markdown string, UpdatedAt time.Time, Version int}` — a single versioned document, stored at `.muster/constitution.md`, merged as the header of every assembled prompt (handoff §3.4).
- **Skill**: `{ID, Name, Desc, Category, Icon, PromptStub, MCPServers []string}` (handoff §3.3) — either built-in (embedded, read-only) or imported (persisted under `.muster/skills/`, mutable via CRUD).
- **Skill loadout**: the resolved, de-duplicated set of skills applicable to one step's assembly — `bead.Skills ∪ step.Skills`. Each contributes the first line of its `PromptStub` to the prompt's "Skills loaded" section; its `MCPServers` names feed best-effort verification (a `runlog.warning` when missing), not the rendered prompt.
- **Assembled prompt**: the full string written to a step's `.muster-prompt-<stepIdx>.txt` — constitution + step/provider framing + skill loadout + bead ticket + earlier-step summaries + resolved step prompt (handoff §9). Replaces the M2/M4 `buildPrompt` output.
- **Memory**: `{Key string, Value string}` — persisted and searched entirely through `bd`'s existing `remember`/`recall`/`forget`/`memories` commands; muster holds no independent copy.
- **PromptRef resolution**: the (until-M6) inert `StepProfile.PromptRef` string, now resolved into either a stored explicit override or `defaultPromptFor(mode, ...)` — the concrete mechanism this milestone builds to close the M4 L2/T043 gap.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A dispatched step's written prompt file contains the constitution markdown, the bead's title/desc, and a resolved step prompt — verified byte-for-byte against the assembled template, replacing the old fixed two-line `buildPrompt` output for every dispatch path (single-step and multi-step chain alike).
- **SC-002**: An operator can `PUT` a new constitution and observe, without restarting muster, that the *next* dispatched step's prompt reflects the new markdown and an incremented version — while a step already running when the `PUT` occurs is unaffected.
- **SC-003**: `GET /api/v1/skills` always returns at least the full built-in catalog, even with zero imports; an imported skill added via `POST /api/v1/skills {url}` survives a muster restart and appears in subsequent listings; deleting a built-in ID always fails with `SKILL_READONLY`.
- **SC-004**: A bead/step with a non-empty resolved skill loadout produces an assembled prompt whose "Skills loaded" section names every resolved skill with its `PromptStub` first line; an unresolvable skill ID never blocks dispatch.
- **SC-005**: A skill with an `MCPServers` entry not present in a (test) agent config still dispatches successfully, with a visible runlog warning — dispatch is never blocked on MCP verification, in any test run.
- **SC-006**: `POST/GET/DELETE /api/v1/memories` round-trip correctly against a fake `bd` on `$PATH` (create → appears in list/search → delete → absent), and a real-`bd` integration test (skip-gated on `bd`'s presence) passes the same round-trip.
- **SC-007**: `go test -race ./...` passes for the whole repository, the M0–M5 suites remain green, and per-package coverage gates (including any new M6 package added to the Makefile `thresholds` map) hold.
- **SC-008**: No existing M0–M5 REST route, response shape, or WS event type is changed or removed (verified by the prior-milestone contract/suite tests).

## Assumptions

- **Skills and the constitution are muster's own operating configuration, not beads issue state** — they are stored as local files under `.muster/` (constitution: single markdown file; skills: embedded read-only built-ins + a local mutable store for imports), never as new Dolt tables and never routed through `bd`. This reads Constitution II as scoped to *issue* state, matching how `.muster/config.toml` (M1–M4) and `--default-vcs` (M3) already establish local operational config as unproblematic under Principle I/II.
- **Memories are the one M6 concern that IS routed through `bd`**, because `bd` already ships the exact primitive (`remember`/`recall`/`forget`/`memories`) — building a second, muster-owned memory store would create a duplicate writer to the same durable-knowledge concern the project's tooling already owns. M6's `/memories*` endpoints are a thin REST facade, following the same "shell out, don't own" posture as `store/bdshell` for issue state.
- **Assembly is a pure string-building function layered in front of the existing launch path** — it changes what `buildPrompt`'s replacement writes to the per-step prompt file; it does not change worktree creation, tmux session naming, capacity gating, idempotency, or quota parsing, all of which M4 already delivers and tests.
- **Skill selection wiring (`bead.Skills ∪ step.Skills`) is resolved to option (b): a reserved `bd label` prefix `skill:<id>`.** Bead-level selection is parsed from `skill:*` labels into `core.Bead.Skills`; the step-level `Step.Skills` per-dispatch override is unioned on top. This needs **no beads schema change** (labels are an existing `bd` concept) but does require new **read-side plumbing**: labels do not currently reach muster's read path (`store.Issue` has no labels; `IssueToBead` hardcodes them empty; the write path rejects labels), so M6 must surface a bead's labels — via `bd`, the authoritative reader, at dispatch/assembly time — before splitting `skill:*` out. Skill *definitions* stay muster config (Clarification 1) while per-bead *selection* lives in the beads SoT via `bd`. Option (a) (config/dispatch-override only) was rejected because per-bead selection would lack a persistent home. FR-018 onward hold as written under this resolution.
- **Prompt assembly is validated against the `claude` adapter only**, since M5 (multi-provider) has no drafted spec at the time of writing (`specs/006-m5-multi-provider/` is an empty directory). The assembly function itself is provider-agnostic (produces a plain string consumed identically by any adapter, per handoff §6.2's "Agent input contract"), so M6 does not need to wait for M5, and M5 does not need to change M6's assembly signature when it lands.
- **No historical versioning of the constitution beyond the monotonic `Version` int** is required in M6 — the current file + the operator's own git/backup discipline is the only history; muster does not implement a version-diff or rollback UI in this milestone.
- **The built-in skill catalog's initial contents** (which specific skills ship embedded at M6) are a plan-phase/seed-data decision, not pinned by this spec; this spec only requires that *some* non-empty, embedded, read-only catalog exists and is enumerable.

## Out of Scope (deferred to later milestones)

- **Multi-provider adapters** (Gemini/Codex/OpenCode/direct-API) — **M5**. M5's spec had not yet been drafted at the time of writing (`specs/006-m5-multi-provider/` empty); M6 is a soft, non-blocking dependency on it — see Assumptions. Assembly is validated against `claude` only in M6.
- **`/repos` CRUD, probe, hot-reload, multi-`.beads` aggregation** — **M7**. M6 does not touch repo attachment/discovery.
- **UI wiring** of the constitution editor, skill browser, and memories panel into the embedded prototype UI — tracked with the **M7** UI work, same posture M4 took for its own UI wiring.
- **Sub-bead auto-split, escalation, loop/gate policy** — **M8**. M6's assembly surfaces earlier-step summaries and skill/constitution content into the prompt, but does not add any new automatic dispatch-triggering behavior.
- **A new beads-model `skills` field or schema migration** — the resolved `bd label`-based mechanism (see Clarifications) needs **no beads schema change** at all: `skill:<id>` labels use the existing, already-authoritative label surface. M6 does not introduce a new authoritative beads column.
- **Constitution version history / diff / rollback UI** — deferred; M6 ships the monotonic version counter only.
- **Skill registry marketplace/discovery** (browsing a catalog of importable skills beyond a raw URL) — out of scope; `POST /api/v1/skills {url}` is a direct import, not a curated marketplace.
- **Formulas/molecules** (`GET /api/v1/formulas`, `POST /api/v1/cook`) — not part of M6; skills are a prompt-assembly ingredient, formulas are a separate step-chain templating concept mentioned elsewhere in the roadmap and not scoped here.

## Dependencies & Roadmap Position

- **Builds on M4**: fills the `PromptRef`/assembly gap M4 deliberately left stubbed — cited explicitly in `internal/orchestrator/steps_types.go`, `internal/orchestrator/steps.go`'s `resolveChain` doc comment, `specs/005-m4-dispatcher/tasks.md` T043 ("real skill/constitution assembly is M6. Do not build M6 assembly here"), and `specs/005-m4-dispatcher/review.md` finding L2 ("`PromptRef` is stored but resolves to the M2 bead prompt in M4; real assembly is M6"). M6 replaces `buildPrompt` and gives `StepProfile.PromptRef` its first real consumer.
- **Soft dependency on M5** (multi-provider): not a hard blocker — M5 had no drafted spec at the time this spec was written. Assembly is built and tested against the one adapter that exists today (`claude`) and is provider-agnostic in shape, so it does not need to wait for M5's adapter abstraction, and M5 should not need to change M6's assembly interface when it lands.
- **Builds on the Constitution's local-config precedent**: `.muster/config.toml` (M1–M4) and `--default-vcs` (M3) already establish that muster's own operational settings live in local files, not Dolt/`bd` — M6 extends this same precedent to the constitution file and the imported-skill store.
- **Builds on `bd`'s existing memory primitives**: `bd remember`/`bd recall`/`bd forget`/`bd memories` already implement the persistence and search Memories CRUD needs; M6 does not invent a parallel mechanism, consistent with Constitution II's single-writer principle applied by analogy to durable knowledge more broadly.
- **Unblocks**: no downstream milestone is strictly blocked on M6, but M6 is a prerequisite for **any** milestone that cares about prompt quality — a better-assembled prompt (constitution + skills + history) benefits every future dispatch once it ships, and M8's sub-bead/escalation policy work will likely want to reference skill loadouts and constitution content when it designs its own prompts.
