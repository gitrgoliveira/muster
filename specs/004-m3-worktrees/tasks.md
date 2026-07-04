# Tasks: M3 — Worktrees (`wt.Backend`)

**Input**: Design documents from `specs/004-m3-worktrees/`

**Prerequisites**: spec.md ✓, research.md ✓ (spike), plan.md ✓, data-model.md ✓, contracts/ ✓, checklists/ ✓

**TDD Policy** (Constitution IV, NON-NEGOTIABLE): write tests first; confirm they FAIL; then
implement until green. `go test -race ./...` must pass clean; per-package coverage gates in plan.md.

**Story independence**: US1 (interface + git) is the MVP and blocks all others. US2 (diff
endpoints) and US4 (status) build on US1. US3 (jj) is **independently droppable** — it adds a
second backend behind the interface without touching US1/US2 code paths.

**Parallelization**: `[P]` = different files, no dependency on an incomplete task. Groups marked
`<!-- parallel-group: N (max 3) -->`. Same-file tasks are `<!-- sequential -->`.

---

## Phase 1: Setup

<!-- parallel-group: 1 (max 3 concurrent) -->
- [x] T001 [P] Create `internal/wt/` package skeleton with `doc.go` (package doc: VCS-agnostic per-bead worktree abstraction; wraps `internal/worktree` for git).
- [x] T002 [P] Add fake `jj` test binary (argv-recording shell script emitting canned `diff --summary`/`diff --git`/`root`/`--version` by args) under `internal/wt/testdata/`, mirroring M2's fake-`tmux`/`claude` pattern. Ensure executable bit is committed.
- [x] T003 [P] Add a jj real-repo test helper (`jj git init` a tmpdir + workspace) guarded by a `jjAvailable()` skip check, in `internal/wt/testhelp_test.go`; reuse the git tmpdir helper style from `internal/worktree/testhelp_test.go`.

**Checkpoint**: `go build ./...` green; skeleton + fakes in place.

---

## Phase 2: Foundational — types & interface (blocks all stories)

<!-- sequential -->  (all in internal/wt/wt.go + wt_test.go)
- [x] T004 [US1] Test `VCS.Valid()` allow-list (`git|jj`, reject others) + `ChangeKind`/`FileChange`/`WorktreeStatus` zero-values, in `internal/wt/wt_test.go`.
- [x] T005 [US1] Define `VCS`, `ChangeKind`, `FileChange`, `WorktreeStatus`, sentinel errors (`ErrNotImplemented`, `ErrWorktreeNotFound`, `ErrVCSUnavailable`), and the `Backend` interface (full §8 surface) in `internal/wt/wt.go`.
- [x] T006 [US1] Test + implement `safeRelPath(worktree, path)` (reject absolute, `..`, non-local, escape) in `internal/wt/path.go` + `internal/wt/path_test.go` (FR-007). Table-driven, include `../../etc/passwd`, absolute, symlink-escape cases.
- [x] T007 [US1] Test + implement `Detect(ctx)` (probe `git --version`, `jj --version`) and `For(vcs)` resolver (git→gitBackend, jj→jjBackend, else error) in `internal/wt/wt.go` + `wt_test.go` (FR-010).

**Checkpoint**: interface + shared helpers compile; `For`/`Detect`/`safeRelPath` green.

---

## Phase 3: US1 — git backend, behavior-preserving (Priority: P1 — MVP) 🎯

**Goal**: the orchestrator dispatches through `wt.Backend` with zero behavioral change vs M2.

<!-- sequential -->  (git backend in internal/wt/git.go + git_test.go)
- [x] T008 [US1] Test `gitBackend.Create` delegates to `worktree.Ensure`: assert create/reuse/non-git-repo/symlink/wrong-branch/two-bead-isolation all behave identically to the M2 `internal/worktree` tests, against a real git tmpdir, in `internal/wt/git_test.go` (SC-001).
- [x] T009 [US1] Implement `gitBackend.Create(ctx, worktreesDir, srcRepo, beadID)` as a thin adapter over `worktree.Ensure`; `Finalize`/`Push`/`Remove` return `ErrNotImplemented`, in `internal/wt/git.go` (FR-002, FR-017).
- [x] T010 [US1] Test + implement `gitBackend.Status` (`git status --porcelain` empty ⇒ Clean; missing dir ⇒ Exists=false→ErrWorktreeNotFound) in `internal/wt/git.go` + `git_test.go`.
- [x] T011 [US1] Refactor orchestrator `Dispatch` to obtain a `wt.Backend` via `wt.For(defaultVCS)` and call `Backend.Create` instead of `worktree.Ensure`; keep all M2 orchestrator tests green (behavior-preserving), in `internal/orchestrator/orchestrator.go`.
- [x] T012 [US1] Add `wt.Detect`-based startup probe + `--default-vcs` wiring into orchestrator construction (refuse `vcs=jj` dispatch when jj unavailable → `ErrVCSUnavailable`), in `internal/orchestrator/orchestrator.go` + test (FR-011).

**Checkpoint** 🎯 MVP: full M2 dispatch path runs through `wt.Backend`; M2 suite 100% green (SC-001); `-race` clean.

---

## Phase 4: US2 — diff exposure (Priority: P1)

**Goal**: file list + unified diff over HTTP for the git backend.

<!-- sequential -->  (git diff logic in internal/wt/git.go)
- [x] T013 [US2] Test `gitBackend.DiffSummary` parses `git status --porcelain=v1 -z` incl. **untracked** (`??`→Added), M/D, and **rename** (`R`, requires rename detection — pin the flag, CHK018) into `[]FileChange` with `OldPath`; fake + real git, in `internal/wt/git_test.go`.
- [x] T014 [US2] Implement `gitBackend.DiffSummary` (non-mutating; no `git add -N`) in `internal/wt/git.go` (FR-013, plan invariant 1).
- [x] T015 [US2] Test + implement `gitBackend.Diff(ctx, beadID, path)`: whole = `git diff HEAD` + `git diff --no-index -- /dev/null <untracked>` appended; single-file = `git diff HEAD -- <path>` (or `--no-index` if untracked); returns streaming `io.ReadCloser` whose Close reaps the child, in `internal/wt/git.go` + `git_test.go` (FR-006, FR-008).

<!-- sequential -->  (service + handlers + router share files; do in order)
- [x] T016 [US2] Add `services.BeadService.Worktree(ctx, id)` + `Diff(ctx, id, path)` delegating to the orchestrator's `wt.Backend` (nil-safe when orchestrator absent, mirroring M2), in `internal/services/beads.go` + test.
- [x] T017 [US2] Test dispatch/handler cases: `GET /worktree` 200 summary / 404 WORKTREE_NOT_FOUND; `GET /diff` 200 stream / 404 / 400 INVALID_PATH (traversal) / 412 VCS_UNAVAILABLE, in `internal/api/beads/handlers_test.go` (FR-005/006/007/009).
- [x] T018 [US2] Implement thin handlers `Worktree` (render summary JSON) + `Diff` (stream `text/x-diff`, flush, no buffering) in `internal/api/beads/handlers.go`; validate `?path=` via `wt.safeRelPath` before delegating.
- [x] T019 [US2] Register routes `GET /api/v1/beads/{id}/worktree` and `GET /api/v1/beads/{id}/diff` in `internal/api/router.go` (additive; reuse ID-validation middleware).
- [x] T020 [US2] Integration test: dispatch (fake claude edits add/modify/delete/untracked files) → `/worktree` lists all with correct kinds → `/diff` applies cleanly → `?path=` returns one file, in `internal/orchestrator/integration_test.go` or `internal/api/beads/` (SC-002/003).

**Checkpoint**: diff exposure works end-to-end for git; additive-surface checklist (contract) verified (SC-007).

---

## Phase 5: US3 — jj backend (Priority: P2 — DROPPABLE)

**Goal**: second backend behind the same interface. Can be cut without touching US1/US2 if it
proves not worth the marginal cost (per clarification) — nothing below is imported by US1/US2.

<!-- sequential -->  (jj backend in internal/wt/jj.go + jj_test.go)
- [x] T021 [US3] Test `jjBackend.Create`: jj-native repo (`jj root` ok) → `jj workspace add <path>`; **plain-git repo → ErrVCSUnavailable** (no colocate, no fallback); fake-jj unit + real-jj integration (skip if absent), in `internal/wt/jj_test.go` (FR-003/004a/011a).
- [x] T022 [US3] Implement `jjBackend.Create` + jj-native probe (`jj root`); `Finalize`/`Push`/`Remove` → `ErrNotImplemented`, in `internal/wt/jj.go`.
- [x] T023 [US3] Test + implement `jjBackend.DiffSummary` (`jj diff --summary`, space-delimited M/A/D/R/C parser — separate from git's) and `Status` (`jj status`), in `internal/wt/jj.go` + `jj_test.go`.
- [x] T024 [US3] Test + implement `jjBackend.Diff` (`jj diff --git [path]`, streaming) — assert output is git-format and flows through the SAME `/diff` handler as git, in `internal/wt/jj.go` + `jj_test.go` (research §4).
- [x] T025 [US3] Real-jj integration test (skip if no `jj`): create workspace → edit add/modify/delete → `/worktree` + `/diff` return correct results via the same endpoints as git, in `internal/wt/jj_test.go` (SC-002 for jj).
- [x] T026 [US3] Pin `JJ_CONFIG`/git identity env in the jj tests for hermeticity (research §6); ensure no user config is required.

**Checkpoint**: `--default-vcs jj` against a jj-native repo produces identical DTO shapes to git; `VCS_UNAVAILABLE` on a git repo.

---

## Phase 6: US4 — status observability (Priority: P3)

<!-- sequential -->  (health DTO + orchestrator accessor)
- [x] T027 [US4] Test status DTO additions: `vcs.{defaultVCS,git,jj}` availability + `worktreeCount`, all M2 fields intact, in `internal/api/health/handler_test.go` (FR-012, SC-007).
- [x] T028 [US4] Add `worktreeCount` accessor to the orchestrator (count per-bead dirs under `--worktrees-dir`) + surface `wt.Detect` results; wire into the status handler + DTO, in `internal/orchestrator/`, `internal/api/health/dto.go` + `handler.go`.
- [x] T029 [US4] Decide + implement `WorktreeStatus.Ahead/Behind`: either git `rev-list --count @{u}...HEAD` (best-effort) OR leave `0` with a documented note (checklist follow-up #2). Pick one, test it.

---

## Phase 7: Polish & gates

<!-- parallel-group: 2 (max 3 concurrent) -->
- [x] T030 [P] Update README (flags table: `--default-vcs`; API table: `/worktree`, `/diff`; runtime deps: `jj` optional) + [quickstart.md](quickstart.md) if drift.
- [x] T031 [P] `bd remember` note: M3 gotchas — git untracked-files needs `status --porcelain` not `diff HEAD`; `jj diff --git` is git-compatible; jj source-repos-only; fake-jj exec bit.
- [x] T032 [P] Add `--default-vcs` to `make help`/startup banner + config allow-list reject-invalid test in `internal/config/` (FR-018).

<!-- sequential -->
- [x] T033 Confirm per-package coverage gates (plan targets: `internal/wt` ≥85%, others ≥ M2) and `go test -race ./...` clean.
- [x] T034 Verify additive surface: assert every M0/M1/M2 route + status field + WS event type is unchanged (SC-007) — a focused regression test.

---

## Dependencies & parallelism summary

- **Phase 1–2** (setup + interface) block everything.
- **US1 (P3 tasks T008–T012)** is the MVP and blocks US2/US4.
- **US2 (T013–T020)** depends on US1; endpoint/service/router tasks are sequential (shared files).
- **US3 (T021–T026)** depends only on the Phase-2 interface — **droppable**, parallel-safe vs US2 at the file level (all in `internal/wt/jj*.go`), but share the `/diff` handler test infra so land after T018.
- **US4 (T027–T029)** depends on US1 (orchestrator worktree count).
- **Polish (T030–T034)** last.

**Estimated**: 34 tasks. MVP (through US1 checkpoint) = T001–T012. Full read+diff (drop jj) = through T020. Complete = T034.
