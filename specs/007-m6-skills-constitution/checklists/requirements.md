# Requirements & Constitution-Compliance Checklist: M6 — Skills & Constitution

**Purpose**: Validate the *quality* of the M6 requirements (completeness, clarity, consistency, measurability, coverage) and their alignment with the Muster Constitution — before implementation. This is a unit-test suite for the spec, not for the code.
**Created**: 2026-07-07
**Feature**: [spec.md](../spec.md) · [plan.md](../plan.md) · [research.md](../research.md)
**Depth**: Thorough · **Audience**: Reviewer (pre-implementation gate)

## Requirement Completeness

- [ ] CHK001 Are the required *contents* of `defaultPromptFor(mode)` specified per mode, or only that the function exists? [Gap, Spec §FR-002]
- [ ] CHK002 Is the exact assembled-prompt template inlined or referenced precisely enough to verify SC-001 byte-for-byte (rather than only pointing at handoff §9)? [Completeness, Spec §SC-001]
- [ ] CHK003 Is the base directory that `.muster/constitution.md` and imported skills resolve to unambiguously specified (absolute vs cwd-relative vs `~/.muster`)? [Gap, Spec §FR-008/FR-016]
- [ ] CHK004 Does the spec specify the accepted format/schema of a skill imported via URL (front-matter fields, body semantics)? [Gap, Spec §FR-014]
- [ ] CHK005 Is the discovery/location of "the agent's own existing MCP configuration" specified for the best-effort check? [Gap, Spec §FR-021]
- [ ] CHK006 Are requirements defined for how a bead's `skill:<id>` labels are *read* by muster (given the write path rejects labels)? [Completeness, Spec §FR-018]
- [ ] CHK007 Is the derivation and maximum length of an "earlier-step summary" specified, beyond "a one-line summary from runlog"? [Completeness, Spec §FR-004]
- [ ] CHK008 Are the built-in skill catalog's minimum guarantees (non-empty, enumerable, each with all §3.3 fields) stated as requirements rather than left to seed data? [Completeness, Spec §FR-012]
- [ ] CHK009 Is the response shape for every new endpoint (`/constitution`, `/skills*`, `/memories*`) fully specified, including the fresh-install/empty cases? [Completeness, Spec §FR-007/013/023]

## Requirement Clarity & Measurability

- [ ] CHK010 Is "bounded" (skill-import timeout + size cap) quantified with specific values? [Ambiguity, Spec Edge Cases / §FR-017]
- [ ] CHK011 Is "best-effort" MCP verification defined precisely enough to be objectively verifiable (what counts as "checked")? [Clarity, Spec §FR-021]
- [ ] CHK012 Is the constitution's *initial* `Version` value unambiguously specified, or does the spec still offer "0 (or 1)"? [Ambiguity, Spec §US2 AS1]
- [ ] CHK013 Can "the assembled prompt's header is exactly that markdown" be objectively measured (whitespace/version-placement rules defined)? [Measurability, Spec §US1 AS1]
- [ ] CHK014 Is "the *next* dispatch (not any already-running step)" defined with a precise cut-over point relative to prompt-file write time? [Clarity, Spec §FR-009]
- [ ] CHK015 Are the auto-derived memory key rules specified when `POST /memories` omits a key? [Clarity, Spec §US5 AS1]

## Requirement Consistency & Conflicts

- [ ] CHK016 Does FR-018's "explicit per-step/per-dispatch selection *taking precedence*" reconcile clearly with the same requirement's "union, not override"? [Conflict, Spec §FR-018]
- [ ] CHK017 Are "skills are muster operating config, not issue state" (Clarification 1) and "skill *selection* lives in `bd` labels" (issue metadata) reconciled without contradiction? [Consistency, Spec §Clarifications/§FR-018]
- [ ] CHK018 Is the constitution `Version` "embedded/referenced in the assembled prompt" (FR-007/§3.4) described consistently with the prompt template that shows only `${constitution.Markdown}`? [Consistency, Spec §FR-007]
- [ ] CHK019 Do the "no silent default" postures for unresolvable skills (FR-020) and missing MCP servers (FR-021) use a consistent, defined warning surface? [Consistency, Spec §FR-020/021]
- [ ] CHK020 Are the memory endpoints in the spec body (§US5) consistent with the handoff §4.1 surface (`?repo=`, `Idempotency-Key`) the contracts reference? [Consistency, Spec §US5]

## Scenario & Edge-Case Coverage

- [ ] CHK021 Are requirements defined for a malformed `skill:` label value (empty id, `skill:a:b`, whitespace)? [Edge Case, Gap]
- [ ] CHK022 Are requirements defined for concurrent skill import/delete of the same id (race), beyond the single-actor collision rule? [Coverage, Gap]
- [ ] CHK023 Is the behavior specified when the resolved skill loadout is very large / unbounded (many `skill:*` labels)? [Edge Case, Gap]
- [ ] CHK024 Are recovery requirements defined for a corrupt/unreadable constitution file (fallback + surfaced warning)? [Recovery, Spec Edge Cases]
- [ ] CHK025 Is the earlier-step-summary requirement explicit that a *failed* prior step is included and labelled, not omitted? [Coverage, Spec Edge Cases]
- [ ] CHK026 Are requirements defined for `bd` running in `--readonly`/sandbox mode when a `/memories*` call is made? [Exception Flow, Spec Edge Cases]
- [ ] CHK027 Is the empty/degenerate assembly path (no constitution, no skills, no prior steps) specified to still produce a well-formed prompt? [Coverage, Spec §FR-005]

## Non-Functional Requirements

- [ ] CHK028 Does the spec state security requirements (scheme allowlist / SSRF protection) for the server-side skill-URL fetch, or is it left implicit in "bounded"? [Gap, Security]
- [ ] CHK029 Are performance/latency expectations for reading labels at dispatch time specified, or explicitly declared out of scope? [Gap, NFR]
- [ ] CHK030 Are observability requirements (where the skip/warning signals appear: runlog line vs WS event vs status field) specified rather than left as "and/or"? [Clarity, Spec §FR-020]

## Dependencies & Assumptions (Constitution "verify before building")

- [ ] CHK031 Is the assumption that `bd label` "is real, wired, and already round-trips through `store/bdshell`" validated against the code (research found labels are NOT surfaced read-side)? [Assumption, Spec §Clarifications]
- [ ] CHK032 Is the assumption that `bd` provides `remember/recall/forget/memories` with the assumed CLI/JSON shape validated by a spike against the real `bd`, not trusted from prose? [Assumption, Spec §US5]
- [ ] CHK033 Is the cited precedent "the already-existing `.muster/config.toml` (M1–M4)" verified to exist (research found it does not)? [Assumption, Spec §Clarifications]
- [ ] CHK034 Is the soft, non-blocking dependency on M5 (multi-provider) documented with the fallback that M6 validates against `claude` only? [Assumption, Spec §Dependencies]

## Constitution Compliance (requirements-level)

- [ ] CHK035 Are the additive-only guarantees (no M0–M5 route/shape/WS-event changed or removed) stated as verifiable requirements with a prior-suite check? [Completeness, Spec §FR-027/SC-008 — Principle V]
- [ ] CHK036 Is the "no new Dolt table / nothing routed around `bd`; memories through `bd`" storage boundary stated as a requirement for every new persistent artifact? [Completeness, Spec §FR-028 — Principle II]
- [ ] CHK037 Is the thin-handler split (validate→service→render; assembly/registry/`bd`-shelling in service/orchestrator) stated as a requirement? [Completeness, Spec §FR-030 — Principle III]
- [ ] CHK038 Are the test-first, per-layer coverage-gate, `-race`-clean, and fake-on-`$PATH`+real-binary requirements stated for every new external-tool/URL path? [Completeness, Spec §FR-029 — Principle IV]

## Notes

- Check items off as completed: `[x]`
- Items CHK031 / CHK033 already have known answers from Phase 0 research: both assumptions were **false** — the spec's rationale prose is optimistic; the plan compensates (read labels via `bd`; resolve `<musterDir>` from the `~/.muster` precedent). Flag these for spec-prose cleanup, not a design change.
- CHK012 / CHK016 are residual spec ambiguities the plan pinned (version=0; union-with-precedence) but the spec text still reads loosely — candidates for a spec tidy-up pass.
- Traceability: 38/38 items carry a `[Spec §…]` reference or a `[Gap]/[Ambiguity]/[Conflict]/[Assumption]` marker.
