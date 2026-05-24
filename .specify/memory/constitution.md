<!--
SYNC IMPACT REPORT
==================
Version change: N/A → 1.0.0 (initial ratification)
Added sections:
  - Principles I–IX covering binary self-containment, orchestrator-not-model,
    beads-as-unit-of-work, layered architecture, constitution-as-code,
    per-step skills, TDD, simplicity, and multi-repo routing.
  - Development Workflow
  - Quality Gates
  - Governance
Removed sections: N/A (initial ratification — replaced blank template)
Templates requiring updates:
  - .specify/templates/plan-template.md — no change required
  - .specify/templates/spec-template.md — no change required
  - .specify/templates/tasks-template.md — no change required
Deferred TODOs: None

Sources informing this ratification:
  - handoff/spec.md  (full backend contract, data model, milestone definitions)
  - handoff/DESIGN.md (mental model, modes, skills, dispatcher, multi-repo routing)
  - specs/001-m0-skeleton/plan.md (layered architecture, TDD ordering, quality gates)
  - bracket-creator/.specify/memory/constitution.md (structural model)
-->

# Muster Constitution

## Core Principles

### I. Single Binary, Self-Contained

`muster` MUST be distributed and run as a single Go binary. The embedded
UI, seed data, and all runtime dependencies MUST be compiled in at build
time. No external process, database, message broker, or cloud service is
required to start or operate the server.

**Rationale**: The primary audience is a developer (or agent) running
`muster` locally. A self-contained binary with zero prerequisites is the
lowest possible barrier to entry. Requiring an external service would break
offline use and complicate single-machine agent orchestration.

**Non-negotiable rules**:
- The web UI MUST be embedded via `//go:embed ui/*`. Serving from disk at
  runtime is not acceptable.
- Moving the binary to a different directory MUST NOT break the UI or any
  API endpoint — no hardcoded paths, no relative file references.
- `go build ./...` MUST produce a clean binary with no CGo dependencies.
- Startup MUST print a banner (port, schema version, build info) to stdout
  and exit non-zero if the port is already in use.
- Graceful shutdown on `SIGINT`/`SIGTERM` with a 5-second drain timeout is
  mandatory. WS clients MUST receive a normal close frame (1001).
- Beads persistence via the embedded Dolt DB lives under `.muster/beads/`
  relative to the working directory; this path MUST be configurable but
  defaults to the repository root.

### II. Orchestrator, Not a Model

Muster shells out to CLI agents (Claude Code, Gemini CLI, OpenCode, Codex)
or calls provider APIs directly. It MUST NEVER embed or act as an LLM
itself. Its role is to schedule, dispatch, supervise, and persist — not to
generate tokens.

**Rationale**: Muster's value is the orchestration layer — dispatching the
right bead to the right agent at the right time, capturing output, and
persisting results to Beads. If Muster tried to generate code itself, it
would duplicate the responsibility of its adapters and blur the abstraction
boundary that makes swapping agents trivial.

**Non-negotiable rules**:
- All code generation, reasoning, and tool use MUST happen inside the
  agent process (CLI subprocess, SDK call, or direct API request).
- Muster reads agent output (stdout, worktree diffs, tmux capture) — it
  MUST NOT interpret or transform the semantic content of that output beyond
  parsing structured events.
- The `Adapter` interface (`internal/cli`, `internal/sdk`, `internal/apiprov`)
  is the ONLY point where provider-specific logic is permitted. No handler,
  service, or dispatcher code may contain provider-specific conditionals.
- Every new provider MUST implement the `Adapter` interface. A
  "special-cased" provider that bypasses the interface is a violation.

### III. Beads as the Unit of Work

A **bead** is the atomic unit of tracked work. It encapsulates intent
(title, desc, acceptance criteria), execution plan (steps, skills, agent
assignments), lifecycle state (column, history, gates), and cross-repo
dependencies. The Beads Dolt DB is the canonical persistence layer; all
other representations are derived from it.

**Rationale**: A single canonical work unit enables the dispatcher to reason
uniformly about any task regardless of source repo, provider, or milestone.
Without this invariant, per-repo or per-provider special cases proliferate
until the orchestrator becomes a patchwork of conditionals.

**Non-negotiable rules**:
- The canonical bead type lives in `internal/core`. No other package may
  define a parallel bead representation; translation to/from DTOs happens
  only at the transport boundary (`internal/api/`).
- Bead schema changes (field additions, removals, renames) are breaking
  changes and MUST follow the amendment procedure in the Governance section.
- Sub-beads (`bd-a1f2.1`) are first-class; dispatcher, store, and API MUST
  handle them consistently with root beads.
- Skills and step chains are per-bead configuration, not dispatcher state.
  The dispatcher MUST NOT hardcode skill or mode logic — it reads from the
  bead's `steps` field.
- IDs use the `bd-<4-hex-chars>` format. Sub-beads use `bd-<hash>.<n>`.

### IV. Strict Layered Architecture

The codebase is organised into layers with a one-way dependency direction.
No layer may import from a layer above it.

```
api/       ──→  disp/, core/
disp/      ──→  cli/, sdk/, apiprov/, wt/, quota/, skills/, store/, core/
cli/sdk/   ──→  tmux/, core/
apiprov/   ──→  core/
wt/        ──→  core/
store/     ──→  core/
quota/     ──→  core/
skills/    ──→  core/
tmux/      ──→  (stdlib only)
core/      ──→  (stdlib only)
```

`cmd/muster/main.go` is the ONLY file permitted to import from multiple
top-level packages for wiring purposes.

**Rationale**: One-way dependencies ensure that the dispatcher can be tested
without an HTTP stack, the domain model can be validated without a store,
and transport decisions do not leak into domain code. This boundary also
isolates provider-specific code from orchestration logic, making it safe to
add or remove providers without touching the dispatcher.

**Non-negotiable rules**:
- `internal/core` MUST NOT import any package outside the Go standard
  library.
- `internal/store` MUST NOT import `internal/disp` or `internal/api`.
- `internal/disp` MUST NOT import `internal/api`.
- `internal/api` handlers MUST be thin translators: decode → call
  service/dispatcher → encode. Business logic in a handler is a violation.
- Provider-specific logic (CLI flags, auth flows, mode mappings) MUST live
  in its adapter package; it MUST NOT appear in `internal/disp`.
- Wiring happens exclusively in `cmd/muster/main.go`.

### V. Constitution-as-Code

A **constitution** is a markdown document prepended to every agent
invocation's system prompt. It encodes universal rules: commit-message
format, test discipline, dependency policy, code review standards. The
constitution active for a bead is loaded from `.muster/constitution.md` in
the target repository at dispatch time.

**Rationale**: Consistent agent behavior across runs, repos, and providers
requires rules that travel with the work. A constitution is the mechanism
by which Muster enforces project standards autonomously — without it, every
agent invocation is a blank slate that may or may not respect project norms.

**Non-negotiable rules**:
- The constitution is loaded fresh at each step dispatch; cached versions
  MUST NOT be used if the file on disk has changed.
- The constitution MUST be shown in the bead drawer UI alongside a preview
  and an edit link.
- If no constitution file exists for a target repo, the dispatcher MUST
  proceed without one — it MUST NOT inject a default constitution silently.
- Constitution content MUST NOT be modified by the dispatcher or any adapter.
  It is read-only at dispatch time.
- This file (`muster/.specify/memory/constitution.md`) is the Muster
  project's own constitution — it governs how `muster` itself is developed.

### VI. Per-Step Skills

A **skill** is a loadable capability: a tool pack, MCP server configuration,
or RAG index. Skills are applied **per-step**, not per-bead. Each step
carries its own `skills[]` field. There is no bead-level skill default.

**Rationale**: Applying the same skills to every step of a bead is the
wrong granularity. A `plan` step needs spec-system skills; a `build` step
needs code tools; a `review` step needs lint and test runners. Collapsing
these into a single bead-level list forces every step to carry irrelevant
capabilities and risks skill-context pollution between phases.

**Non-negotiable rules**:
- The `Bead.Skills` field is deprecated. New code MUST NOT read from it;
  migration appends its contents to `Step[0].Skills` on load.
- Skills are resolved from four source paths in priority order: project
  (`.agents/skills/`), user (`~/.config/agents/skills/`), builtin
  (`muster://skills/`), and URL (manually imported).
- Adding a skill to a step MUST NOT require code changes — only
  configuration in the skill registry.
- Spec-system skills (Speckit, Beads memory, OpenSpec) are visually
  distinguished in the UI but are not architecturally privileged.

### VII. Test-Driven Development (TDD)

All new behaviour MUST be driven by failing tests written **before** the
production code. The mandatory cycle is Red → Green → Refactor. Retrofitting
tests after the fact is not TDD and MUST NOT be labelled as such.

**Rationale**: A bug in the dispatcher — wrong bead dispatched, stale status
returned, quota exceeded without requeue — silently stalls developer work.
Writing tests first forces explicit acceptance criteria before any
implementation bias can shape what is tested. The race detector is mandatory
because the dispatcher, WS hub, and tmux supervisor run concurrent goroutines.

**Non-negotiable rules**:
- Tests MUST exist in a failing state before production code is written.
  Implementation phases in `plan.md` enforce this ordering per layer.
- `go test -race ./...` MUST pass at the end of every implementation phase.
- Coverage gates (per `plan.md §Test Coverage Targets`) are hard CI gates,
  not aspirational targets.
- No PR may remove test coverage without documented justification.
- Table-driven unit tests are preferred for `internal/core`, `internal/disp`,
  and adapter logic. `httptest.NewServer` integration tests are required for
  `internal/api`. WS coverage uses `coder/websocket` test dialer.

### VIII. Simplicity over Cleverness (YAGNI + DRY)

The codebase MUST favour the simplest solution that satisfies the
requirement. Complexity requires explicit justification. Logic that appears
in more than one place MUST be extracted into a single canonical location.

**YAGNI**: Do not add functionality until it is required. Speculative
generality is a liability, not an asset.

**DRY**: Every business rule, constant, or algorithm MUST have a single
unambiguous representation. Duplication between Go types and transport DTOs
is debt that MUST be tracked and resolved.

**Rationale**: Muster coordinates many moving parts (dispatcher, tmux,
adapters, WS hub, quota tracker). Unnecessary complexity in any one layer
compounds with the inherent complexity of the others. The simplest correct
implementation is always preferred.

**Non-negotiable rules**:
- New abstractions require a documented reason in the relevant RFC or PR.
- Dependencies MUST be evaluated against binary size and build-time impact.
- If a simpler alternative was considered and rejected, the reason MUST be
  recorded in the plan's Abandoned Ideas section.
- The M0 in-memory store is intentionally naive. Complexity added there
  before the M1 Beads-backed store lands is wasteful unless required by
  an M0 acceptance criterion.

### IX. Multi-Repo Routing and Cross-Repo Dependencies

Muster aggregates beads from multiple independent repositories. Routing
(which repo a bead is dispatched from/to), namespacing (collision-free IDs
across repos), and cross-repo dependencies are architectural concerns that
MUST be addressed explicitly — never as an afterthought.

**Rationale**: A bead hub that silently drops, duplicates, or conflates
beads from different repos is worse than no hub — it creates false confidence
in cross-team coordination. Multi-repo correctness is a non-negotiable
property.

**Non-negotiable rules**:
- Bead IDs MUST be namespaced by source repo so that cross-repo collisions
  are structurally impossible (e.g., `muster-` prefix routing per
  `BEADS_MULTIREPO_SETUP_GUIDE.md`).
- Routes live in `.beads/routes.jsonl`. Adding a new repo source MUST NOT
  require code changes — only route configuration updates.
- Cross-repo dependencies use the `external:` prefix
  (e.g., `external:billing-repo/bd-100`). The dispatcher MUST respect these
  when determining `bd ready` status.
- Any PR that changes aggregation or routing logic MUST include an
  integration test covering at least two distinct repo sources.
- `bd pin <id> --for <agent>` is an explicit routing override. The
  dispatcher MUST respect pinned agents before falling back to
  capacity-based assignment.

## Development Workflow

- Features start with a user-facing description of the problem (Working
  Backwards from the developer experience using `muster`).
- For milestones > 1 week: write a PRFAQ sketch → `spec.md` →
  `plan.md` → `tasks.md` under `specs/<milestone>/`.
- For features < 1 week / single component: a brief memo or PR description
  with rationale suffices.
- All work targets `main` via Pull Requests. PRs MUST reference the
  associated spec or memo where applicable.
- The CI pipeline (build, race tests, coverage, lint) MUST pass before
  merge. No exceptions without documented, time-bound justification.
- Commit messages MUST follow `<type>: <summary>` format
  (e.g., `feat: add cursor-pagination to GET /api/v1/beads`).
- Implementation phases in `plan.md` enforce the TDD ordering
  (failing test → minimum code → refactor) per layer.

## Quality Gates

Every PR MUST satisfy the following before merge:

| Gate | Requirement |
|------|-------------|
| Build | `go build ./...` produces a clean binary with no CGo |
| Tests | `go test -race ./...` passes with no failures or data races |
| Coverage | `go tool cover` meets per-package thresholds in `plan.md §Test Coverage Targets` |
| Lint | `gofmt -l .` output empty + `go vet ./...` clean + `golangci-lint run` clean |
| Dependency direction | No import from a lower layer to a higher layer (verified via `go list`) |
| Binary self-containment | For any PR touching `cmd/muster/` or `ui/`: author certifies the binary serves the UI correctly after being moved to a temp directory |
| WS correctness | For any PR touching `internal/ws/` or `internal/api/stream/`: integration test via `httptest.NewServer` + WS dial covers the changed path |
| Adapter interface | For any PR adding a new provider: a stub `Adapter` implementation in the adapter package with at least one passing unit test |
| Multi-repo correctness | For any PR touching aggregation or routing: integration test covers ≥ 2 repo sources |
| Documentation | `README.md` and CLI `--help` updated if behaviour changes |
| Complexity justified | Any new abstraction, dependency, or pattern is explained in the PR |

## Governance

This constitution supersedes all other implicit or informal practices for
the `muster` project. Amendments require:

1. A written proposal (memo or PR description) describing the change and
   its rationale.
2. Review by the project maintainer (`gitrgoliveira`).
3. A version bump per semantic versioning rules:
   - **MAJOR**: Backward-incompatible governance changes, principle
     removals, or fundamental redefinitions.
   - **MINOR**: New principle or section added, or materially expanded
     guidance.
   - **PATCH**: Clarifications, wording, or typo fixes.
4. Update of `Last Amended` date on ratification.
5. Sync impact report comment added at the top of this file listing what
   changed and which templates require updates.

All PRs and feature reviews MUST verify compliance with the principles
above. Complexity violations MUST be documented in the plan's Complexity
Tracking table before the PR is opened.

For AI-agent runtime guidance, refer to `.specify/` templates and skill
files in `.claude/skills/`.

**Version**: 1.0.0 | **Ratified**: 2026-05-24 | **Last Amended**: 2026-05-24
