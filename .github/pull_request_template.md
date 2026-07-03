<!--
Title: conventional-commit style, e.g.
  feat(m3): short imperative summary
  fix(orchestrator): ...
  docs(m2): ...
  chore: ...
Scope is usually the milestone (m3) or the package (orchestrator, wt, api).
-->

## Summary

<!-- Bullet points: what changed and why it matters. Lead with user-visible or API behavior. -->

-
-

<!-- Optional: include when it helps reviewers:

## Why

Motivation / root cause. For fixes, state the root cause explicitly.

-->

## Files changed

<!-- REQUIRED. One row per file touched (or per package for large sweeps). -->

| File | What |
|---|---|
| `path/to/file.go` | one-line description |

## API / surface impact

<!--
REQUIRED. muster's public surface is additive and backward-compatible
(Constitution V). State one of:
  - "No surface change." — internal only, and
  - Any new/changed HTTP route, status/DTO field, WS event type, or CLI flag,
    with a note confirming existing M0/M1/M2+ surface is unchanged.
-->

-

## Screenshots

<!--
REQUIRED only when the embedded UI (`cmd/muster/ui/`) changes: layout,
visual, copy, or behavior. Attach before/after images. Delete this section
if the change has no UI impact.
-->

## Test plan

<!--
Every applicable box must be checked before the PR is ready. Don't just read
the diff — run the gates. Tests-first is NON-NEGOTIABLE (Constitution IV).
-->

- [ ] `make test` passes (`go test -race ./...`, all packages green)
- [ ] `make cover-check` passes (per-package coverage gates met)
- [ ] `make lint` clean (`go vet` + `gofmt -l .` no drift)
- [ ] New/updated unit tests cover the change; fakes-on-`$PATH` + skip-gated real-binary integration where an external CLI is involved
- [ ] Behavior-preserving refactors keep prior-milestone suites green (e.g. M2 suite for orchestrator changes)
- [ ] `.specify/memory/constitution.md` principles upheld (single binary; beads is source of truth; thin handlers / logic behind interfaces; additive surface)
- [ ] Docs updated: `README.md` (flags/API tables) and the relevant `specs/NNN-*/` artifacts for any new or changed flag, endpoint, or behavior
- [ ] No new console errors or warnings (for embedded-UI changes)

## Tracking

<!--
muster's own development is tracked in specs/ (milestones), not the beads DB
(which tracks a different project). Link the milestone/spec this PR advances.
-->

Relates to: `specs/NNN-milestone/`
