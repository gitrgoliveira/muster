# Phase 0 Research — M1 Beads-Backed Store

Eight decisions resolve the unknowns flagged in Technical Context.

---

## 1. Embedded mode: read from `issues.jsonl`

**Decision**: **Read and parse `issues.jsonl`** directly. No Dolt library, no spawned process.
The `bd` CLI manages the Dolt database and exports `issues.jsonl` as a passive data file that
muster reads.

**Rationale**:
- `bd dolt start` is not supported in embedded mode — the embedded Dolt database has no server
  process. `bd` reads the Dolt data files directly in-process.
- Importing `github.com/dolthub/dolt/go/libraries/doltcore` pulls a multi-hundred-MB transitive graph.
- `issues.jsonl` maps 1:1 to the `store.Issue` struct — verified against live data from
  `~/repos/beads-central/.beads/issues.jsonl`. Each line is a complete JSON object.
- `bd` writes `issues.jsonl` atomically via `os.Rename` on every mutation, so it's always consistent.
- fsnotify watches the same file for change detection, making the architecture simple: one file
  is both the data source and the change trigger.

**Alternatives considered**:
- **Spawn `dolt sql-server`** — rejected: `bd dolt start` explicitly rejects embedded mode.
  muster would have to bypass `bd` and call `dolt` directly, creating a parallel code path.
- **Direct in-process embed via `doltcore`** — rejected: dependency weight + maintenance burden.
- **Shell out to `bd list --json` per request** — rejected: subprocess overhead per API call;
  JSONL parsing is simpler and faster.

---

## 2. Server mode: `bd dolt start` + MySQL wire

**Decision**: In server mode (`dolt_mode: "remote"`), muster reads connection details directly
from `metadata.json` (`dolt_host`, `dolt_port`, `dolt_user`, `dolt_database`), reads the password
from the `BEADS_DOLT_PASSWORD` env var (if set), calls `bd dolt start` (idempotent) to ensure the
server is up, then connects via **`github.com/go-sql-driver/mysql v1.8.x`**.

DSN format: `<user>:<pass>@tcp(<host>:<port>)/<db>?parseTime=true&collation=utf8mb4_0900_ai_ci`.
Password is omitted when `BEADS_DOLT_PASSWORD` is unset.

**Rationale**:
- Empirically verified (2026-05-24) that `bd dolt show --json` in embedded mode returns ONLY
  `{backend, data_dir, database, embedded, schema_version}` — no host/port/user. Even in server
  mode the output is not guaranteed to expose the password. Reading `metadata.json` directly is
  the canonical source: `bd init --server --server-host=... --server-port=... --server-user=...`
  writes these fields, and `bd dolt set <key> <value>` updates them.
- `bd` already manages the Dolt server lifecycle — port allocation, PID tracking, auto-start.
  muster delegates start/stop rather than reimplementing it.
- `bd dolt start` is idempotent: if the server is already running, it's a no-op.
- `parseTime=true` is required so `created_at` etc. come back as `time.Time`, not `[]byte`.
- `collation=utf8mb4_0900_ai_ci` matches Dolt's default.

**Alternatives considered**:
- Use `bd dolt show --json` for connection details — rejected: verified output does not include
  host/port/user in embedded mode and is brittle in server mode.
- Spawn `dolt sql-server` directly — rejected: duplicates `bd`'s server management logic.
- `gorm` or another ORM — rejected: muster queries are five hand-written SELECTs; an ORM is overkill.

---

## 3. `issues.jsonl` change-detection strategy

**Decision**: watch `<beads-dir>/issues.jsonl` with `fsnotify` for `Write | Create | Rename` events
(Linux merges Create+Rename for atomic writes; macOS emits Write). Debounce events with a **500 ms
trailing window**: after the first event, wait until 500 ms of quiet, then act.

**Rationale**:
- `bd` writes `issues.jsonl` atomically via `os.Rename`, so the file inode changes; we must
  re-add the watch after a Rename event. The watcher loop handles this.
- 500 ms covers most `bd close <id1> <id2> ...` bulk writes in a single re-read.

**Alternatives considered**:
- 100 ms debounce — too aggressive; bulk closes produce duplicate re-reads.
- 2 s debounce — exceeds spec SC-002 (2 s end-to-end budget for WS event).

---

## 4. Polling fallback trigger

**Decision**: fall back to **5 s polling of the file's mtime** when:
1. `fsnotify.NewWatcher()` returns an error (kernel feature missing), or
2. `fsnotify.Watcher.Add(path)` returns `EINVAL` or `EROFS` (network filesystem refuses inotify).

**Rationale**: NFS, fuse, and some SMB mounts disable inotify. Polling is degraded but functional —
the user gets up to 5 s extra latency, well within "live enough" for a UI.

**Alternatives considered**: hard-fail with an error — rejected; some users will run muster against
network-mounted `.beads/` for shared dev environments.

---

## 5. `bd` CLI surface for writes (v1.0+)

**Decision**: muster shells out to the following `bd` subcommands with `BEADS_DIR` set in the env,
always passing `--json` for structured output and `--dolt-auto-commit=on` for durable writes.
All user-supplied values go in the `--flag=value` argv form (single argv element) so `bd` cannot
mis-parse them as flags, and a `--` argv separator is inserted before any user-supplied positional
arguments.

| API call | Shell command (with `--json --dolt-auto-commit=on`) |
|---|---|
| `PATCH /api/v1/beads/{id}` (title) | `bd update <id> --title=<new>` |
| `PATCH /api/v1/beads/{id}` (description) | `bd update <id> --description=<new>` |
| `PATCH /api/v1/beads/{id}` (priority) | `bd update <id> --priority=<0-4>` |
| `PATCH /api/v1/beads/{id}` (status → in_progress) | `bd update <id> --claim` |
| `POST /api/v1/beads/{id}/move {toColumn:"done"}` | `bd close <id>` |
| `POST /api/v1/beads/{id}/move` (other columns) | `bd update <id> --status=<state>` |
| `POST /api/v1/beads` | `bd create --title=... --description=... --type=... --priority=...` |
| `POST /api/v1/beads/{id}/dispatch` | `bd update <id> --claim` (sufficient for M1) |
| `POST /api/v1/beads/{id}/comments` | `bd update <id> --append-notes=<text>` (additive — does NOT clobber existing notes) |

**Rationale**:
- `--json` (verified 2026-05-24): `bd create --json` returns the new issue as a single JSON object;
  `bd update --json` returns an array of updated issues. This eliminates fragile stdout-text parsing.
- `--append-notes` (verified 2026-05-24): additive with newline separator; `--notes` replaces.
  Using `--notes` would clobber comments — a data-loss bug.
- `--dolt-auto-commit=on`: without this, mutations remain in Dolt's working set and are not durable
  through `bd dolt push`. A later milestone (not M2) may revisit batch commits.
- Argv injection mitigation: a malicious title like `"--priority=0"` could otherwise be parsed as
  a flag by cobra. `--title=<value>` in a single argv element prevents this; `--` separator hardens further.

**Alternatives considered**: emit a single `bd batch ...` per request — rejected: not in `bd`'s
current surface; would couple muster's release cadence to a future `bd` feature.

---

## 6. Schema version probe

**Decision**: muster reads `schema_version` from `metadata.json` **if present**. If absent,
muster defaults to schema version 1. The supported range is hard-coded as `MinSchema=1, MaxSchema=2`.

**Rationale**: `metadata.json` carries the field. In embedded mode there is no SQL connection to
fall back to, and even in server mode the metadata should be authoritative. Defaulting to 1 handles
older databases where the field wasn't yet written.

**Alternatives considered**: query Dolt always — rejected: not available in embedded mode (no SQL
connection); adds unnecessary coupling.

---

## 7. Server mode lifecycle

**Decision**: muster calls `bd dolt start` at startup in server mode. This is idempotent — if the
server is already running, it returns immediately. muster does **not** call `bd dolt stop` on
shutdown, because other `bd` processes may depend on the running server.

**Rationale**: `bd` manages the Dolt server lifecycle (port allocation, PID tracking, logs in
`.beads/`). muster delegates this rather than reimplementing it. The server is a shared resource.

**Alternatives considered**:
- muster spawns and owns the `dolt sql-server` process — rejected: conflicts with `bd`'s server
  management; port collisions if both try to start a server.
- Require the user to manually run `bd dolt start` before `muster serve` — rejected: adds an
  unnecessary manual step; `bd dolt start` is safe to call repeatedly.

---

## 8. Delta computation

**Decision**: on fsnotify fire, muster re-reads from the backend (`Backend.List`) and computes
deltas in memory against the previous snapshot. The watcher emits `WatcherEvent{ChangedIDs: <ids>}`;
the services layer turns those into `bead.updated` / `bead.created` / `bead.deleted` WS events.

**Remote mode (no watcher)**: in remote (Dolt SQL) mode there is no `issues.jsonl` watcher, so the
`BeadService` publishes WS frames directly after each successful CLI write (`bead.created` on create,
`bead.updated` on patch/move/dispatch/comment). Embedded mode leaves this off (the watcher is the
single source, avoiding double-announce). External `bd` mutations against a remote Dolt server are
still not broadcast — that requires a Dolt change-notification mechanism, deferred to a later milestone.

**Initial snapshot bootstrapping**: `Watcher.Run(ctx)` calls `backend.List` **synchronously** before
starting fsnotify and before returning. This populates `snapshot` with the real initial state, so
the first delta is a no-op (or contains only real changes that happened during startup), NOT a flood
of `bead.created` events for every existing issue.

**Delta computation uses deep equality** (not just ID presence): two issues with the same ID but
different fields produce a `bead.updated` event. This avoids spurious events from `bd dolt commit`
rewriting `issues.jsonl` with identical content but new file metadata.

In **embedded mode**, `Backend.List` re-reads `issues.jsonl` — the change trigger and data source
are the same file. In **server mode**, `Backend.List` re-queries SQL — `issues.jsonl` is just the
change trigger.

**Shared in-memory snapshot**: the JSONL backend maintains an in-memory cache guarded by
`sync.RWMutex`, refreshed when (a) the watcher fires, or (b) the file's mtime is newer than the
cached version. Both the watcher and API requests read the cache, avoiding repeated file IO when
many clients are connected.

**Rationale**:
- The watcher is backend-agnostic: it calls `Backend.List`, diffs, and emits events. The backend
  implementation handles the read strategy.
- For ≤5000 issues, the diff cost is microseconds.
- Synchronous initial snapshot eliminates the "everything looks new" startup flood.

**Alternatives considered**:
- Subscribe to Dolt's commit log via `dolt log --streaming` — rejected: not yet stable in Dolt 1.x.
- Separate watcher implementations per mode — rejected: unnecessary complexity; the Backend
  abstraction already handles the difference.
- ID-set-only diff — rejected: noisy WS events from `bd dolt commit` rewrites.

---

**Status**: all unknowns resolved. Proceed to Phase 1 design.
