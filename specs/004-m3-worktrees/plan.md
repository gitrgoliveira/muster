# Implementation Plan: M3 — Worktrees (`wt.Backend`)

**Branch**: `004-m3-worktrees` (spec dir; work on the current branch) | **Date**: 2026-07-03 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/004-m3-worktrees/spec.md`

## Summary

Promote M2's hardened `internal/worktree` git helper into a VCS-agnostic **`wt.Backend`**
abstraction with two implementations (`wt.Git`, `wt.JJ`) and expose the agent's work
product through two new read endpoints — `GET /beads/{id}/worktree` (file list) and
`GET /beads/{id}/diff[?path=]` (unified diff). The spike ([research.md](research.md))
pinned the tool contracts: `jj diff --git` is byte-compatible with `git diff`; `jj root`
detects a jj-native repo; and the git backend must include **untracked** files via
`git status --porcelain` because `git diff HEAD` omits them (kept **non-mutating** — no
`git add -N`). Backend selection is a global config default (`--default-vcs`, default
`git`) since beads has no per-bead `vcs` field; jj requires a jj-native source repo and is
refused (`VCS_UNAVAILABLE`) otherwise. The interface declares the full roadmap §8 surface;
the write-side (`Finalize`/`Push`/`Remove`) returns `ErrNotImplemented` in M3 (filled M4).

## Technical Context

**Language/Version**: Go (same toolchain as M0–M2; `go.mod` unchanged — no new modules)

**Primary Dependencies**: stdlib only (`os/exec`, `net/http`, `io`, `path/filepath`);
external runtime tools shelled out: `git` (required), `jj` ≥ 0.42 (optional)

**Storage**: none of its own (Constitution I) — worktrees live under `--worktrees-dir`
(`~/.muster/worktrees/<beadID>`), the same layout M2 established

**Testing**: `go test` with fakes-on-`$PATH` (fake `jj`, argv-recording) + real-binary
integration tests gated on `jj`/`git` presence (skip when absent); `go test -race ./...`

**Target Platform**: local loopback server (Constitution: local-first), macOS/Linux

**Project Type**: single Go project (CLI + embedded HTTP/WS server) — extends M2 layout

**Performance Goals**: diff endpoints stream (no whole-diff buffering, FR-008); no other
hot path introduced

**Constraints**: additive REST/WS surface only (Constitution V); no credential/identity
handling (Constitution — jj read+create needs no user config); diff reads must not mutate
a running agent's worktree/index

**Scale/Scope**: read + create slice only; ~1 new internal package (`internal/wt`),
2 new endpoints, additive status fields, 1 new config flag

## Constitution Check

*GATE: evaluated before Phase 0 and re-checked after design. All PASS.*

| Principle | Assessment |
|---|---|
| **I. Single Binary, Self-Contained** | ✅ No new Go modules; `jj`/`git` are shelled-out runtime deps, probed like `tmux`. No embedded VCS library. |
| **II. Beads Is Source of Truth** | ✅ Worktrees/diffs are reconstructable, disposable muster-side state; no authoritative issue state added. VCS choice is config, not persisted per-bead. |
| **III. Layered Architecture** | ✅ New capability lives in its own `internal/wt` package behind the `Backend` interface; handlers stay thin (parse → service → render); diff streaming delegated to the backend. |
| **IV. Test-First, Per-Layer Coverage** | ✅ Fakes-on-`$PATH` + skip-gated real-binary integration tests; `-race` clean; per-package gates below. |
| **V. Additive, Backward-Compatible Surface** | ✅ Two new endpoints + additive status fields; zero change to M0/M1/M2 paths/shapes/events (SC-007). |
| **Security & Orchestration** | ✅ No credentials (jj needs none for read+create); no silent VCS fallback (FR-011/FR-004a); isolation preserved (per-bead worktree); local-first. |

**Complexity Tracking**: no violations — no new modules, no new heavyweight deps, one new
internal package behind an interface. Table omitted.

## Project Structure

### Documentation (this feature)

```text
specs/004-m3-worktrees/
├── spec.md              # ✅ Phase 1 (fleet) — clarified
├── research.md          # ✅ spike — jj/git contracts pinned
├── plan.md              # ← this file
├── data-model.md        # types: Backend, VCS, FileChange, WorktreeStatus
├── quickstart.md        # end-to-end diff walkthrough (git + jj)
├── contracts/
│   ├── wt-backend.md     # the wt.Backend interface contract
│   └── http-endpoints.md # /worktree + /diff request/response contract
├── checklists/          # Phase 4 (fleet)
└── tasks.md             # Phase 5 (fleet / speckit-tasks)
```

### Source Code Layout (extends M2)

```text
internal/
├── worktree/            # M2 git helper — RETAINED (its tests are the git backend's
│   └── worktree.go      #   regression contract, SC-001); wt.Git delegates into Ensure
├── wt/                  # NEW — the VCS-agnostic abstraction
│   ├── doc.go
│   ├── wt.go            # Backend interface, VCS type, FileChange, WorktreeStatus,
│   │                    #   ErrNotImplemented, For(vcs) resolver, Detect (probe git+jj)
│   ├── git.go           # wt.Git: Create/Status wrap worktree.Ensure; DiffSummary via
│   │                    #   `git status --porcelain=v1 -z`; Diff via `git diff HEAD`
│   │                    #   (+ `--no-index` per untracked file). Finalize/Push/Remove →
│   │                    #   ErrNotImplemented.
│   ├── jj.go            # wt.JJ: Create via `jj workspace add`; DiffSummary via
│   │                    #   `jj diff --summary`; Diff via `jj diff --git [path]`;
│   │                    #   jj-native probe via `jj root`. write-side → ErrNotImplemented.
│   ├── path.go          # worktree-relative ?path= validation (shared, FR-007)
│   ├── wt_test.go       # interface/For/Detect + path-safety unit tests
│   ├── git_test.go      # fake-git + real-git integration (extends worktree testhelp)
│   └── jj_test.go       # fake-jj + real-jj integration (skip if jj absent)
├── orchestrator/
│   └── orchestrator.go  # Dispatch uses wt.Backend.Create (was worktree.Ensure);
│                        #   VCS_UNAVAILABLE refusal; worktreeCount accessor; diff accessors
├── services/
│   └── beads.go         # Worktree(id) + Diff(id, path) service methods → orchestrator/wt
├── api/
│   ├── router.go        # + GET /beads/{id}/worktree, GET /beads/{id}/diff
│   ├── beads/handlers.go# thin handlers: parse, delegate, render (summary JSON / diff stream)
│   └── health/          # + VCS availability + worktreeCount in status DTO
└── config/              # + --default-vcs / MUSTER_DEFAULT_VCS (allow-list git|jj)
cmd/muster/main.go       # probe wt.Detect at startup; wire --default-vcs into orchestrator
```

**Structure Decision**: **Wrap, don't rewrite.** `internal/worktree` is retained verbatim
so its M2 test suite remains the git backend's regression contract (SC-001); the new
`internal/wt` package owns the interface and delegates git create/reuse into
`worktree.Ensure`. This keeps the highest-risk refactor (US1) behavior-preserving and
isolates all new surface in one new package behind one interface (Constitution III).

## Phase 0 — Research (research.md) ✅ DONE

Spike against real `jj` 0.42.0 / `git` 2.55.0 pinned every command contract (§8 of
research.md). No open unknowns remain. Key decisions carried into design:
- git summary = `git status --porcelain=v1 -z` (non-mutating, includes untracked).
- git whole-diff = `git diff HEAD` + `git diff --no-index -- /dev/null <untracked>`.
- jj = `jj workspace add` / `jj diff --summary` / `jj diff --git [path]`; detect via `jj root`.

## Phase 1 — Design

### data-model.md (highlights)

`Backend` interface (full §8 surface), `VCS` enum (`git|jj`, `Valid()` allow-list),
`FileChange{Path, Kind}` with `ChangeKind` enum, `WorktreeStatus{Exists, Clean, Ahead,
Behind}`, `ErrNotImplemented` sentinel, `ErrWorktreeNotFound` (→ 404), `ErrVCSUnavailable`
(→ 412). Change-kind mapping table for git (`XY`/`R###`) vs jj (`M/A/D/R/C`).

### contracts/

- `wt-backend.md` — the Go interface contract: method semantics, which are implemented vs
  `ErrNotImplemented` in M3, and the per-backend command each delegates to.
- `http-endpoints.md` — `/worktree` (200 summary JSON / 404 `WORKTREE_NOT_FOUND`) and
  `/diff` (200 `text/x-diff` stream / 404 / 400 on bad `?path=`), plus the additive status
  DTO fields. Both marked additive over the M2 contract.

### Agent context update

No CLAUDE.md/runtime-agent change needed; README + quickstart get the two endpoints, the
`--default-vcs` flag, and the `jj` optional-dependency note (task in Polish phase).

## Phase 2 — Tasks (`speckit-tasks`)

Tasks will follow the M2 shape: setup (fakes, skeletons) → US1 (interface + git wrap,
behavior-preserving, TDD) → US2 (diff endpoints) → US3 (jj backend) → US4 (status) →
polish (docs, coverage, `-race`). US3 is structured to be **droppable** without touching
US1/US2 (per the clarification). `[P]` groups: the three `internal/wt` files and the two
contract docs are independent; endpoint handler + router touch shared files → sequential.

### Per-package coverage gates (plan targets)

| Package | Gate | Rationale |
|---|---|---|
| `internal/wt` | ≥ 85% | core new logic; parsers + path safety must be exercised |
| `internal/worktree` | unchanged (M2 gate) | retained; no regression permitted |
| `internal/api/beads` | ≥ M2 gate | two new handlers covered incl. 404/400 paths |
| `internal/config` | ≥ M2 gate | `--default-vcs` allow-list + reject-invalid |
| `internal/services`, `internal/api/health` | ≥ M2 gate | additive methods/fields covered |

## Complexity Tracking

No constitution violations to justify. (No new modules; one new internal package behind an
interface; no heavyweight transitive deps.)

## Key Design Decisions & Rationale

| Decision | Why | Alternative rejected |
|---|---|---|
| Wrap `internal/worktree` rather than rename to `internal/wt` | Preserves M2's hardened tests as the git regression contract (SC-001); minimizes churn on the M2 dispatch path | Rename/move the package — touches every importer and risks silent behavioral drift in the highest-risk refactor |
| git summary via `git status --porcelain` (not `git diff --name-status HEAD`) | Includes **untracked** files (agent's new files) without mutating the index; one command | `git add -N` + `git diff` — mutates the index, can race a running agent |
| `jj diff --git` for the unified diff | Byte-compatible with `git diff`; one media type across backends (spike-verified) | jj's native colored/format diff — not git-apply-able, backend-detectable by client |
| Interface declares write-side now, `ErrNotImplemented` in M3 | Stable roadmap-§8 contract; M4 fills it without changing callers | Omit until M4 — a later interface change ripples through `For`/handlers |
| VCS = global config default, no per-bead field | beads has no `vcs` column (same constraint as M2's missing `review`); avoids an M1 model change | Persist per-bead VCS — out of scope; requires bead-model change |
| jj source repos only (no auto-colocate) | Keeps scope bounded; `jj git init` on the user's repo is a surprising side effect | Auto-colocate on `vcs=jj` — mutates the user's repo layout implicitly |
