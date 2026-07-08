# Implementation Plan: M6 — Skills & Constitution

**Branch**: `claude/wizardly-bardeen-284f35` (feature dir `007-m6-skills-constitution`, bead `muster-ep0`) | **Date**: 2026-07-07 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/007-m6-skills-constitution/spec.md`

## Summary

M6 turns the inert two-line `buildPrompt(title, desc)` placeholder (M2, left untouched by M4) into a real, versioned, inspectable **prompt-assembly** path, and adds the three ingredients that feed it: a **constitution** (a single versioned markdown file merged into every dispatch), a **skill registry** (embedded read-only built-ins + URL-imported skills stored as local files, resolved into the prompt via `PromptStub`s), and **memories CRUD** (a thin REST facade over `bd remember/recall/forget/memories`). Skill *selection* is wired via reserved `skill:<id>` **`bd` labels** (resolved: option b) parsed into the existing-but-dormant `core.Bead.Skills`, unioned with a per-dispatch `Step.Skills` override. Everything is **additive** to the M0–M5 REST/WS surface and every non-prompt-content dispatch behavior (launch, session naming, advance/loopback, capacity, idempotency, quota) is preserved.

Technical approach: one assembly function (`internal/orchestrator/assemble.go`) replaces `buildPrompt` at its single call site (`orchestrator.go:861`); three thin handler packages (`internal/api/{constitution,skills,memories}`) over three new services; a new `internal/skills` package (embedded built-ins + `.muster/skills/` file store); label read-side plumbing so `skill:<id>` labels reach `core.Bead.Skills`; bounded per-step output retention so earlier-step summaries exist; and additive WS events (`constitution.changed`, a runlog warning signal). See [research.md](research.md) for the empirical basis and code anchors.

## Technical Context

**Language/Version**: Go (module `github.com/gitrgoliveira/muster`; toolchain per `go.mod`).

**Primary Dependencies**: stdlib (`net/http`, `os`, `embed`, `sync`); `go-chi/chi/v5` (routing); `coder/websocket` (WS); `fsnotify` (file watch); **promote `gopkg.in/yaml.v3` from indirect → direct** (skill front-matter). No new heavyweight deps (Principle I). External runtime tools unchanged: `bd`, `git`/`jj`, `tmux`, `claude`.

**Storage**: Local files only for muster-owned config — `<musterDir>/constitution.md` (single file) and `<musterDir>/skills/<id>.md` (one file per imported skill); built-in skills are `go:embed`-ed (read-only). **No new Dolt tables, nothing routed through `bd`** for constitution/skills (Principle II). Memories are **not** muster-stored — they go through `bd`. Per-step output summaries are in-memory, disposable run state.

**Testing**: `go test -race ./...`; fakes-on-`$PATH` for `bd` (existing `store/bdshell` pattern) + skip-gated real-binary integration; fake-HTTP for URL import + skip-gated real-network. Per-package coverage gates added to the Makefile `thresholds` map.

**Target Platform**: Local loopback single binary (`bin/muster serve`), `127.0.0.1:7766`, no auth (Principle "Local-first").

**Project Type**: Single Go binary (web service + embedded UI). Existing layered layout `core → store → services → api` + `orchestrator`.

**Performance Goals**: No new hot path. Assembly is a per-launch string build (bounded). Label reads are performed at dispatch/assembly time (not per list-render) to avoid N+1 on `List`.

**Constraints**: Additive-only surface (Principle V); handlers thin (Principle III); TDD + `-race` clean (Principle IV, non-negotiable); no credential handling; muster never spawns/manages MCP servers (read-only verification only).

**Scale/Scope**: ~5 new/edited packages, 4 user stories (P1×2, P2×2, P3×1), 30 functional requirements. Two "hidden" scope items surfaced by research (label read-plumbing; per-step output retention) — see Complexity Tracking.

## Constitution Check

*GATE: evaluated against `.specify/memory/constitution.md` v1.0.0. Re-checked post-design (Phase 1).*

| Principle | Verdict | Evidence / How M6 complies |
|---|---|---|
| **I. Single Binary, Self-Contained** | ✅ PASS | Built-ins via `go:embed` (precedent `cmd/muster/embed.go:5`); only new dep is `yaml.v3` (already in graph, promoted to direct) — no heavyweight/DB engine. URL import uses stdlib `net/http`. No new daemon. |
| **II. Beads Is the Source of Truth** | ✅ PASS | Constitution + imported skills are muster **operating config** (local files under `.muster/`), not issue state — no authoritative durable issue state added. Skill *selection* lives in beads as `skill:<id>` labels, **read via `bd`** (the authoritative interface). Memories go **through `bd`** (`remember/recall/forget/memories`), not a parallel store. Per-step summaries are disposable/reconstructable. |
| **III. Layered Architecture — Thin Handlers** | ✅ PASS | 3 new `dto.go`+`handlers.go` packages (validate→service→render), mirroring `internal/api/beads`. Assembly, skill-registry, and `bd`-shelling live in `orchestrator`/`services`/`bdshell`, never in handlers. |
| **IV. Test-First, Per-Layer Coverage (NON-NEGOTIABLE)** | ✅ PASS (plan-enforced) | TDD per layer; fake-on-`$PATH`/fake-HTTP + skip-gated real-binary/real-network tests (FR-029); new packages added to Makefile coverage `thresholds`; `-race` clean; M2/M4 suites updated-not-broken (SC-007/008). |
| **V. Additive, Backward-Compatible Surface** | ✅ PASS | Full route + WS-event inventory confirms none of `/constitution`, `/skills*`, `/memories*`, `constitution.changed`, `runlog.warning` exist. All new. `store.Issue.Labels` addition and `DispatchRequest.Skills` are additive struct fields (internal, not wire-breaking). |

**Additional constraints**: No credential handling (URL import fetches a skill doc, not creds) ✅; user-controlled autonomy unchanged (assembly doesn't alter permission modes) ✅; adapter-agnostic (assembly produces a plain string every adapter consumes) ✅; isolation unchanged ✅; local-first ✅.

**Gate result: PASS.** The two effort items in Complexity Tracking are *scope/effort* notes, not principle violations — both stay inside the principles (labels read via `bd`; summaries are disposable run state).

## Project Structure

### Documentation (this feature)

```text
specs/007-m6-skills-constitution/
├── plan.md              # This file
├── research.md          # Phase 0 — decisions + code anchors (done)
├── data-model.md        # Phase 1 — entities (done)
├── quickstart.md        # Phase 1 — reviewer walkthrough (done)
├── contracts/           # Phase 1 — REST + WS contracts (done)
│   ├── rest-api.md
│   └── ws-events.md
└── tasks.md             # Phase 2 — /speckit-tasks (NOT created here)
```

### Source Code (repository root)

```text
internal/
├── skills/                      # NEW — Skill type, built-in embed, imported-file store, registry
│   ├── skill.go                 #   Skill{ID,Name,Desc,Category,Icon,PromptStub,MCPServers}, front-matter parse
│   ├── registry.go              #   built-in (embed.FS) + imported (.muster/skills/*.md) union, CRUD, collision rule
│   ├── builtin/                 #   //go:embed builtin  — read-only markdown skills (seed catalog)
│   └── import.go                #   URL import: scheme allowlist, 10s timeout, 1MiB cap
├── orchestrator/
│   ├── assemble.go              # NEW — assemblePrompt(...) (replaces buildPrompt), defaultPromptFor table
│   ├── orchestrator.go          # EDIT — call assemblePrompt at :861; delete buildPrompt; per-step summary tee; Config gains Constitution+Skills providers
│   ├── runlog.go                # EDIT — tee bounded tail into run.stepSummaries
│   └── steps.go / steps_types.go# EDIT — resolvePrompt(PromptRef,mode); thread Step.Skills
├── core/
│   ├── bead.go                  # (Labels+Skills fields already exist — now populated)
│   └── step.go                  # (Skills field already exists — now threaded)
├── store/
│   ├── issue.go                 # EDIT — add Labels []string
│   ├── bdshell/verbs.go         # EDIT — add Labels(read), Remember/Recall/Forget/Memories verbs
│   └── (dolt|jsonl)/            # EDIT (if Dolt-join path chosen) — see research §3; default: bd-CLI label read
├── services/
│   ├── mapper.go                # EDIT — split labels → Bead.Labels vs skill:* → Bead.Skills
│   ├── constitution.go          # NEW — ConstitutionService (RWMutex snapshot, file persistence, publish)
│   ├── skills.go                # NEW — SkillRegistry service (wraps internal/skills)
│   └── memories.go              # NEW — MemoriesService (bd wrapper)
├── api/
│   ├── constitution/            # NEW — dto.go + handlers.go (GET/PUT)
│   ├── skills/                  # NEW — dto.go + handlers.go (GET, GET categories, POST import, DELETE)
│   ├── memories/                # NEW — dto.go + handlers.go (GET/POST/DELETE, POST /prime)
│   ├── render/errors.go         # EDIT — add CodeSkillReadonly = "SKILL_READONLY"
│   └── router.go                # EDIT — register the 3 new packages
├── ws/event.go                  # EDIT — add EventConstitutionChanged (+ runlog warning); optional Frame.Version
├── config/                      # EDIT — resolve <musterDir> (constitution + skills base dir)
cmd/muster/main.go               # EDIT — construct 3 services, thread into orchestrator.Config + router
Makefile                         # EDIT — add coverage thresholds for new packages
```

**Structure Decision**: Extend the existing single-binary layered layout. New capability (skills) gets its own package behind the registry interface (Principle III). Every new REST route is a thin handler package parallel to `internal/api/beads`. No new top-level project.

## Complexity Tracking

> These are **effort/scope** items surfaced by Phase 0 research where the spec's stated assumption was optimistic. Neither is a Constitution *violation* — both are recorded here for the tasks phase and cross-model review to size correctly.

| Item | Why needed | Note / Simpler alternative rejected |
|---|---|---|
| **Label read-side plumbing** (research C1/§3) | Option (b) requires `skill:<id>` labels to reach `core.Bead.Skills`, but labels do **not** flow through the read path today (`store.Issue` has no `Labels`; `IssueToBead` hardcodes empty; write path rejects labels). | Read labels via `bd` (authoritative, backend-agnostic) at dispatch/assembly time. Rejected: hardcoding a Dolt labels-table join (Dolt-only, couples to beads schema, blind for the JSONL backend). |
| **Per-step output retention** (research C3/§6) | FR-004 earlier-step summaries need each step's runlog, but runlog is streamed live and **never retained** server-side. | Capture a bounded tail into disposable in-run state (Principle II compliant). Rejected: a durable runlog store (over-scoped, touches Principle II) and re-reading tmux scrollback at assembly (racy / unavailable post-close). |
| **Runlog warning signal** (research §5) | FR-020/FR-021 require *visible* (non-silent) warnings for unresolvable skills / missing MCP servers, but `ws.Frame` has no severity field and `runlog.line` is best-effort-dropped under load. | Add an additive `runlog.warning` event type (not dropped). Rejected: reusing `runlog.line` (droppable + no severity metadata). |
