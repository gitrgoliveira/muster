# Contract: Config files consumed by muster

muster reads **two** config files from `<beads-dir>`. Both are written by the `bd` CLI; muster never writes them.

---

## `<beads-dir>/metadata.json` (REQUIRED)

### Schema

```json
{
    "database":       "dolt",
    "backend":        "dolt",
    "dolt_mode":      "embedded" | "remote",
    "dolt_database":  "<schema_name>",
    "dolt_host":      "127.0.0.1",
    "dolt_port":      3306,
    "dolt_user":      "root",
    "project_id":     "<uuid>",
    "schema_version": 1
}
```

| Field | Type | Required | Notes |
|---|---|---|---|
| `database` | string | yes | Must be `"dolt"` in M1. Other values cause exit-1 with `STORE_UNAVAILABLE: unsupported database "<v>"`. |
| `backend` | string | yes | Must be `"dolt"` in M1. |
| `dolt_mode` | enum | yes | `"embedded"` or `"remote"`. Other values cause exit-1. |
| `dolt_database` | string | yes | The Dolt schema name. For embedded, the subdirectory under `embeddeddolt/`. |
| `dolt_host` | string | server mode only | Host of the Dolt SQL server. Default `127.0.0.1`. |
| `dolt_port` | int | server mode only | Port of the Dolt SQL server. Default `3306`. |
| `dolt_user` | string | server mode only | MySQL user. Default `root`. |
| `project_id` | UUID | yes | Echoed in `GET /api/v1/orchestrator/status`. Not used for routing in M1. |
| `schema_version` | int | no | When absent, defaults to 1. |

Server-mode password (when needed) comes from the `BEADS_DOLT_PASSWORD` environment variable, NOT
from `metadata.json` (which is git-tracked).

### Sample (verified against `~/repos/beads-central/muster/.beads/metadata.json`)

```json
{
  "database": "dolt",
  "backend": "dolt",
  "dolt_mode": "embedded",
  "dolt_database": "muster",
  "project_id": "7f795925-90d3-4cea-a8d9-0e0dfd6b4815"
}
```

### Failure modes

| Condition | Exit code | Message |
|---|---|---|
| File missing | 1 | `invalid beads-dir: metadata.json not found at <path>` |
| File unparseable | 1 | `invalid beads-dir: cannot read metadata.json: <err>` |
| Unsupported `database` or `backend` | 1 | `invalid beads-dir: unsupported <field> "<value>"` |
| Unknown `dolt_mode` | 1 | `invalid beads-dir: dolt_mode must be "embedded" or "remote"` |
| `dolt_database` empty | 1 | `invalid beads-dir: dolt_database is empty` |
| `schema_version` outside `[1,2]` | 1 | `beads schema v<N> not supported by muster (need 1..2)` |

---

## `<beads-dir>/config.yaml` (OPTIONAL)

Contains `bd` configuration (issue-prefix, export.auto, integration secrets, etc.). muster does
NOT read `config.yaml` directly. Server-mode connection details live in `metadata.json` instead
(see above table).

### Fields read by muster

None directly.

### Failure modes (server mode only)

| Condition | Exit code | Message |
|---|---|---|
| `dolt_host` empty in metadata.json | 1 | `invalid beads-dir: dolt_host required for dolt_mode=remote` |
| `dolt_port` zero/negative | 1 | `invalid beads-dir: dolt_port required for dolt_mode=remote` |
| `dolt_user` empty | 1 | `invalid beads-dir: dolt_user required for dolt_mode=remote` |
| `bd dolt start` fails | 1 | `cannot start dolt server: <bd stderr>` |
| MySQL connection fails after start (5 s timeout) | 1 | `cannot connect to dolt server: <err>` |

---

## Stable values consumed elsewhere

After successful load, muster advertises:

- `X-Beads-Dir` header on all API responses: absolute path to `<beads-dir>`.
- `X-Beads-Database` header on all API responses: value of `dolt_database`.
- `GET /api/v1/orchestrator/status` body includes `{beadsDir, doltDatabase, doltMode, projectID, schemaVersion}`.
