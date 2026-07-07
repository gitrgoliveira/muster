# Data Model: M6 — Skills & Constitution

**Feature**: `specs/007-m6-skills-constitution` | **Phase**: 1 (Design) | **Date**: 2026-07-07

Entities below are muster-side types. Per Principle II, none is authoritative *issue* state: the constitution and imported skills are local-file operating config; skill *selection* is read from beads labels via `bd`; memories are owned by `bd`; per-step summaries are disposable run state.

---

## Skill  *(new — `internal/skills`)*

From handoff §3.3 (`prototype/handoff/spec.md:240-256`).

| Field | Type | Notes |
|---|---|---|
| `ID` | `string` | Stable identifier, globally unique across built-in + imported. Referenced by `skill:<ID>` labels. |
| `Name` | `string` | Display name. |
| `Desc` | `string` | One-line description. |
| `Category` | `string` | `spec｜code｜web｜doc｜design｜pm｜infra` (open set; `GET /skills/categories` returns the distinct present set). |
| `Icon` | `string` | Single glyph (UI). |
| `PromptStub` | `string` | System-prompt fragment merged into assembly when the skill loads (one short paragraph; markdown body of the skill file). May be empty (edge case: empty stub ⇒ skill still listed by name, contributes no text). |
| `MCPServers` | `[]string` | Names the agent must already have configured. Best-effort verified, never spawned/managed by muster. |

**Source & mutability**
- **Built-in**: parsed from `//go:embed builtin` markdown files. **Read-only** — `DELETE` on a built-in ID ⇒ typed `SKILL_READONLY` (FR-015).
- **Imported**: one markdown file per skill at `<musterDir>/skills/<ID>.md`, mutable via CRUD, reloaded at startup / on file change.

**File format** (built-in and imported): YAML front-matter + markdown body.
```markdown
---
id: repo-grep
name: Repo Grep
desc: Search the repository efficiently
category: code
icon: 🔎
mcpServers: []
---
Use ripgrep to locate symbols before editing. Prefer exact matches…   ← becomes PromptStub
```

**Validation / rules**
- Import with an `id` colliding a **built-in** ⇒ **rejected** (typed error). Colliding an existing **imported** id ⇒ **upsert** (overwrite). (Research §4, spec edge case.)
- Malformed / unreachable / oversize import ⇒ typed error, **no** partial registration (FR-017).

---

## SkillLoadout  *(derived — assembly-time, not persisted)*

The resolved, de-duplicated set of skills applied to one step's assembly.

- `loadout = dedup(Bead.Skills ∪ Step.Skills)` (FR-018, handoff §6.2).
- **`Bead.Skills`**: from reserved `skill:<id>` `bd` labels, parsed in the store/service layer (research §3). `core.Bead.Skills` (`internal/core/bead.go:20`) — already exists, previously always empty.
- **`Step.Skills`**: per-dispatch override (`core.Step.Skills`, `internal/core/step.go:6`), carried on `DispatchRequest` (new optional `Skills []string`). Takes precedence / unions on top.
- Each resolved ID → looked up in the registry. **Unresolvable ID** ⇒ skipped + runlog warning (FR-020), never a silent drop, never a blocked dispatch.

---

## Constitution  *(new — `internal/services`, file-backed)*

From handoff §3.4 (`spec.md:258-266`).

| Field | Type | Notes |
|---|---|---|
| `Markdown` | `string` | The document. Prepended as the `<system role="muster">` header of every assembled prompt. |
| `Version` | `int` | Monotonic. **Starts at `0`** (never-set/empty default); first `PUT` ⇒ `1`. Embedded/referenced in every dispatch. |
| `UpdatedAt` | `time.Time` | Last `PUT`. |

**Persistence & concurrency**
- Single file `<musterDir>/constitution.md`. Missing file ⇒ `{Markdown:"", Version:0}` (not an error — FR-011).
- In-memory struct behind `sync.RWMutex`. Assembly `RLock`s and copies **markdown+version together** (atomic snapshot — edge case: no torn read under a concurrent `PUT`).
- `PUT` overwrites the file, bumps `Version`, updates `UpdatedAt`, swaps the struct under write-lock, emits `constitution.changed`. **Next** dispatch sees it; an already-written prompt file for a running step is untouched (FR-009).
- Startup reload from disk (FR-010).

---

## AssembledPrompt  *(derived — the bytes written per step)*

The full string written to `<worktree>/.muster-prompt-<stepIdx>.txt` (existing filename convention, `orchestrator.go:554-561`). Replaces the M2/M4 `buildPrompt` output. Structure (handoff §9):

```
<system role="muster">
{Constitution.Markdown}                              ← empty-ok
# Step {stepIdx+1} of {n}: {mode} mode
Provider: {run.Agent}
Skills loaded:
  {for each loadout skill: "{Name} — {PromptStub firstLine}"}
Bead {bead.ID}: {bead.Title}
Acceptance criteria:
{bead.Desc}
Earlier-step summaries:
  {for each prior done|failed step: one-line summary}   ← failed steps labelled, not omitted
Primed memories:                                        ← present only if /memories/prime ran for this bead
  {…}
</system>
<user>
{resolvePrompt(StepProfile.PromptRef, mode) || defaultPromptFor(mode)}
</user>
```
- Degenerate case (empty constitution, no skills, no prior steps, no primed memories) ⇒ still a well-formed prompt (FR-005) — the M2-equivalent single-step path.
- If mode is synthesized (handoff §6.1), the existing `<system role="muster-stage">` stage prefix is included **inside** the step-prompt section (additive layering, not replaced).

---

## StepSummary  *(new — in-memory, disposable run state)*

Backs FR-004 earlier-step summaries (research §6). Not persisted.

| Field | Type | Notes |
|---|---|---|
| (map key) | `int` | step index |
| value | `string` | bounded tail (last N KiB / M lines) of that step's pane output, captured as it streams (`runlog.go`) |
| step status | `done｜failed` | failed steps are included, labelled |

Lives on `Run` (guarded by the existing run mutex), lifetime = the run. Reconstructable/disposable (Principle II).

---

## Memory  *(owned by `bd`, not muster)*

| Field | Type | Notes |
|---|---|---|
| `Key` | `string` | Optional on create (auto-derived if absent). |
| `Value` | `string` | The insight. |

- Persisted & searched entirely via `bd remember/recall/forget/memories` (FR-023/026). Muster holds **no** independent copy. Exact `bd` flag/JSON shapes pinned by a spike + fake-`bd` test before the wrapper is written (research §8).
- `PrimedMemories` (per-bead snapshot from `/memories/prime`) is disposable run state folded into that bead's next `AssembledPrompt`.

---

## store.Issue.Labels  *(additive field — `internal/store/issue.go`)*

New `Labels []string` on the M1 row type so labels reach the read path (research C1/§3). Populated by the label read mechanism (default: `bd`-CLI read); split in `IssueToBead` (`internal/services/mapper.go:29-30`): `skill:*` → `core.Bead.Skills` (prefix stripped), remainder → `core.Bead.Labels`. Additive; no wire/DTO change to existing bead responses beyond the already-present (formerly-empty) `labels`/`skills` fields now carrying data.

---

## Entity relationships

```
Constitution ─┐
              ├─► AssembledPrompt ──► .muster-prompt-<idx>.txt ──► claude adapter (unchanged)
SkillLoadout ─┤        ▲
 (Bead.Skills ∪ Step.Skills, resolved via Registry[Skill])
StepSummary ──┤        │
PrimedMemories┘        │
                       └── one per step launch (step 0 + every advance/loopback), via doLaunch
```
