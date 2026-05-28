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
| `database` | string | no | Defaults to `"dolt"` when absent. Non-dolt values cause exit-1 with `unsupported database "<v>"`. |
| `backend` | string | no | Informational; not consumed by muster. |
| `dolt_mode` | enum | no | `"embedded"` (default when absent) or `"remote"`. Other values cause exit-1. |
| `dolt_database` | string | yes | The Dolt schema name. For embedded, the subdirectory under `embeddeddolt/`. |
| `dolt_host` | string | required in server mode | Host of the Dolt SQL server. No default — empty value causes exit-1 in remote mode. |
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
| File missing | 1 | `error: cannot read metadata.json: <err>` |
| File unparseable | 1 | `error: cannot parse metadata.json: <err>` |
| Unsupported `database` | 1 | `error: unsupported database "<value>" (only "dolt" is supported)` |
| Unknown `dolt_mode` | 1 | `error: invalid dolt_mode "<value>" (want embedded or remote)` |
| `schema_version` outside `[1,2]` | 1 | `error: beads schema v<N> not supported by muster (need 1..2)` |

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
| `dolt_host` empty in metadata.json | 1 | `error: dolt_host is required in remote mode` |
| `dolt_database` empty in metadata.json | 1 | `error: dolt_database is required in remote mode` |
| `bd dolt start` fails | — | Warning logged; continues to attempt SQL connection |
| MySQL connection fails (10 s timeout) | 1 | `cannot connect to dolt server: <err>` |

---

## Stable values consumed elsewhere

After successful load, muster advertises:

- `X-Beads-Dir` header on all API responses: absolute path to `<beads-dir>`.
- `X-Beads-Database` header on all API responses: value of `dolt_database`.
- `GET /api/v1/orchestrator/status` body includes `{beadsDir, doltDatabase, doltMode, projectID, schemaVersion}`.
