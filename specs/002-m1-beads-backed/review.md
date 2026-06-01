# Phase 7 Cross-Model Review — M1 Beads-Backed Store

**Reviewer**: Opus 4.7 | **Date**: 2026-05-24 | **Adversarial pass**: fresh eyes after Phase 6

The Sonnet pass produced a coherent spec. Phase 6 (Opus 4.6) caught structural gaps (Store interface refactoring, ID regex, mapper layer). This pass looks for what *both* prior passes missed: subtle correctness, security, performance, and operational risks.

---

## CRITICAL findings

### R-C1 — `bd dolt show --json` does NOT expose connection details
**Verified empirically**. The actual output is:
```json
{"backend":"dolt","data_dir":"...","database":"mp","embedded":true,"schema_version":1}
```
No `host`, `port`, `user`. The spec, research §2, and task T086 all assume `bd dolt show --json` returns a usable DSN. **This will fail at implementation time.**

**Fix**: The server-mode DSN must come from either (a) reading `config.yaml` directly (`dolt.host`/`port`/`user` fields), or (b) a different `bd` invocation (`bd dolt config get host`, etc.). Need to verify against a non-embedded `bd` setup before coding T086. Until verified, US3 (server mode) is at risk.

### R-C2 — `bd dolt start` will fail in embedded mode tests
Embedded test fixtures created via `bd init` will be in **embedded** mode. Any integration test (T035, T047, T073) that exercises the *startup* path of muster against such a fixture and accidentally hits the server-mode branch will get a hard fail with "not supported in embedded mode". Make sure tests pin `dolt_mode: embedded` explicitly, and that the mode selector in `cmd/muster/main.go` cannot fall through into server mode by accident.

---

## HIGH findings

### R-H1 — Argument injection via `bd` flag composition
T066 says: "compose `bd update <id>` flags from request body fields". A title like `"--priority=0"` passed as `--title=--priority=0` may be parsed by `bd` as two flags depending on Cobra's behavior, allowing field hijacking. **Mitigation**: always use the `--flag=value` form (single argv element) AND inject a `--` argv separator before the value-bearing flags, OR explicitly validate that user input does not start with `-`. Neither tasks.md nor bd-cli-bridge.md mentions this. Add to T066/T067.

### R-H2 — `Issue.Description` in list responses can be megabytes
`bd show <id>` shows multi-line descriptions; some can be tens of KB. `GET /api/v1/beads` returns every issue including description. With 5000 issues × 10KB avg = 50MB per list response. **Performance goal "p95 ≤ 50ms" is at risk**, and clients may OOM. M0's seed data has tiny descriptions, masking this. **Fix**: add a `Filter.SelectDescription bool` (default false for List, true for Get) and truncate description to N chars in list responses. Not in any task.

### R-H3 — No request body size limit
No `http.MaxBytesReader` wrapper anywhere. A 10GB PATCH body will be slurped into memory by the JSON decoder before any handler logic runs. M0 has the same gap, but M1 inherits it by claiming "no breaking changes". **Fix**: add a body size cap (1 MB?) as a middleware in Phase 9 polish, with task and CHK entry.

### R-H4 — JSONL read race during atomic rename window
fsnotify fires after rename completes, so reads triggered by the watcher are fine. BUT: an in-flight `GET /api/v1/beads` running concurrently with `bd close` may open `issues.jsonl` *during* the rename window. On macOS the old inode is still readable through an already-open fd, but on Linux a re-open can succeed and read either the old or new file. Worse: the JSONL backend re-reads per call, so `os.Open` may race with the new file being partially written *if* `bd` does append-then-rename. **Mitigation**: use `os.ReadFile` (single syscall on Linux returns either old or new) and reject reads where the file ends with an incomplete JSON line (trailing `{...,"updat`). Add to T042. The Edge Cases section mentions "retry parse 3× with 100ms delay" but only for fsnotify-triggered reads, not for API-triggered reads.

### R-H5 — Watcher initial snapshot bootstrapping
The spec, research §8, and data-model.md describe diff-against-snapshot but never specify when/how `snapshot` is first populated. If empty, the first fsnotify event after startup will look like every issue was just created → flood of `bead.created` WS events. **Fix**: `Watcher.Run` must call `backend.List` synchronously and populate `snapshot` *before* returning (or before starting fsnotify). Add to T053.

### R-H6 — `memstore.go` cannot serve both M0 tests AND new Backend interface
M0 `memstore` returns `core.Bead`; M1 `Backend.List` returns `[]store.Issue`. T014 says "update if needed" but T046 also says "all M0 tests green". One must give. **Resolution**: rewrite memstore to return `store.Issue` and delete or rewrite the M0 tests that assert on `core.Bead`-shaped store outputs. Acknowledge that "all M0 tests pass" is unachievable — the store-level tests are by definition impacted. Update plan.md complexity tracking or scope.

---

## MEDIUM findings

### R-M1 — No `bd dolt commit` after writes
Each `bd update`/`bd close` mutates the working set; without `bd dolt commit`, restarting `bd dolt` (or `bd dolt push`) may leave a dirty state. The `dolt-auto-commit` flag exists in `bd` (off/on/batch). **Decision needed**: should muster pass `--dolt-auto-commit=on` to its `bd` invocations, or rely on the user's default? Document the choice or it will silently produce data-loss surprises on Dolt remote push.

### R-M2 — Polling fallback conflicts with SC-002
SC-002: WS event within 2s. FR-007: polling every 5s. **Edge case acceptance**: when fsnotify falls back to polling, SC-002 is violated. Spec acknowledges polling adds latency but doesn't relax SC-002 for that path. Either weaken SC-002 to say "≤2s when fsnotify available, ≤6s on fallback", or add explicit edge case to spec.

### R-M3 — `bd create` stdout format unverified
T067 assumes `bd create` prints the new ID matching `[a-z]+-[0-9a-z]+`. The real format hasn't been verified — `bd create` might print prefix/suffix lines, JSON, color codes, or a banner. Add a verification task: run `bd create --title=test --type=task --description=test` once in a sandbox and pin the exact format in bd-cli-bridge.md.

### R-M4 — No max-line-size guard in JSONL parser
Malformed `issues.jsonl` with a 1GB single line would OOM muster. Use `bufio.Scanner` with `Buffer(make([]byte, 0, 1<<16), 4<<20)` to cap lines at 4 MB. Add to T042.

### R-M5 — Concurrent watcher + API readers cause double work
A `bd close` triggers (a) watcher re-reads `issues.jsonl` and emits delta, and (b) the optimistic-UI client subsequently calls `GET /api/v1/beads/{id}` to confirm — another full file re-read. Under bursty external mutations + many connected clients, the JSONL file is reparsed dozens of times per second. **Mitigation**: in the JSONL backend, maintain an in-memory snapshot guarded by `sync.RWMutex`, refreshed when (a) fsnotify says so, or (b) `List`'s read sees a newer mtime. The watcher and the backend share the snapshot. Not in any task. Worth a small refactor before Phase 9.

### R-M6 — `Online` field never updated
M0's `OrchestratorStatusResponse.Online` is hardcoded `true`. M1 inherits this but the field is now meaningful (the Backend can become unhealthy). Either remove `Online` from the response or wire it to `backend.Ping()` (add to interface). Not addressed in T034.

### R-M7 — `bd update --notes=<text>` does not allow empty text? Append vs replace?
Unverified. The contract says `POST /comments` maps to `bd update <id> --notes=<text>`. We don't know whether `--notes` appends or replaces existing notes. If it replaces, comments are clobbered — a major regression. Pin the behavior before T070.

---

## LOW findings

### R-L1 — `bd dolt commit` race with muster watcher
If user runs `bd dolt commit` while muster is mid-read, the `issues.jsonl` may be rewritten with a "committed" state that differs by metadata only. Watcher would emit spurious `bead.updated` events with no actual field changes. Not a correctness bug; just noisy WS traffic. Consider deep-equality in the diff, not just ID-set diff.

### R-L2 — Hardcoded ports/timeouts not configurable
500ms debounce, 5s poll, 5s `bd` timeout, 30s reconnect cap — all hardcoded. Users on slow disks or remote NFS may need higher values. Defer to **M10 — Polish + harden** (operational hardening) but document now.

### R-L3 — `cmd/muster/embed.go` vs current `cmd/musterd/` structure
Plan references `cmd/muster/embed.go`. Tasks.md never names it. T001 only renames the directory; need to confirm whether files inside also need updating (package declarations, etc.).

---

## Summary

| Severity | Count | Resolution |
|---|---|---|
| CRITICAL | 2 | ✅ Both resolved |
| HIGH | 6 | ✅ All 6 resolved |
| MEDIUM | 7 | ✅ All 7 resolved |
| LOW | 3 | 2 resolved, 1 deferred to M10 (hardening) |

## Resolution status (after Phase 7.5 spike + spec/task updates)

### CRITICAL — RESOLVED
- **R-C1**: Phase 7.5 spike verified `bd dolt show --json` output. Updated research §2 + spec FR-004 + data-model + config-file contract: server-mode connection details come from `metadata.json` (`dolt_host`/`dolt_port`/`dolt_user`/`dolt_database`) + `BEADS_DOLT_PASSWORD` env var.
- **R-C2**: Mode selector in `cmd/muster/main.go` only calls `bd dolt start` when `BackendConfig.Mode == "remote"`. Embedded mode never touches `bd dolt start`.

### HIGH — RESOLVED
- **R-H1** (argv injection): Added §"Argv safety" to `bd-cli-bridge.md` and added FR-009 update; CHK111b + CHK114a verify `--flag=value` argv form and injection test.
- **R-H2** (description size): Added `Filter.TruncateDesc int` field; FR-016 specifies 2 KB list-path cap, full on `Get`; CHK148b/148c verify.
- **R-H3** (no body size limit): Added FR-015 (1 MB cap via `http.MaxBytesReader`), T008, CHK009.
- **R-H4** (JSONL race): Added FR-017 (4 MB line cap, 3× retry with 100 ms backoff on API-triggered reads), T042/CHK048a.
- **R-H5** (snapshot bootstrap): Updated research §8 + data-model invariants + T053 + CHK084a: `Watcher.Run` populates snapshot synchronously before fsnotify starts.
- **R-H6** (memstore vs M0 tests): Updated FR-014 with carve-out + T014 explicitly rewrites memstore for the new interface + acknowledges that M0 store-level tests get rewritten (service-level tests still pass via the new mapper).

### MEDIUM — RESOLVED
- **R-M1** (no `bd dolt commit`): Added FR-018 — every `bd` invocation passes `--dolt-auto-commit=on`. Verified flag exists.
- **R-M2** (polling vs SC-002): Updated SC-002 to "≤2 s with fsnotify, ≤6 s on polling fallback".
- **R-M3** (`bd create` stdout): Phase 7.5 spike pinned format: `bd create --json` returns single JSON object. Use `--json` everywhere. Updated bd-cli-bridge.md + T067.
- **R-M4** (max line size): Covered by FR-017 + T042 (4 MB line cap via `bufio.Scanner.Buffer`).
- **R-M5** (double work): Added shared in-memory cache in `internal/store/jsonl/backend.go` with `sync.RWMutex`; T042b.
- **R-M6** (`Online` field): Added `Backend.Ping(ctx) error` method to interface; T034b + CHK163.
- **R-M7** (`--notes` vs `--append-notes`): Phase 7.5 spike verified `--append-notes` is additive, `--notes` is destructive. Updated bd-cli-bridge.md + T070 + CHK147 to use `--append-notes`.

### LOW
- **R-L1** (deep-equality diff): Resolved — research §8 + T054 + CHK091a use deep-equality, suppress empty deltas.
- **R-L2** (hardcoded timeouts): Deferred to **M10 — Polish + harden** (documented in research as known limitation).
- **R-L3** (`embed.go` rename): Resolved via T001 — entire `cmd/musterd/` dir is renamed; all files inside come along.

## Verification spike (Phase 7.5) — empirical findings

Run 2026-05-24 against a sandbox `bd` v1.0+ repo:

1. **`bd dolt show --json` (embedded mode)** returns `{backend, data_dir, database, embedded, schema_version}` — NO host/port/user. Server-mode connection MUST come from `metadata.json` directly.
2. **`bd create --json`** returns a clean single JSON object matching `store.Issue` shape.
3. **`bd update <id> --append-notes=<text> --json`** is additive (verified: two appends produced `"first comment\nsecond comment"`).
4. **`bd update <id> --notes=<text>`** replaces (would clobber). Use `--append-notes` only.
5. **`BEADS_DIR=<dir>`** correctly routes operations to the target repo (verified by creating `spk-1oi` in isolated sandbox).
6. **`bd dolt start`** rejects embedded mode with clear error — embedded mode never spawns a server.

**Verdict**: spec, plan, research, data-model, contracts, tasks, checklist all updated. Phase 8 implementation can proceed.
