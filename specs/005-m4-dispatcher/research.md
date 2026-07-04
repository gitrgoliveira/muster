# Research — M4 Dispatcher (Phase 0)

Format per decision: **Decision** / **Rationale** / **Alternatives considered**. Items marked **SPIKE** must be pinned against the real binary before the dependent task is implemented (Constitution: "verify assumptions empirically before building" — the same discipline that caught the stale `claude --plan` flag in M2 and pinned `jj diff --git` in M3).

## R1 — Scheduler: capacity + FIFO queue

**Decision.** A `scheduler` type inside `internal/orchestrator` holding: an integer `capacity`, a count/set of `active` bead IDs, and an ordered `waiting` FIFO slice of pending dispatch descriptors, all guarded by the orchestrator's existing `mu`. `Dispatch` admits immediately when `active < capacity`, otherwise appends a **pending** Run (state `StepPending`) to `waiting` and returns it as *queued*. When a run terminates (in the watcher-completion path that already fires on session end/timeout/cancel), the orchestrator pops the head of `waiting`, does the deferred Detect/Ensure/Invoke/Spawn, and marks it active — with **no client call**.

**Rationale.** Reuses the exact lock and completion hook the M2 reservation already established (closing the same TOCTOU window). Keeping the queue inside the orchestrator avoids exporting `runs`/`mu`. FIFO is the clarified policy and is trivially testable for order. `StepPending` already exists in `core` (`enums.go`), so no new core enum is required — a waiting run is `StepPending`, an admitted one is `StepActive`.

**Alternatives considered.** (a) A separate `internal/scheduler` package — rejected: forces exporting internal run state, re-opens TOCTOU. (b) A buffered channel as the queue — rejected: FIFO fairness + cancel-a-specific-waiter + runtime resize are awkward on a channel; an explicit slice under the lock is clearer and `-race` clean. (c) Golang `x/sync/semaphore` — rejected: adds a dependency (Constitution I) for what a counter under the existing mutex does.

## R2 — Runtime-mutable capacity (drain, not kill)

**Decision.** `SetCapacity(n)` validates `n>0` (typed error otherwise) and updates `capacity` under `mu`. Raising it immediately admits waiters up to the new limit. Lowering it below the current active count **does not** terminate running agents — it simply stops admitting until `active` drains below `n`. Exposed via `PUT /api/v1/orchestrator/capacity`.

**Rationale.** Matches the clarified "configurable in the UI" answer and FR-005a. Killing live agents on a config change would violate the isolation/least-surprise posture and could orphan tmux sessions. Fail-fast on `n≤0` mirrors the startup parser (no silent defaults).

**Alternatives considered.** Kill-newest-active on shrink — rejected: destroys in-flight work for a config tweak. Refuse shrink-below-active — rejected: the operator should be able to express "no new work" without a special case.

## R3 — Startup capacity config

**Decision.** `--max-concurrent-dispatches` flag / `MUSTER_MAX_CONCURRENT_DISPATCHES` env, default **4**, parsed by a new `config.ParseMaxConcurrent` that returns a typed error on non-integer or `≤0` (fail fast at startup, exactly like `ParseDefaultVCS`).

**Rationale.** Global setting (beads has no per-bead capacity column — same posture as M3 VCS). Default 4 = modest parallelism, bounded host load. Fail-fast satisfies "no silent defaults for user-controlled behavior."

**Alternatives considered.** `NumCPU`-derived default — rejected in clarify (unpredictable across hosts, harder to assert in tests). Unbounded default — rejected (the whole point of the milestone).

## R4 — Idempotent dispatch (the one contract migration)

**Decision.** Idempotency is keyed on **bead identity + in-flight state** (clarified). `Dispatch` for a bead that already has an active-or-waiting run returns that existing run with a `joined: true` marker instead of `ErrRunAlreadyActive`. At the HTTP layer this becomes **200 OK + existing run + `joined:true`** (was **409 Conflict**). This is the single intentional Constitution-V migration; M2's `TestDispatch_409_RunAlreadyActive` and `TestDispatch_409_DuplicateRun` are rewritten to assert the idempotent contract. `ErrRunAlreadyActive` is retained internally only if still needed for a genuinely conflicting case (different in-flight parameters); the default in-flight duplicate now joins.

**Rationale.** FR-017/FR-018 + the clarified "return the existing run, not an error." M2 is already idempotent in *effect* (the reservation prevents a second spawn); M4 makes it idempotent in *ergonomics*. Racing duplicates still yield exactly one run because the join happens under the same `mu` as the reservation.

**Alternatives considered.** (a) Keep 409, enrich the body — additive, zero migration, but a 409 still reads as an error (rejected in Complexity Tracking). (b) Idempotency-Key header — rejected in clarify (adds a key store; bead identity suffices).

## R5 — Git write-side contract

**Decision.**
- **Finalize(msg):** in the bead worktree, `git status --porcelain` first; if empty → **no-op success** ("nothing to commit", no commit created). Else `git add -A` then `git commit -m <msg>`. (Staging here is fine — the agent is no longer running when the operator finalizes; contrast the M3 read path which must stay non-mutating.)
- **Push:** `git push <remote> muster/<beadID>`; remote defaults to `origin`, name configurable. Non-zero exit (no remote, auth failure, rejected) → explicit error, never silent success.
- **Remove:** `git worktree remove <path>` (with `--force` only if the worktree is clean per policy) and prune; a subsequent `Status` reports absent.

**Rationale.** Straight git plumbing already available; branch `muster/<beadID>` is the existing convention (`internal/worktree`). Empty-commit no-op matches FR-010.

**Alternatives considered.** `git commit --allow-empty` on no change — rejected in clarify (pollutes history). Auto-remove after push — rejected (GC is M9; Remove is explicit/on-demand).

## R6 — JJ write-side contract — **SPIKE (real `jj`)**

**Decision (to confirm by spike).** jj auto-snapshots the working copy, so **Finalize** is `jj describe -m <msg>` on the working-copy revision (creating a new empty working revision as needed), rather than an explicit add+commit; **no-change** finalize is a no-op success (detect via `jj diff --summary` empty, mirroring the M3 jj read path). **Push** uses the colocated git remote — `jj git push --branch muster/<beadID>` (or `git push <remote> muster/<beadID>` against the colocated repo). **Remove** = `jj workspace forget <name>` + remove the workspace dir.

**SPIKE items to pin against real `jj` (≥0.42, as in M3):** exact describe/new incantation that produces a committed revision on `muster/<beadID>`; whether `jj git push` needs a bookmark/branch created first; the no-op-empty detection; and workspace teardown. Record byte-level command + output in this file before implementing the jj write-side task (US2 jj half). If a jj write-op proves materially harder than git, US2's jj slice can be gated/skip-marked exactly as M3 kept jj independently droppable — git write-side still ships complete.

**Rationale.** M3 already pinned jj *read* semantics; the write-side is the unproven half. The spike de-risks it before code, per the Constitution.

## R7 — Step chain & pointer model

**Decision.** A `StepChain` is an ordered `[]StepProfile`, each `StepProfile = {Name, PermissionMode, PromptRef}`. The per-bead **step pointer** (current index) is orchestrator run state on the `Run` (the `StepIdx` field already exists on `Run`, currently always 0). Resolution order for a dispatch: explicit chain in the `DispatchRequest` → configured default chain → single implicit step (index 0, M2 behavior). Advance/loop-back are **operator-driven**: `POST /steps/advance` moves the pointer +1 (range-checked against chain length), `POST /steps/loopback` moves it to an earlier explicit index (range-checked ≥0 and < current). Each transition runs that step's profile as a fresh agent invocation over the same worktree and emits a `step.*` event.

**Rationale.** Reuses the dormant `Run.StepIdx`. Per-step permission mode reuses the existing per-`Mode` argv plumbing (`adapter.Mode.Args(pm)`), and never silently defaults a mode (FR-012a). Operator-driven keeps the M8 policy engine out of M4. Single-step default keeps every M2 test green.

**Alternatives considered.** Persisting the chain/pointer in beads — rejected (no column; Constitution II). Automatic agent-driven progression — rejected in clarify (M8). A separate `steps` service — unnecessary; the pointer lives with the run.

## R8 — Quota from Claude Code's on-disk session record — **SPIKE (real `claude`)**

**Decision (to confirm by spike).** After a run ends, read Claude Code's own persisted per-session usage/cost record from disk (candidate: a session JSON under `~/.claude/` keyed by session/project) and map it to `QuotaUsage{InputTokens, OutputTokens, CostUSD, Known bool}`. Missing/unreadable/garbled → `QuotaUsage{Known:false}` (best-effort; never fails the run). `claude` adapter's `QuotaSource()` returns `QuotaCLIOutput` (was `QuotaNone`).

**SPIKE items to pin against real `claude`:** the exact on-disk path pattern, the JSON field names for tokens/cost, and how a session created by our tmux-launched interactive `claude` is correlated to a bead's run (session id? cwd/worktree path? most-recent-by-mtime under the worktree?). Record the real file path + a redacted sample payload in this file before implementing FR-022. If no stable on-disk record exists, US5 is dropped (spec allows it) without touching US1–US4.

**Rationale.** The interactive TUI transport streams redrawing ANSI bytes — parsing cost mid-stream yields garbage (M2 research). An on-disk record is ANSI-free and robust. Advisory-only keeps it non-blocking.

**Alternatives considered.** Scrape `capture-pane` scrollback — rejected in clarify (fragile). Post-run headless `claude -p` probe — rejected (second process; may not reflect the interactive session's true cost).

## R9 — Recovery interplay

**Decision.** On startup recovery (M2 `recovery.go`), each recovered active session re-registers into the scheduler's `active` set so capacity accounting is correct, and idempotency treats a recovered run as in-flight (a re-dispatch joins it). Recovered runs carry `StepIdx` from the session name (already parsed), so the step pointer survives restart for the current step.

**CRITICAL correction (review C1).** `recovery.go:80-85` **today kills** any recovered session with `StepIdx != 0 || Loop != 0` (`TestRecoverSessions_UnsupportedIndicesKilled` asserts this). M4 MUST relax this: a recovered `StepIdx` within `[0, chainLen)` **re-registers** rather than being killed — otherwise a restart destroys a live build/review agent (StepIdx 1/2). **Chain-unknown-at-recovery:** the session name carries `StepIdx` but NOT the chain (chain lived only in memory). So a recovered run is reconstructed as a **single-step run pinned at its recovered `StepIdx`** and **refuses to advance/loop-back until the bead is re-dispatched** (which re-supplies the chain). Genuinely malformed/negative indices are still killed. This is task T053; the assertion test is migrated (Complexity Tracking).

**Rationale.** Keeps FR-019 (idempotent after crash-recovery) and correct capacity accounting without a new persistence mechanism — the recovered tmux sessions are the source of truth for "what's active," exactly as in M2. Not persisting the chain honors Constitution II (no new durable state); the "refuse to advance until re-dispatched" rule is the safe, honest consequence.

**Alternatives considered.** Ignore recovered runs in capacity — rejected (would under-count; drain-not-kill tolerates a transient over-count instead, review L1). Persist the chain to survive restart — rejected (new durable muster state, Constitution II).

## Spike Log

*(Populated during implementation, before the dependent tasks — R6 jj write-side and R8 claude quota. Each entry: command run, real output/redacted sample, resulting pinned contract. Empty until the spike tasks run.)*

- [ ] **R6 jj write-side** — pinned commands + output: _pending_
- [ ] **R8 claude on-disk quota** — pinned path + sample payload: _pending_
