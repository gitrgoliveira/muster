# Research: M6 — Skills & Constitution

**Feature**: `specs/007-m6-skills-constitution` | **Date**: 2026-07-07 | **Phase**: 0 (Outline & Research)

All file:line anchors are against the repo at branch `claude/wizardly-bardeen-284f35`. Findings were gathered by three parallel read-only code-exploration passes over `internal/orchestrator`, `internal/core`, `internal/store`, `internal/api`, `internal/ws`, `internal/services`, `internal/config`, `cmd/muster`, and `prototype/handoff/spec.md`.

---

## 0. Spec-assumption corrections (empirical — Constitution "verify before building")

Three premises in the spec/clarifications did **not** survive contact with the code. None invalidate the milestone's decisions, but each changes scope or storage and is pinned here.

### C1 — `bd` labels do **not** round-trip through muster's read path today

- **Spec premise** (Clarification, option b): *"repurpose `bd label` (which is real, wired, and already round-trips through `store/bdshell`)."*
- **Reality**: `store.Issue` (`internal/store/issue.go:6-24`) has **no `Labels` field**; the Dolt backend's `listSQL`/`getSQL` (`internal/store/dolt/query.go:6-18`) select only from the `issues` table (no labels join) and `scanIntoIssue` (`internal/store/dolt/backend.go:143-151`) scans 17 columns with no labels; the JSONL backend `json.Unmarshal`s into `store.Issue` and would drop any labels; and the single conversion site `IssueToBead` (`internal/services/mapper.go:11-51`) hardcodes `Labels: []string{}` and `Skills: []string{}` (`:29-30`). The **write** path actively rejects labels (`internal/services/beads.go:420-421, 468-469`: "labels not supported by bd CLI").
- **Decision**: Option (b) stands, but M6 must **surface labels on the read side**. See §3 for the chosen mechanism (`bd show --json` enrichment vs Dolt labels-table join).

### C2 — There is no `.muster/config.toml`; the real precedent is `~/.muster/`

- **Spec premise** (Clarification 1): skills/constitution are *"the same category as the already-existing `.muster/config.toml` (M1–M4)."*
- **Reality**: no `.muster/config.toml` and **no TOML dependency** exist (`grep toml` over `*.go`/`go.sum` = empty). Muster's backend config is `<beads-dir>/metadata.json` (`internal/config/metadata.go:11-23`) plus flags + `MUSTER_*` env (`cmd/muster/main.go`, `internal/config/orchestrator.go`). The genuine `.muster` precedent is the **worktrees root** `~/.muster/worktrees` (`internal/config/orchestrator.go:105-111`).
- **Decision**: The local-file storage decision (not Dolt, not `bd`) still holds — it just rests on the `~/.muster/worktrees` precedent, not a config.toml. See §3.5 for `.muster` base-dir resolution.

### C3 — Earlier-step summaries (FR-004) have **no** backing store

- **Spec premise**: assembly includes *"a one-line summary of step 0 drawn from the runlog."*
- **Reality**: runlog bytes are streamed live to the WS hub (`internal/orchestrator/runlog.go:29-59`) and **never retained server-side**. The `Run` struct (`orchestrator.go:69-`) has no per-step output field; the only summary-shaped types are the live `RunSummary` status snapshot (`orchestrator.go:419-450`) and the git/jj `DiffSummary`.
- **Decision**: M6 introduces minimal per-step output retention (capture the **tail** of each step's pane stream into the run), consumed by assembly. See §6.

---

## 1. Prompt assembly — replacing `buildPrompt` (US1, FR-001..006)

**Single swap point.** `buildPrompt(title, desc)` is defined at `internal/orchestrator/orchestrator.go:1252-1259` (produces `"# {title}\n\n{desc}\n"`) and has exactly **one** call site: `doLaunch` at `orchestrator.go:861`. Step-0 dispatch, queued-admission, and every `Advance`/`LoopBack` relaunch all funnel through `doLaunch` (`steps.go:256` → `relaunchNextStep`), so replacing that one call covers the entire chain. The `Run` struct preserves `BeadTitle`/`BeadDesc` (`orchestrator.go:71-72`) for relaunch.

- **Decision**: Introduce `internal/orchestrator/assemble.go` with `assemblePrompt(in AssemblyInput) string`, called at `orchestrator.go:861` in place of `buildPrompt`. `buildPrompt` is deleted (its M2 behavior becomes the degenerate output of assembly when constitution/skills/summaries are all empty — FR-005).
- **Rationale**: one funnel = one change; adapter contract (below) is untouched.
- **Adapter contract is safe.** The claude adapter runs `exec claude --permission-mode <pm> < .muster-prompt-<n>.txt` via `sh -c` (`internal/adapter/claude/claude.go:217-218`); it validates `PromptFile` is a single filename under the worktree (`:190-193`) and is otherwise **prompt-content-agnostic**. M6 changes only the bytes written at `orchestrator.go:860-864`. No adapter change (FR-006).
- **Template** (handoff §9, `prototype/handoff/spec.md:790-815`) — assembly emits:
  ```
  <system role="muster">
  ${constitution.Markdown}

  # Step ${i+1} of ${n}: ${mode} mode
  Provider: ${provider.Name}
  Skills loaded:
    ${for each merged skill: name — promptStub.firstLine}

  Bead ${bead.ID}: ${bead.Title}
  Acceptance criteria:
  ${bead.Desc}

  Earlier-step summaries:
  ${for each done/failed step: one-line summary from runlog}
  </system>
  <user>
  ${step.Prompt || defaultPromptFor(mode, specSkill)}
  </user>
  ```
- **`PromptRef` resolution (FR-002).** `StepProfile` (`steps_types.go:8-12`) has `{Name, PermissionMode, PromptRef}` — no `Mode`, no inline `Prompt`. Nothing reads `PromptRef` today. Decision: `PromptRef` resolves at assembly time via `resolvePrompt(promptRef, mode)`: if `PromptRef` names a stored override (see §7 default table keyed by ref/mode) use it; else fall back to `defaultPromptFor(mode)`. No new `StepProfile.Prompt` field is required for M6 (keeps the M4 type stable); an explicit inline override is represented by a reserved `PromptRef` value the override table resolves. Per-step **mode** framing uses `run.Mode` (mode is uniform per run in M4; `Step ${i+1} of ${n}` uses `stepIdx`/`len(*run.Chain)`).

---

## 2. `defaultPromptFor(mode, specSkill)` (FR-002, §7 below is the table)

- **Decision**: a package-level `map[core.Mode]string` in `internal/orchestrator/assemble.go` (or `prompts.go`), covering the 6 core modes (`internal/adapter/claude/enums.go:76-82`: plan/agent/build/review/apply/yolo — exact set to confirm at impl). `specSkill` (a spec-authoring skill hint) is optional; when unset the plain per-mode default is used.
- **Rationale**: handoff §9 says *"`defaultPromptFor` returns the same text the UI shows in the step editor — the backend and UI share the table"* (`spec.md:815`), but **no shared Go table exists today** (`cmd/muster/ui/` is an embedded static web asset, not shared Go code). So the authoritative source for the per-mode strings is **`prototype/handoff/spec.md §9/§6.1`**, transcribed into a Go table (`internal/orchestrator/prompts.go`) — see tasks T015. It is not "found in the UI" at implementation time.
- **Note**: only `plan` and `agent`/`default` modes are actually launchable today (`modeSupported`, `orchestrator.go:822`; adapter `Modes()`, `claude.go:131-146`). `build`/`review`/`apply`/`yolo` exist as enums but are rejected at dispatch. The table covers all modes for forward-compat; assembly only ever runs for supported modes.

---

## 3. Skill selection wiring — option (b), `skill:<id>` labels (US4, FR-018)

**The core plumbing gap (C1).** To populate the dormant `core.Bead.Skills` (`internal/core/bead.go:20`) from reserved `skill:<id>` labels, labels must first reach the read path. `core.Bead` already has a `Labels []string` field (`bead.go:13`), also hardcoded empty today.

Two candidate read mechanisms:

| Mechanism | How | Pros | Cons |
|---|---|---|---|
| **(i) `bd show --json` enrichment** | Add a `bdshell` verb that reads a bead's labels via the `bd` CLI (authoritative reader), enrich `store.Issue`/`core.Bead` in the service layer | Uses `bd` as the reader (symmetry with `bd` as writer, Principle II); backend-agnostic (works for both dolt + jsonl) | Extra `bd` invocation per read; N+1 on `List` unless batched |
| **(ii) Dolt labels-table join** | Extend `listSQL`/`getSQL` + `scanIntoIssue` to join the beads labels table; add `Labels` to `store.Issue` | No extra process; single query | Dolt-only (JSONL backend still blind); couples muster to the beads label schema; more invasive |

- **Decision**: **(i) `bd`-CLI label read**, surfaced through the store/service layer, is the primary path — it keeps `bd` as the single authoritative interface to issue state (Principle II), is backend-agnostic, and avoids hardcoding the Dolt label schema. Batch on `List` by reading labels only when a bead is dispatched/assembled (skills are needed at assembly time, not on every list render), which sidesteps the N+1 concern. `IssueToBead` (`mapper.go:29-30`) splits the label set: entries matching `^skill:(.+)$` → `Bead.Skills` (stripped of prefix); the rest → `Bead.Labels`.
- **Union & precedence (FR-018)**: effective loadout = dedup(`Bead.Skills` ∪ `Step.Skills`). `Step.Skills` (`internal/core/step.go:6`) is the per-dispatch override, carried on the (new) assembly input; it takes precedence when a step is present. `DispatchRequest` gains an optional `Skills []string` for the step-level override (mirrors M4's chain override threading).
- **Unresolvable skill IDs (FR-020)**: a `skill:<id>` label naming an unknown skill is **not** a silent drop — it is skipped with a runlog warning (see §5).

---

## 4. Skill registry: built-ins (embed) + imported (file store) (US3, FR-012..017)

- **`Skill` type (handoff §3.3, `spec.md:240-256`)** — create `internal/skills` with:
  ```go
  type Skill struct {
      ID, Name, Desc, Category, Icon, PromptStub string
      MCPServers []string
  }
  ```
- **Built-ins (FR-012)**: `//go:embed builtin` + `embed.FS` under `internal/skills/builtin/`, mirroring the existing embed precedent `cmd/muster/embed.go:5-6` (`//go:embed ui`). Read-only at runtime. Initial catalog contents are seed-data (plan does not pin the exact list; the spec requires only a non-empty enumerable catalog).
- **Format decision (research item 1)**: built-in and imported skills are **markdown files with a YAML front-matter header** (`---` block with `id/name/desc/category/icon/mcpServers`, body = `PromptStub`). Rationale: matches how the ecosystem ships skills (front-matter + prose stub), keeps `PromptStub` human-editable, and `gopkg.in/yaml.v3` is already in the module graph (currently `// indirect` via testify — **promote to direct**). JSON was rejected: it makes the `PromptStub` prose awkward to author/read and diverges from the "markdown skill" convention.
- **Imported-skill storage (research item 3)**: **one file per imported skill** at `<musterDir>/skills/<id>.md`. Rationale: individual files diff/version cleanly (Principle II "reconstructable, disposable" — git is the history), enable atomic per-skill add/delete without rewriting a shared JSON, and reuse the JSONL backend's mtime+`fsnotify` reload pattern (`internal/store/jsonl/backend.go:124`, `internal/store/watcher.go`). A single `skills.json` was rejected: concurrent CRUD would need whole-file rewrites and locking.
- **ID-collision rule (research item 4)**: importing a skill whose `id` equals a **built-in** id is **rejected** with a typed error (not shadowed, not namespaced) — matches FR-015's "built-ins are read-only, never silently overridden" posture and keeps IDs globally unique so `skill:<id>` label resolution is unambiguous. Importing an id equal to an existing **imported** skill is an **upsert** (overwrite in place). This is the explicit, non-silent rule the spec's edge-case requires.

---

## 5. Skill URL import & MCP verification (US3/US4, FR-014, FR-017, FR-021..022)

- **URL import safety (research item 2)**: `POST /api/v1/skills {url}` fetches server-side (no existing HTTP-client helper — use stdlib `net/http`). Pins:
  - **Scheme allowlist**: `https` only (also allow `http` for `localhost`/loopback to support local skill servers in dev). Rationale: muster binds loopback with no auth; an unrestricted server-side fetch is an SSRF vector.
  - **Timeout**: `http.Client{Timeout: 10s}` (aligns with the bdshell 30s ceiling but tighter for a fetch).
  - **Size cap**: `io.LimitReader` at **1 MiB** (matches the existing `BodyLimit` 1 MiB cap, `internal/api/middleware/bodylimit.go`).
  - **Validation**: parse front-matter; a malformed/oversize/unreachable resource yields a typed error and registers **no** partial skill (FR-017).
- **MCP verification (FR-021/022, best-effort, non-blocking)**: assembly reads the agent's own MCP config (read-only) and, for each `MCPServers` entry not found, emits a **runlog warning** — never blocks dispatch. Muster does **not** spawn/manage MCP servers (handoff §3.3, `spec.md:256`). Locating the agent's MCP config is provider-specific (claude: its own config file); M6 does a best-effort probe and treats an unreadable config as "server not found" (a warning), per US4 AS3 / edge case.
- **Runlog warning mechanism (shared by FR-020 & FR-021)**: the `ws.Frame` envelope (`internal/ws/event.go:57-115`) currently has **no severity/`Kind` field**, and the hub best-effort-drops `runlog.line` under backpressure (`hub.go:117`). **Decision**: add a new **additive** WS event type `runlog.warning` (or an additive `Kind`/`Level *string omitempty` field on `Frame` consumed by `runlog.line`). Prefer a distinct `EventType` so warnings are **not** dropped under load (unlike `runlog.line`). This is additive (Principle V) — no existing event changes.

---

## 6. Earlier-step summaries — per-step output retention (FR-004, C3)

- **Decision**: capture a bounded **tail** (e.g. last N KiB / last M lines) of each step's pane output into a new per-step field on `Run` (e.g. `stepSummaries map[int]string`, guarded by the existing run mutex). The `runlogStreamer` (`runlog.go:29-59`) already sees every byte; tee the tail into the run as it streams. On assembly of step *k*, include a one-line summary of each prior `done`/`failed` step *< k* (edge case: failed steps are labelled, not omitted).
- **Rationale**: minimal, disposable, reconstructable state (Principle II) — it lives only for the run's lifetime and is not authoritative issue state. Avoids introducing a durable runlog store (out of scope; that would be a larger, separate concern).
- **Alternatives rejected**: (a) persisting full runlogs to disk/Dolt — over-scoped and touches Principle II; (b) re-reading tmux scrollback at assembly — racy against a live pane and unavailable post-close.

---

## 7. Constitution storage, versioning & atomic reads (US2, FR-007..011)

- **Storage (§3.6, `spec.md:282-288`)**: single file `<musterDir>/constitution.md`. Not Dolt, not `bd` (FR-008).
- **`.muster` base-dir resolution (C2)**: introduce a resolved `musterDir` (default: the same base as worktrees — `~/.muster` — with a `--muster-dir`/`MUSTER_DIR` override; the plan pins the flag name). `constitution.md` and `skills/` live directly under it. Missing dir/file is **not** an error (FR-011): a fresh install resolves to an empty/default constitution.
- **Starting `Version` (research item 5)**: **`0`** for the never-set/empty default document; the first successful `PUT` increments to `1`. Rationale: `0` cleanly denotes "no operator constitution yet" and keeps "version embedded in every prompt" honest (a fresh install advertises `v0`). `GET` on a fresh install returns `{markdown:"", version:0}` (well-defined, not 404 — AS1).
- **Atomic snapshot read (edge case: PUT racing assembly)**: the in-memory constitution is a single struct `{Markdown string, Version int, UpdatedAt time.Time}` behind a `sync.RWMutex`; assembly takes an `RLock` and copies **markdown+version together** (no torn read). `PUT` writes the file then swaps the struct under a write lock, and emits `constitution.changed` (FR-007). A running step's already-written prompt file is untouched (FR-009); only the *next* assembly sees the new version. Reload-from-disk on startup (FR-010) uses the same struct.

---

## 8. Memories CRUD — thin `bd` wrapper (US5, FR-023..026)

- **Decision**: new `internal/api/memories` handler package + `internal/services/memories.go`, backed by **new `bdshell` verbs** `Remember/Recall/Forget/Memories` following the exact exec pattern of `Create`/`AppendNote` (`internal/store/bdshell/verbs.go:32,118`) over `Run`/`RunJSON`/`RunVoid` (`exec.go:67,107,119`). No muster-owned memory store (FR-026).
- **Empirical check required at impl (Constitution "verify before building")**: confirm the actual `bd remember/recall/forget/memories` flag shapes and JSON output against the real `bd` (the spec's Clarification cites them, but pin them with a spike + a fake-`bd`-on-`$PATH` test, mirroring the existing `store/bdshell` fake pattern).
- **`/memories/prime` (FR-024)**: associates a snapshot of current memories with a bead so its next dispatch's assembly includes a "Primed memories" section. Snapshot-at-prime-time (not live-requeried), stored as disposable per-bead run state.
- **Error surfacing (FR-025)**: any `bd` failure → typed error to the client (never an empty-list masquerade), reusing the `*CLIError`→`ServiceError` translation already in `internal/services/beads.go` (`wrapCLIError`).

---

## 9. New REST handlers, WS event & DI wiring (US2/3/5, FR-027..030)

- **Additive-only confirmed (Principle V, FR-027/SC-008)**. Full current route set (`internal/api/router.go:45-76`) and WS event set (`internal/ws/event.go:7-53`) were inventoried; none of `/constitution`, `/skills*`, `/memories*`, `constitution.changed`, or `runlog.warning` exist. M6 adds only new ones.
- **Handler pattern (Principle III)**: each new package is `dto.go` + `handlers.go` with `type Handlers struct { svc *services.XService }` + `NewHandlers(...)`, mirroring `internal/api/beads/handlers.go:19-26`. Validate/shape-check in the handler; business logic in the service. Reuse `render.WriteJSON`/`render.WriteError` (`internal/api/render`), add `CodeSkillReadonly = "SKILL_READONLY"` to `render/errors.go:9-24`, and apply `middleware.BodyLimit` to `PUT /constitution` + `POST /skills` (`internal/api/middleware/bodylimit.go`).
- **Services (Principle III)**: `ConstitutionService`, `SkillRegistry`, `MemoriesService` in `internal/services`, each with a `NewXService(...)` constructor + optional `Publisher` (`internal/services/events.go:8`) for `constitution.changed`. Follow the `*ServiceError{Code,Message}` idiom (`beads.go:61-66`).
- **WS event**: add `EventConstitutionChanged EventType = "constitution.changed"` (and `EventRunlogWarning` if chosen) to the const block at `event.go:7-53`; optional additive `Version *int` on `Frame`.
- **DI wiring (`cmd/muster/main.go`)**: construct the three services in `main()` alongside `svc`/`hub`/`orchestrator` (`main.go:355-380`), pass `hub.Broadcast` as their `Publisher`, and thread the constitution + skill registry into `orchestrator.Config` (`orchestrator.go:229`) so `assemblePrompt` can reach them. Register routes by extending `api.NewRouter` (or threading a config struct like the existing `StatusConfig`/`SchedulerSnapshotter` precedent, `router.go:47-50`).

---

## 10. Testing strategy (Principle IV — NON-NEGOTIABLE)

- **TDD, per layer, `-race` clean.** New packages (`internal/skills`, `internal/api/{constitution,skills,memories}`, `internal/services/{constitution,skills,memories}`, assembly in `internal/orchestrator`) each get a coverage gate added to the Makefile `thresholds` map (SC-007).
- **Fakes-on-`$PATH` + skip-gated real-binary tests** (FR-029): the memories path gets a fake-`bd` unit test **and** a real-`bd` integration test (skip when absent); the skill-URL-import path gets a fake-HTTP unit test **and** a real-network integration test (skip-gated).
- **Prior-milestone suites stay green** (SC-007/008): M2 `buildPrompt` and M4 chain tests are **updated** to assert the new assembled-prompt shape while every non-prompt-content behavior (launch, session naming, advance/loopback, capacity, idempotency, quota) is asserted unchanged.
- **Byte-verifiable assembly** (SC-001): assemble a prompt with a known constitution + skills + 2-step chain and assert the written `.muster-prompt-<idx>.txt` exactly matches the template — no agent execution needed.

---

## Open items intentionally left to the tasks/impl phase

- Exact `--muster-dir`/`MUSTER_DIR` flag name and default (§7) — cosmetic, pinned at impl.
- Exact built-in skill catalog contents (§4) — seed data, spec requires only "non-empty".
- Exact per-mode `defaultPromptFor` strings (§2) — transcribed from `prototype/handoff/spec.md §9/§6.1` into `internal/orchestrator/prompts.go` (tasks T015); there is no shared UI Go table to source them from.
- `runlog.warning` vs additive `Frame.Kind` field (§5) — both additive; pick at impl for minimal surface.
- `bd` memory flag shapes (§8) — pinned by a spike against the real `bd` before writing the wrapper.
