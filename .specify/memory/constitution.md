# Muster Constitution

<!--
  Ratified 2026-05-29. Principles are derived from what M0 and M1 actually
  enforced (see prototype/handoff/spec.md and specs/00{1,2}-*/plan.md
  "Constitution Check" sections), not aspirations. Amend via the Governance
  process below; bump the version and record the change.
-->

## Core Principles

### I. Single Binary, Self-Contained

Muster ships as **one Go binary** with the UI embedded (`go:embed`). It owns **no database of its own** and spawns **no daemons**. External tools it depends on (`bd`, `dolt`, `tmux`, `git`, the agent CLIs) are **runtime dependencies that are shelled out to and probed at startup** — never linked into the binary, never bundled. Adding a Go module that pulls a heavyweight transitive graph (e.g. embedding a database engine) is a constitution-level decision requiring justification in the plan's Complexity Tracking.

*Rationale*: keeps the binary small, the deployment a single artifact, and the dependency surface auditable. This is why M1 reads `issues.jsonl`/MySQL wire instead of importing Dolt.

### II. Beads Is the Source of Truth

The beads database is the canonical store of issue state; **muster is infrastructure over it, not the owner of it**. Muster MUST NOT hold authoritative, durable issue state of its own. Any muster-side state (caches, in-flight runs, transient runlogs) is reconstructable and disposable. Writes to issue state go through the authoritative writer (the `bd` CLI in v1), not around it.

*Rationale*: the product vision is "beads-central" — the data is the application; everything else is configuration. Two writers to one truth corrupt it.

### III. Layered Architecture — Handlers Stay Thin

Code is layered `core` (pure domain types) → `store` (backends) → `services` (business logic) → `api` (HTTP/WS handlers). **Handlers contain no business logic**; they parse, delegate to a service, and render. New cross-cutting capabilities (e.g. orchestration) live in their own packages behind interfaces, not inlined into handlers or the store.

*Rationale*: makes backends swappable (M1 swapped the store with zero handler change) and keeps tests targeted per layer.

### IV. Test-First, Per-Layer Coverage (NON-NEGOTIABLE)

TDD is mandatory: tests are written and failing before implementation, per layer. Every package carries an explicit coverage gate in the plan, enforced in CI, and `go test -race ./...` MUST pass clean. External-tool integrations are tested with fakes on `$PATH` (the fake-`bd` pattern) plus an integration test gated on the real tool's presence.

*Rationale*: this is an agent-built codebase across many sessions; the test gate is the contract that prevents drift.

### V. Additive, Backward-Compatible Surface

The public REST endpoints and the WebSocket event protocol evolve **additively across milestones**. A milestone MUST NOT break the shapes, paths, or event types a prior milestone shipped (M1 FR-014, M2 FR-019). New endpoints and event types are fine; changing or removing existing ones requires an explicit, versioned migration decision.

*Rationale*: the embedded UI and any external client depend on a stable surface; milestones are additive PR series, not rewrites.

## Additional Constraints — Security & Orchestration

- **No credential handling.** Muster detects auth state but never performs, proxies, stores, or logs credentials for the tools it drives (M2 FR-016). Login is the user's out-of-band action.
- **User-controlled autonomy.** When muster runs an autonomous agent, the autonomy/permission level is **supplied by the user**, never silently defaulted by muster (M2 FR-021). The most dangerous mode is never the implicit one.
- **Adapter-agnostic orchestration.** Agent execution sits behind interfaces (`Adapter`, transport) so providers and transports are swappable; no single vendor's CLI shape leaks into the core (handoff §7).
- **Isolation.** Agent work happens in a per-bead isolated worktree, never the user's main checkout.
- **Local-first.** v1 targets local loopback; multi-user/hosted concerns are explicitly out of scope (handoff §21 v2).

## Development Workflow — Spec-Driven Delivery

- Work proceeds **spec → plan → tasks → implementation** (the `specs/NNN-*/` flow). Scope cuts and deferrals between milestones are stated explicitly in the spec.
- **Verify assumptions empirically before building.** External contracts (CLI flags, output shapes, wire formats) are pinned by a spike against the real tool before the plan finalizes them — not trusted from prose. (M1 Phase 7.5; M2 2026-05-29 spike, which caught the stale `claude --plan` flag.)
- Each milestone is a self-contained PR series gated by its named test layer. Changes are committed and **pushed** at session end; work is not done until pushed.
- Persistent cross-session knowledge lives in beads (`bd remember`), not scattered markdown.

## Governance

This constitution supersedes ad-hoc practice. Every plan's **Constitution Check** gate evaluates the feature against these principles; violations MUST be justified in the plan's Complexity Tracking or the feature is revised. Amendments require: a stated rationale, a version bump per the policy below, and an update to any dependent template wording that references a changed principle.

**Versioning policy** (semantic): **MAJOR** = remove/redefine a principle in a backward-incompatible way; **MINOR** = add a principle or materially expand guidance; **PATCH** = clarifications and wording.

**Version**: 1.0.0 | **Ratified**: 2026-05-29 | **Last Amended**: 2026-05-29
