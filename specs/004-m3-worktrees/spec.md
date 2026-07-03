# Feature Specification: M4 — Worktrees (`wt.Backend`)

**Feature Branch**: `004-m3-worktrees` (spec dir; work proceeds on the current branch per operator choice)

**Created**: 2026-07-02

**Status**: Draft

**Input**: Promote the minimal per-bead git-worktree helper pulled forward in M2 (`internal/worktree`) into the full **`wt.Backend`** abstraction from the canonical roadmap (`prototype/handoff/spec.md` §8, milestone **M3 — Worktrees**). Deliver: (1) a single `wt.Backend` interface so the orchestrator and the new diff endpoints never branch on VCS; (2) a **git** backend that preserves M2's existing `Ensure` create/reuse/isolation semantics; (3) a **jj** (Jujutsu) backend as a second concrete implementation, selected per-bead; (4) **diff exposure** — the two new read endpoints `GET /beads/{id}/worktree` (file list + change summary) and `GET /beads/{id}/diff[?path=]` (unified diff); (5) startup probing of both VCS binaries surfaced in orchestrator status, with `VCS_UNAVAILABLE` handling when a bead's chosen backend is absent. Scope is deliberately bounded to the **read + create** side of the worktree lifecycle: the write-side finalize/push, worktree GC, and sub-bead worktree ownership are later milestones (M4/M8/M9) and are out of scope here.

> **Naming note:** the roadmap labels this milestone **M3**. The spec directory is numbered `004-*` to continue the existing `001/002/003` sequence (M0/M1/M2). "M3" and directory `004-m3-worktrees` refer to the same milestone throughout.

## Clarifications

### Session 2026-07-03

- **Q: jj is colocated-on-git and has no consumer yet — keep the jj backend in M3, or descope it?** → A: **Keep jj in M3.** Build `wt.JJ` as a second concrete backend now, matching the roadmap's M3 = jj + git pairing. The `wt.Backend` interface stays VCS-agnostic so both backends share the diff endpoints and status.
- **Q: How does the jj backend create a worktree when the source repo is a plain git repo?** → A: **jj source repos only.** The jj backend requires a jj-native (or jj-colocated) source repo; a plain-git source selected as `vcs=jj` is refused (not auto-colocated). Git remains the path for git source repos. This bounds the jj surface to genuinely-jj repositories.
- **Q: beads has no per-bead `vcs` field — where does a bead's VCS choice come from in M3?** → A: **Config default only** (`--default-vcs` / `MUSTER_DEFAULT_VCS`, allow-list `git|jj`, defaulting to `git`). This mirrors M2's workaround for the missing `review` column: muster does not persist per-bead VCS state (Constitution II — beads is the source of truth; muster holds no durable issue state). Per-bead override is deferred until a bead-model `vcs` field exists (a later milestone). Consequently FR-015's post-worktree immutability is **not applicable in M3** (there is nothing per-bead to mutate) and is deferred with the field.
- **Q: Should the M3 `wt.Backend` interface declare the write-side methods (Finalize/Push/Remove) now, or omit them until M4?** → A: **Declare all now.** The interface matches roadmap §8 verbatim including `Finalize`/`Push`/`Remove`, giving M4 a stable contract. M3's implementations of those three return a sentinel `ErrNotImplemented`; only `Status`/`Create`/`DiffSummary`/`Diff` are implemented and tested in M3.
- **Q: When a bead has never been dispatched (no worktree exists), what do the diff/worktree endpoints return?** → A: **404 Not Found.** "No worktree" is modelled as the resource not existing, distinct from a worktree that exists but is clean (which returns 200 with an empty summary/diff).

## Clarifications

### Session 2026-07-03

- **Q: jj works on top of git — is supporting jj even worth it in M3?** → A: **Yes, keep it — but structured so it's cheap to cut.** The spike ([research.md](research.md)) settled the technical worth: `jj diff --git` is byte-compatible with `git diff`, and jj auto-snapshots the working copy (sidestepping the git untracked-files gotcha), so the second backend is genuinely *cheap* once the interface exists. Its strategic value is the roadmap's original intent — jj is the **proof the `wt.Backend` abstraction is real and not git-shaped**; building the interface with only one implementation risks a git-shaped seam that M4/M5 pay for. **Risk control**: jj stays **User Story 3 (P2)** — independently testable, gated on the `jj` binary, and removable without touching US1/US2. If mid-implementation jj proves not worth the marginal cost, US3 can be dropped and M3 still ships a complete git-only interface + diff milestone. (This is the answer to "is it worth it": the architecture makes including it cheap and dropping it clean.)
- **Q: Where does a bead's VCS choice come from (beads has no `vcs` field)?** → A: **A config default only** (`--default-vcs` / `MUSTER_DEFAULT_VCS`, default `git`) for M3 — the same "work around the missing bead field via config" pattern M2 used for the absent `review` column. Per-bead override (dispatch-request field or bead label) is deferred until a bead-model VCS field exists. Because jj requires a jj-native source repo (below), `default_vcs=jj` is only meaningful when the mapped repo is jj-native.
- **Q: How does the jj backend create a worktree for a git source repo?** → A: **jj source repos only.** muster does NOT run `jj git init` to colocate a plain git repo. The jj backend probes `jj root`; if the source repo is not jj-native, a `vcs=jj` dispatch is refused (`VCS_UNAVAILABLE`) rather than colocated or silently falling back to git. Auto-colocate is kept in reserve for a later milestone.
- **Q: Does the M3 `wt.Backend` interface declare the write-side methods (Finalize/Push/Remove) now, or omit them until M4?** → A: **Declare all now**, matching the roadmap §8 interface verbatim. M3 implements the read+create methods (`Status`, `Create`, `DiffSummary`, `Diff`); the write-side methods (`Finalize`, `Push`, `Remove`) exist on the interface but return a documented `ErrNotImplemented` sentinel in M3, so M4 can fill them without changing the contract.
- **Q: When a bead has never been dispatched (no worktree), what do the diff/worktree endpoints return?** → A: **`404 Not Found`** with a clear code (e.g. `WORKTREE_NOT_FOUND`), distinguishing "no worktree" from "worktree exists but clean" (which is a `200` with an empty summary).

## User Scenarios & Testing *(mandatory)*

M3 is the point where a dispatched agent's file changes stop being a black box. M0/M1 were a read/CRUD data server; M2 introduced process orchestration and streamed the agent's *terminal output*; M3 exposes the agent's *work product* — the files it changed and the diffs — through a VCS-agnostic interface that also admits a second backend (jj) alongside git. To keep the leap bounded, M3 is the **read + create** slice: create/reuse a per-bead worktree (already true for git in M2), report its status, and expose its diff — deliberately excluding the finalize/commit/push write path (M4), worktree garbage collection (M9), and sub-bead worktree ownership (M8).

### User Story 1 - Behavior-preserving `wt.Backend` interface over git (Priority: P1)

The orchestrator stops calling the concrete `worktree.Ensure` directly and instead goes through a `wt.Backend` interface whose git implementation preserves M2's exact create/reuse/isolation/safety semantics. A dispatch that worked in M2 works identically in M3, now routed through the abstraction.

**Why this priority**: This is the foundational refactor every other story depends on — diff exposure and the jj backend both require the interface to exist. It carries the highest regression risk (it touches the M2 dispatch path) and must land first, fully green, with zero behavioral change. It is the MVP: shipping only this delivers the interface seam with no user-visible change but unblocks everything else.

**Independent Test**: Run the full M2 dispatch integration path (dispatch → worktree created on `muster/<beadID>` → agent runs in the worktree → exit recorded) entirely through `wt.Backend`; assert the M2 worktree tests (create, reuse, non-git-repo error, two-bead isolation, symlink refusal, wrong-branch refusal) still pass verbatim against the git backend.

**Acceptance Scenarios**:

1. **Given** a git repo and a bead ID, **When** the orchestrator dispatches through `wt.Backend.Create`, **Then** a linked worktree is created on branch `muster/<beadID>` at the per-bead path, identical to M2's `worktree.Ensure`.
2. **Given** an existing valid per-bead worktree, **When** `Create` is called again, **Then** it is reused (not recreated) — after the same repo-match, branch-match, and symlink guards M2 enforced.
3. **Given** a path that is a symlink, a non-worktree directory, a worktree linked to a different repo, or one on the wrong branch, **When** `Create`/`Status` runs, **Then** it is refused with the same errors M2 produced.
4. **Given** `repoPath` is not a git repository, **When** `Create` runs, **Then** it returns the M2 "not a git repo" error.

---

### User Story 2 - Diff exposure: file list + unified diff (Priority: P1)

An operator (or the UI, or an external client) can see what a dispatched agent changed in its worktree: a summary list of changed files with change kinds, and the unified diff for the whole bead or a single file — without attaching to the tmux session.

**Why this priority**: This is the headline, user-visible value of M3 — the roadmap names it explicitly ("diff exposure, file list"). It is the reason the abstraction is worth building. It is independently testable and demonstrable the moment the git backend and the two endpoints exist, even before jj lands.

**Independent Test**: Dispatch (or simulate) an agent that edits, adds, and deletes files in a git worktree; call `GET /beads/{id}/worktree` and assert the returned file list contains each path with the correct change kind; call `GET /beads/{id}/diff` and assert it returns the unified diff; call `GET /beads/{id}/diff?path=<f>` and assert it returns only that file's diff.

**Acceptance Scenarios**:

1. **Given** a worktree with changed files, **When** `GET /api/v1/beads/{id}/worktree` is called, **Then** it returns the parsed change summary (per-file path + change kind: added/modified/deleted/renamed) from the backend's `DiffSummary`.
2. **Given** a worktree with changes, **When** `GET /api/v1/beads/{id}/diff` is called, **Then** it streams the unified diff for the whole worktree.
3. **Given** a `?path=<file>` query, **When** `GET /api/v1/beads/{id}/diff?path=<file>` is called, **Then** it streams the unified diff for only that file.
4. **Given** a bead with no worktree (never dispatched), **When** either endpoint is called, **Then** it returns a clear "no worktree" response rather than a server error.
5. **Given** a `?path=` value that escapes the worktree (e.g. `../`), **When** the diff endpoint is called, **Then** the path is rejected (no traversal, no arbitrary-file disclosure).

---

### User Story 3 - Second backend: jj (Jujutsu), selected per-bead (Priority: P2)

A bead can use **jj** instead of git for its worktree. The same `wt.Backend` interface drives both; the dispatcher, status, and diff endpoints never branch on the backend. `wt.For(bead)` returns the correct concrete implementation.

**Why this priority**: jj is the roadmap's proof that the abstraction is real and not git-shaped — but the product is fully usable with git alone, so it ranks below the interface and diff exposure. It is independently testable against a real `jj` binary (skipped when absent, mirroring the tmux pattern).

**Independent Test**: With `jj` on `$PATH`, create a jj worktree for a bead via `wt.For` → `Create`, change files, and assert `DiffSummary`/`Diff` return correct results through the same interface and endpoints used for git; assert the git path is unaffected.

**Acceptance Scenarios**:

1. **Given** a bead whose selected VCS is `jj` and `jj` is installed, **When** `Create` runs, **Then** a jj worktree/workspace is created for the bead.
2. **Given** a jj worktree with changes, **When** the diff endpoints are called, **Then** they return correct file list + diffs via `jj diff --summary` / `jj diff` — same DTO shapes as git.
3. **Given** a bead whose selected VCS is `jj` but `jj` is **not** installed at dispatch time, **When** dispatch runs, **Then** it is refused with `VCS_UNAVAILABLE` (412) rather than silently falling back to git.
4. **Given** `wt.For(bead)`, **When** the bead's VCS is `git` vs `jj`, **Then** it returns `wt.Git{}` vs `wt.JJ{}` respectively.

---

### User Story 4 - VCS availability & worktree status in orchestrator status (Priority: P3)

`GET /api/v1/orchestrator/status` reports which VCS backends are available (probed at startup) and how many worktrees currently exist, so a client can render the VCS picker and worktree count without guessing.

**Why this priority**: Observability polish that makes stories 1–3 legible to a UI, but not required for the core create/diff capability to function. Additive to the M2 status DTO (Constitution Principle V).

**Independent Test**: Start with git present and jj absent (and vice versa); assert the status DTO reports each backend's availability accurately and a `worktreeCount` consistent with the number of per-bead worktrees on disk.

**Acceptance Scenarios**:

1. **Given** git present and jj absent at startup, **When** `GET /orchestrator/status` is called, **Then** it reports git available, jj unavailable.
2. **Given** N per-bead worktrees exist, **When** status is called, **Then** `worktreeCount` reflects N.
3. **Given** the M2 status fields, **When** M3 adds VCS/worktree fields, **Then** all M2 fields remain present and unchanged (additive surface).

---

### Edge Cases

- **Bead never dispatched (no worktree)**: the endpoints return **404 Not Found** (never a 500). This is distinct from a worktree that exists but is clean, which returns 200 with an empty summary/diff.
- **`?path=` traversal / absolute path / symlink** in the diff endpoint: rejected; the endpoint must not disclose files outside the worktree.
- **Binary / very large diffs**: the diff endpoint streams rather than buffering the whole diff in memory; binary files appear in the summary but their content diff is elided per the backend's default (`git diff`/`jj diff` behavior).
- **VCS binary missing at startup vs at dispatch**: absent-at-startup is reflected in status and the picker; absent-at-dispatch for a bead that requires it yields `VCS_UNAVAILABLE` (no silent git fallback).
- **jj selected for a plain-git source repo**: refused (jj source repos only, per Clarifications) — muster does not auto-colocate a git repo into jj; the dispatch surfaces a clear error rather than falling back to git.
- **Concurrent diff read while the agent is actively writing**: the diff is a best-effort snapshot; a read mid-write reflects whatever is on disk at read time (no locking guarantee in M3).
- **VCS selection**: comes from the `--default-vcs` config value (`git|jj`, default `git`); muster persists no per-bead VCS state (Constitution II). Per-bead override awaits a bead-model field in a later milestone.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST expose a single `wt.Backend` interface that the orchestrator and diff endpoints depend on, so no caller branches on the concrete VCS. The interface MUST cover, at minimum, the read + create surface: worktree status, create/reuse, change summary (file list), and per-file/whole unified diff.
- **FR-002**: The system MUST provide a **git** `wt.Backend` implementation that preserves M2's `worktree.Ensure` semantics exactly: create via `git worktree add -b muster/<beadID>`, reuse an existing valid worktree, and enforce every M2 guard (single-local-segment beadID, non-git-repo error, symlink refusal, repo-match, expected-branch, context-bounded subprocesses, `0o700` dirs, half-create cleanup).
- **FR-003**: The system MUST provide a **jj** `wt.Backend` implementation exposing the same interface, so diff exposure and status work identically across backends.
- **FR-004**: The system MUST route backend selection through a single resolver (`wt.For(vcs)` / `wt.For(bead)` or equivalent) that returns the correct concrete backend for the effective VCS, which in M3 is sourced from the `--default-vcs` config value (`git|jj`, default `git`); muster persists no per-bead VCS state.
- **FR-004a**: The jj backend MUST require a jj-native (or jj-colocated) source repo; selecting `vcs=jj` for a plain-git source repo MUST be refused with a clear error, never auto-colocated or silently run as git.
- **FR-005**: The system MUST add `GET /api/v1/beads/{id}/worktree` returning the parsed change summary (list of `{path, changeKind}`) from the backend's `DiffSummary`.
- **FR-006**: The system MUST add `GET /api/v1/beads/{id}/diff` returning the unified diff for the whole worktree, and MUST honor an optional `?path=<file>` query to return only that file's diff.
- **FR-007**: The diff endpoints MUST reject a `?path=` value that is absolute, contains `..`, or otherwise escapes the worktree, without disclosing any file outside the worktree.
- **FR-008**: The diff endpoints MUST stream their output rather than buffering an entire diff in memory, so a large diff does not exhaust server memory.
- **FR-009**: When a bead has no worktree (never dispatched), the endpoints MUST return **404 Not Found**, distinct from a 200 empty summary/diff for a worktree that exists but is clean.
- **FR-010**: The system MUST probe both VCS binaries (`git`, `jj`) at startup and record which are available; `git` is assumed present (M2 already requires it at run time), `jj` is optional.
- **FR-011**: When a bead's selected VCS backend is not installed at dispatch time, the system MUST refuse the dispatch with a `VCS_UNAVAILABLE` (412) condition rather than silently falling back to another backend (Constitution: user-controlled behavior, no silent defaults).
- **FR-012**: The orchestrator status DTO MUST additively report VCS backend availability and a current `worktreeCount`, leaving every M2 status field intact (Constitution Principle V — additive surface).
- **FR-013**: The change summary MUST classify each file's change kind (at least: added, modified, deleted, renamed) from the backend's native summary output (`git diff --name-status` / `jj diff --summary`).
- **FR-014**: The new endpoints and any new WS event types MUST be additive; no M0/M1/M2 endpoint, path, or event shape may change (Constitution Principle V).
- **FR-015**: *(Deferred)* Per-bead VCS immutability-once-a-worktree-exists is **not applicable in M3** — muster stores no per-bead VCS field to mutate (selection is a global config default). This requirement is deferred to the milestone that introduces a bead-model `vcs` field.
- **FR-016**: All external-tool interactions (`git`, `jj`) MUST be tested with fakes-on-`$PATH` plus a real-binary integration test that skips when the binary is absent (Constitution Principle IV).
- **FR-017**: The `wt.Backend` interface MUST declare the full roadmap §8 surface including the write-side methods `Finalize`, `Push`, `Remove`; in M3 those three MUST return a sentinel `ErrNotImplemented` (implemented in M4), while `Status`/`Create`/`DiffSummary`/`Diff` are fully implemented and tested.
- **FR-018**: The system MUST accept a `--default-vcs` flag / `MUSTER_DEFAULT_VCS` env (allow-list `git|jj`, default `git`) and reject any other value at startup.

### Key Entities *(include if feature involves data)*

- **`wt.Backend`**: the VCS-agnostic interface, declaring the **full** roadmap §8 surface. Implemented + tested in M3: `Status(beadID) → WorktreeStatus`, `Create(beadID, srcRepo) → path`, `DiffSummary(beadID) → []FileChange`, `Diff(beadID, path) → stream`. Declared but returning `ErrNotImplemented` in M3 (implemented M4): `Finalize(beadID, msg)`, `Push(beadID)`, `Remove(beadID)`.
- **`WorktreeStatus`**: whether a bead's worktree exists, whether it is clean/dirty, and (optionally) ahead/behind counts.
- **`FileChange`**: one changed file — `{path, changeKind}` where changeKind ∈ {added, modified, deleted, renamed}.
- **`VCS`**: the per-bead backend selector, `"git" | "jj"`.
- **`wt.Git` / `wt.JJ`**: the two concrete `wt.Backend` implementations. `wt.Git` is the promoted M2 `internal/worktree`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of the M2 worktree test suite passes unchanged against the new git `wt.Backend` (zero behavioral regression in the dispatch path).
- **SC-002**: For a worktree with added/modified/deleted files, `GET /beads/{id}/worktree` returns every changed file with the correct change kind, and `GET /beads/{id}/diff` returns a diff that applies cleanly against the base — verified for both git and (when `jj` is present) jj backends.
- **SC-003**: A single-file diff request (`?path=`) returns only that file's hunks and nothing else.
- **SC-004**: A dispatch targeting an unavailable VCS backend is refused with `VCS_UNAVAILABLE` in 100% of cases — never a silent fallback.
- **SC-005**: A traversal/escape `?path=` value is rejected in 100% of attempts; no file outside the worktree is ever disclosed.
- **SC-006**: `go test -race ./...` passes clean and each new/changed package meets its per-package coverage gate (Constitution Principle IV).
- **SC-007**: Every M0/M1/M2 REST endpoint and WS event type remains present and unchanged (verified against the M2 contract).

## Assumptions

- **`git` is present at run time** (already an M2 assumption); `jj` is optional and probed.
- **M3 is read + create only.** Finalize/commit/push (the approval/review write path), worktree GC, and sub-bead worktree ownership are later milestones (M4/M8/M9) and are explicitly out of scope. The interface may *name* those methods to match the roadmap, but M3 neither implements nor tests them.
- **The existing `internal/worktree` package is promoted, not rewritten** — its hardened `Ensure` logic becomes the git backend's `Create`/`Status` internals; the package may be moved/renamed to `internal/wt` or wrapped, preserving its tests.
- **VCS selection is a global config default** (`--default-vcs`, `git|jj`, default `git`) — beads has no `vcs` column (mirroring M2's missing `review` column), so muster persists no per-bead VCS state. Per-bead override awaits a later bead-model change.
- **Local-first** (Constitution): diff endpoints serve loopback clients; no auth/multi-user concerns are introduced here.

## Out of Scope (deferred to later milestones)

- **Finalize / commit / push** the worktree on bead approval — the write-side of the lifecycle (M4 dispatcher / review flow).
- **Worktree garbage collection** (`muster gc`, idle->7-day cleanup) — M9.
- **Sub-bead worktree ownership** and auto-split — M8.
- **`worktree.changed` WS push** on every agent write — may be a small additive follow-up but is not required for the M3 read endpoints.
- **UI wiring** of the Worktree/Diff tabs to these endpoints — the embedded UI remains a mock prototype (tracked separately, M7).
- **VCS picker default persistence** and `muster init`-time `default_vcs` detection — configuration polish beyond the M3 create/diff slice.

## Dependencies & Roadmap Position

- **Pre-implementation spike — DONE (2026-07-03)**: the jj/git worktree and diff contracts are pinned against the real binaries (`jj` 0.42.0, `git` 2.55.0) in [research.md](research.md). Key findings: `jj diff --git` is byte-compatible with `git diff`; `jj root` is the jj-native detector; and the git backend must include **untracked** files via `git status --porcelain` (plain `git diff HEAD` silently omits them) — the diff read stays non-mutating (no `git add -N`).
- **Builds on M2**: the orchestrator, `Dispatch`, tmux transport, and `internal/worktree` (git create/reuse) all exist and are green.
- **Promotes forward**: `internal/worktree` → the full `wt.Backend` abstraction (`prototype/handoff/spec.md` §8).
- **Unblocks**: M4 (dispatcher/finalize/push over the same interface) and the UI's Worktree/Diff tabs (M7).
