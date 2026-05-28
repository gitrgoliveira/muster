# Contract: `bd` CLI bridge

Write endpoints in muster shell out to the `bd` CLI. This contract pins the verbs, env, error handling,
and argv safety rules.

## Argv safety (CRITICAL — argument injection mitigation)

User-supplied values from the request body MUST NOT be passed as separate argv elements where `bd`
might re-interpret them as flags. For example, a malicious title `"--priority=0"` could be parsed
by Cobra as setting `--priority=0`. To prevent this:

1. **Always use the `--flag=value` form as a single argv element.** Never split into two args
   like `["--title", title]` — always `["--title="+title]`.
2. **Insert `--` argv separator** after all named flags, before any positional arguments.
3. **Treat all user-supplied strings as opaque values.** No string concatenation into a flag value
   that could break out (e.g., never `--title="`+title+`"` — use Go's `strconv.Quote` only inside JSON,
   not in argv).
4. **Always pass `--json`** to receive structured output instead of formatted text.
5. **Always pass `--dolt-auto-commit=on`** so mutations commit to Dolt immediately.

---

## Subprocess environment

Every `bd` invocation runs with:

| Var | Value | Why |
|---|---|---|
| `BEADS_DIR` | `BackendConfig.BeadsDir` | Routes `bd` to the same database muster is reading. Critical in multi-repo setups (`mp` vs `muster`). |
| `PATH` | inherited | So `bd` can find `dolt`. |
| `HOME` | inherited | `bd` reads `~/.config/...` for some prefs. |
| `BEADS_DOLT_PASSWORD` | inherited when set | Remote-mode `bd` writes / `bd dolt start` need it to reach a password-protected Dolt server. |
| All other env | NOT inherited | Hygiene — muster starts each `bd` with a minimal env. |

Working directory: muster's own cwd (the value of `BEADS_DIR` is authoritative for `bd`).

Timeout: **30 s** per invocation; enforced via `exec.CommandContext` with a derived context.
On timeout, `exec.CommandContext` kills the process (SIGKILL on Unix); there is no
SIGTERM grace period in M1. A graceful SIGTERM→wait→SIGKILL escalation is a future refinement.

---

## API → `bd` mapping

### `PATCH /api/v1/beads/{id}`

Body is sparse. muster composes the flags for the supplied fields and runs ONE `bd update`:

| Body field | `bd` flag |
|---|---|
| `title` | `--title=<v>` |
| `desc` / `description` | `--description=<v>` |
| `priority` | `--priority=<0-4>` (validate 0..4 in muster before invoking) |
| `assignee` | `--assignee=<v>` |
| `column` | `--status=<v>` (column mapped to a beads status via `columnToStatuses`) |

Example: `PATCH /api/v1/beads/mp-abc {"title":"X","priority":1}`
→ `bd update --json --dolt-auto-commit=on --title=X --priority=1 -- mp-abc`

Output is parsed as a JSON array; the first element is the updated issue.

If the body is empty after stripping unknown fields, return `400 INVALID_REQUEST` without invoking `bd`.
Note: comments/notes are NOT updated via PATCH — they go through `POST /comments` using `--append-notes`.

---

### `POST /api/v1/beads`

Body MUST contain `title` and `type`. muster composes:

```
bd create --json --dolt-auto-commit=on --title=<title> --description=<desc> --type=<type> --priority=<p> [--assignee=<a>]
```

On success, `bd create --json` returns the new issue as a single JSON object (verified empirically
2026-05-24). muster unmarshals into `store.Issue` directly and returns it. **No stdout-text parsing.**

If `bd` produces no JSON or invalid JSON, return `500 INTERNAL`.

---

### `POST /api/v1/beads/{id}/move`

| `toColumn` | `bd` command |
|---|---|
| `done` | `bd close <id>` |
| `running` | `bd update <id> --claim` |
| `review` | `bd update <id> --status=in_progress` (M1 simplification; `review` column not modeled in beads) |
| `scheduled` | `bd update <id> --status=open` |
| `backlog` | `bd update <id> --status=open` |

`beforeID` is **ignored** in M1 (column reordering not supported by `bd` v1.0). muster still
returns 200 OK so the optimistic UI does not break.

---

### `POST /api/v1/beads/{id}/dispatch`

```
bd update <id> --claim
```

History events (`claimed`, `started`) are appended by `bd` itself. muster reads the updated issue
and returns it.

---

### `POST /api/v1/beads/{id}/comments`

```
bd update --json --dolt-auto-commit=on <id> --append-notes=<text>
```

**CRITICAL**: Use `--append-notes` (additive — verified empirically) and NOT `--notes` (destructive
— would clobber all prior notes). The `actor` field from `CommentRequest` is dropped in M1 (`bd v1.0`
does not accept an `--actor` flag for `update`); prefix the actor name into the note text instead
if the UI needs attribution (e.g., `<actor>: <text>`).

(`bd v1.0` does not yet expose a dedicated `comment add` verb. M2 spec may revisit when it lands.)

---

## Exit code → HTTP code

| `bd` exit | Meaning | HTTP | Body code |
|---|---|---|---|
| 0 | success | 200 / 201 | n/a |
| 1 | validation error in args | 422 | `UNPROCESSABLE_ENTITY` |
| 2 | issue not found | 404 | `BEAD_NOT_FOUND` |
| 3 | database lock / unavailable | 503 | `STORE_UNAVAILABLE` |
| context deadline (SIGKILL by exec.CommandContext) | timeout | 504 | `GATEWAY_TIMEOUT` |
| other non-zero | unknown failure | 500 | `INTERNAL` |

The `bd` stderr text is passed through into `error.message` (after truncating to 512 bytes and
stripping ANSI color codes).

---

## When `bd` is not installed

Detected at startup by `exec.LookPath("bd")` (called unconditionally; `--bd-bin`/`BD_BIN` overrides). On miss:

- Log: `warning: bd CLI not available: bd CLI not found`
- Startup banner shows: `bdCLI = (missing — write endpoints disabled)`
- All write endpoints return `501 NOT_IMPLEMENTED` with `{"error":{"code":"BD_CLI_MISSING","message":"bd CLI not available"}}`
- Read endpoints continue to work.
