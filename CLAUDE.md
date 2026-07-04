# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

**muster** is a single Go binary that serves [beads](https://github.com/gastownhall/beads) issues over a REST + WebSocket API and runs CLI coding agents (Claude Code) against them inside per-bead VCS worktrees. It owns no durable state of its own — beads is the source of truth, and muster shells out to external CLIs (`bd`, `dolt`, `git`, `jj`, `tmux`, `claude`) rather than embedding them.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:7510c1e2 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/SYNC_CONCEPTS.md for details and anti-patterns.

## Session Completion

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
<!-- END BEADS INTEGRATION -->


## Build & Test

```bash
make build        # -> bin/muster
make test         # go test -race ./...  (claude always faked; real-tmux/git/jj tests run if the binary is present, else skip)
make cover-check  # per-package coverage gates (see the thresholds map in the Makefile); fails the build if a gated package regresses
make lint         # gofmt -l + go vet + golangci-lint
make test-e2e     # //go:build e2e — real dispatch against claude + tmux; excluded from the default suite
```

Run the server: `bin/muster serve --beads-dir /path/to/.beads` (see the README Flags table for all flags; muster binds `127.0.0.1:7766` and has **no auth** — never expose beyond localhost).

Run a single test / package:

```bash
go test ./internal/wt/... -run TestJJBackend_Create_RealJJ_NativeRepo -v
go test -race ./internal/orchestrator/...
go test -tags=e2e -run TestE2E ./internal/orchestrator/   # manual e2e equivalent
```

`make test` never *requires* `claude`, `tmux`, or `jj`: external CLIs are faked via argv-recording shell scripts under `internal/<pkg>/testdata/` (put on a temp `$PATH`), and real-binary integration tests skip when the tool is absent but **do** run when it's present. The coverage gate for a new package must be added to the `thresholds` map in the Makefile's `cover-check` target, or it isn't enforced.

## Architecture Overview

Strict one-directional layering — dependencies point downward only, and this is enforced socially via the Constitution (below), not by a linter:

```
api (thin HTTP/WS handlers)  →  services  →  store + core
                              ↘  orchestrator  →  adapter/claude, tmux, wt (+ worktree), config
```

- **`internal/core`** — domain types (Bead, columns, etc.). No I/O.
- **`internal/store`** — `store.Backend` interface (read side) with implementations: `store/dolt` and `store/jsonl` read beads; **`store/bdshell` is the authoritative writer** — all issue mutations go through the `bd` CLI, never around it (Constitution II). `store/watcher.go` reloads on file change.
- **`internal/services`** — orchestrates store + `ws` (broadcast) + `wt` (worktrees); this is where business logic lives so handlers stay thin (Constitution III).
- **`internal/api`** — router + per-resource handler packages (`api/beads`, `api/health`, `api/stream`), plus `api/render` (response encoding) and `api/middleware`. Handlers validate input, call a service, render — no logic.
- **`internal/orchestrator`** — the dispatch engine: takes a bead, resolves its source repo (`--repo` prefix map), ensures a per-bead worktree via `wt.Backend`, launches the agent through `adapter/claude` inside a `tmux` session, and streams output as `runlog.line` WS events. `service_adapter.go`/`worktree_adapter.go` bridge it into `services`.
- **`internal/wt`** — VCS-agnostic per-bead worktree abstraction (`wt.Backend`, git + jj); `internal/worktree` is the older git-only helper that `wt`'s git backend wraps (behavior-preserving). Reads are **non-mutating** (`git status --porcelain`, never `git add -N` — it races the running agent).
- **`internal/ws`** — WebSocket hub + typed event envelope (`bead.*`, `runlog.line`, `tmux.session.*`).
- **`internal/adapter/claude`** — builds the `claude` CLI invocation (permission modes, print/stream formats); `internal/tmux` wraps tmux session/pane/pipe management; `internal/shellquote` for safe arg construction.

## Conventions & Patterns

- **Spec-driven delivery.** Work is organized as milestones under `specs/NNN-milestone/` (spec.md, plan.md, tasks.md, research.md, contracts/, checklists/). Before implementing a milestone, read its `plan.md`. The governing rules live in **`.specify/memory/constitution.md`** — every plan has a "Constitution Check" gate. The five principles: (I) single self-contained binary; (II) beads is the source of truth (muster holds no authoritative durable state; writes go through `bd`); (III) layered architecture, thin handlers; (IV) **test-first, per-layer coverage, `-race` clean — NON-NEGOTIABLE**; (V) additive, backward-compatible REST/WS surface — a milestone must not break shapes/paths/event types a prior one shipped.
- **TDD is mandatory** (Constitution IV): write the failing test first, then implement to green. New external-CLI integrations get a fake-on-`$PATH` unit test **and** a skip-gated real-binary integration test.
- **Additive surface only.** New routes/DTO fields/WS event types are fine; changing or removing existing ones needs an explicit versioned migration. Milestone refactors must keep prior-milestone suites green.
- **No silent defaults for user-controlled behavior** — e.g. an unavailable VCS backend returns `VCS_UNAVAILABLE` rather than falling back to git; agent autonomy is never defaulted silently.
- **Beads DB caveat:** the `.beads/` DB in this repo tracks a *different* project (the beads used by the `bd` tooling), **not** muster's own development. muster's own roadmap/tasks live in `specs/`. Use `bd remember` (not MEMORY.md) for cross-session knowledge.
- Conventional-commit messages, scoped by milestone or package (`feat(m3):`, `fix(orchestrator):`). PRs follow `.github/pull_request_template.md`.

<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan at:
specs/005-m4-dispatcher/plan.md
<!-- SPECKIT END -->
