# Quickstart — M1 Beads-Backed Store

End-to-end walk-through for running `muster` against a real beads database.

---

## Prerequisites

- Go 1.26+ on `$PATH`
- `bd` v1.0+ on `$PATH` (install: `brew install gastownhall/tap/bd`)
- `dolt` on `$PATH` (a transitive requirement of `bd`)
- A beads-central checkout at `~/repos/beads-central/` (see `BEADS_MULTIREPO_SETUP_GUIDE.md`)

---

## 1. Build

```bash
cd ~/repos/muster
make build       # produces bin/muster
```

## 2. Start muster against a real beads database

```bash
bin/muster serve --beads-dir ~/repos/beads-central/muster/.beads
```

Expected banner:

```
muster listening on http://127.0.0.1:7766
  build         = dev
  schemaVersion = 1
  beadsDir      = /Users/<you>/repos/beads-central/muster/.beads
  doltDatabase  = muster
  doltMode      = embedded
  readSource    = issues.jsonl
  bdCLI         = /opt/homebrew/bin/bd
```

If `bd` is not installed, the banner will say `bdCLI = (missing — write endpoints disabled)`.

## 3. Verify live data

```bash
curl -s http://localhost:7766/api/v1/beads | jq '.total'
# Should print the real issue count, not 14.

curl -s http://localhost:7766/api/v1/beads | jq '.items[0].id'
# Should be a muster-* ID, not bd-xxxx seed data.
```

## 4. Observe live reload

In one terminal, connect a WebSocket client:

```bash
websocat ws://localhost:7766/api/v1/stream
# Wait for: {"type":"hello",...}
```

In another terminal, mutate a bead via `bd`:

```bash
cd ~/repos/muster
bd update muster-xyz --priority=0
```

Within ~2 seconds, the websocat terminal should print:

```json
{"type":"bead.updated","id":"muster-xyz"}
```

## 5. Verify write-back via API

```bash
curl -X PATCH http://localhost:7766/api/v1/beads/muster-xyz \
  -H 'Content-Type: application/json' \
  -d '{"title":"new title"}'
# 200 OK with the updated bead.

bd show muster-xyz | grep -i title
# Confirms the title changed in Dolt.
```

---

## Switching repositories

To serve a different beads database, restart `muster` with a different `--beads-dir`:

```bash
bin/muster serve --beads-dir ~/repos/beads-central/.beads
# Now serves the mp-* issues from the main project.
```

Multi-repo aggregation (one muster instance serving multiple repos) is **deferred to M7 — Repos & routing** (§20), not M2.

---

## Server mode (remote Dolt)

If your beads database uses a Dolt server (not embedded), set `dolt_mode` to `"remote"`:

```json
{
  "database": "dolt",
  "backend":  "dolt",
  "dolt_mode": "remote",
  "dolt_database": "muster",
  "dolt_host": "127.0.0.1",
  "project_id": "..."
}
```

Configure the server connection via `bd`:

```bash
bd dolt set host 127.0.0.1
bd dolt set port 3306
bd dolt set database muster
```

`muster serve --beads-dir <dir>` will detect `dolt_mode: remote`, run `bd dolt start`
(idempotent), and connect over MySQL wire protocol.

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `invalid beads-dir: metadata.json not found` | `--beads-dir` points to the wrong place | Use the absolute path to `.beads/`, not to its parent |
| `cannot start dolt server` | `bd dolt start` failed (server mode only) | Check `bd dolt status` and logs in `.beads/` |
| `501 NOT_IMPLEMENTED` on PATCH | `bd` is not on `$PATH` | Install `bd` or use `--bd-bin=/path/to/bd` |
| WS event takes 5+ seconds | fsnotify unavailable (network FS) | Expected — polling fallback is at 5 s |
| `beads schema v<N> not supported` | muster is older than the beads DB | Upgrade muster |
