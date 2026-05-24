# Muster — Backend Spec (Go)

The HTTP + WebSocket backend that powers the Muster UI. Drives the dispatcher loop, shells out to CLI agents, captures their work, and persists everything to Beads.

This document is the contract between the UI and the backend. The UI (`Muster.html` + JSX) consumes the API described here; the backend implements it.

---

## 0. Goals & non-goals

**Goals**

- Run an orchestrator daemon (`muster`) that the UI can talk to over HTTP+WS.
- Dispatch beads onto local CLI agents (Claude Code, Gemini CLI, OpenCode, Codex) and direct-API providers.
- Stream live worktree changes, run logs, and step status to the UI.
- Persist beads, sub-beads, dependencies, and run history via Beads (Dolt).
- Honour the constitution, per-task skill loadout, per-step skills/prompts, and loops.
- Track per-provider quotas and parallel capacity; requeue on exhaustion; auto-split when configured.

**Non-goals (v1)**

- Multi-user auth. Single-user, single-machine.
- Multi-machine dispatcher. Everything runs on the user's box.
- Cross-repo orchestration. One bead → one worktree → one repo.
- A web-hosted variant. The daemon binds to localhost.
- An internal LLM. Muster never *is* the model; it always shells out or hits an external API.

---

## 1. High-level architecture

```
                ┌──────────────────────────────┐
                │  UI  (Muster.html, served    │
                │       statically by muster) │
                └──────────────┬───────────────┘
                               │ HTTP + WS
                               ▼
        ┌──────────────────────────────────────────┐
        │              muster  (Go)               │
        │                                          │
        │   api/    →   HTTP & WS handlers         │
        │   core/   →   bead model, step engine    │
        │   disp/   →   dispatcher loop, scheduler │
        │   cli/    →   per-provider CLI adapters  │
        │   sdk/    →   SDK adapters (opencode, …) │
        │   apiprov/→   direct-API adapter         │
        │   wt/     →   worktree manager (jj/git)  │
        │   store/  →   Beads (Dolt) + key store   │
        │   quota/  →   per-provider quota tracker │
        │   constitution/ → cached constitution    │
        │   skills/ →   skill registry & loader    │
        └──────────────────────────────────────────┘
                ▲              │
                │              ▼
                │     ┌────────────────────┐
                │     │  tmux sessions   │  ← one per running step
                │     │  muster/<bead>/<n> │     user can `tmux attach` to watch
                │     │  hosting:         │     or intervene live
                │     │  claude, gemini, │
                │     │  codex, …         │
                │     └────────────────────┘
                │
                ▼
        Beads embedded Dolt DB (.muster/beads)
        Worktrees in .muster/wt/<bead-id>/
```

A single Go binary, `muster`. Defaults to `:7766`. Embeds the UI as static assets (`go:embed`) so the same binary serves the web UI and the API.

---

## 2. Module layout

```
cmd/muster/         main.go, flag parsing, embedded assets
internal/api/       HTTP routes, JSON DTOs, WS hub
internal/core/      Bead, Step, SubBead, Constitution, validation
internal/disp/      Scheduler, capacity guard, requeue/split policy
internal/cli/       Per-CLI adapters (Claude, Gemini, Codex, …) — wrappers around the vendor CLI binary
internal/tmux/      tmux session manager (spawn, attach, capture, kill, list)
internal/sdk/       SDK-based adapters (OpenCode via opencode SDK, …) — in-process, no shell-out
internal/apiprov/   Direct-API adapter (Anthropic, OpenAI, OpenRouter, …)
internal/wt/        Worktree create/diff/snapshot (per-bead choice: jj or git)
internal/store/     Beads (Dolt) wrapper, runlog append, sub-bead links
internal/quota/     Quota counter + reset windows
internal/skills/    Skill registry, prompt-assembly
internal/keychain/  macOS Keychain (and Linux libsecret) wrapper
internal/log/       Structured slog setup
```

Built-in adapters are `claude`, `gemini`, `codex`, with the opencode SDK adapter as a fifth path. From the dispatcher's perspective they share the same `Adapter` interface; the difference is **transport**: CLI adapters run their agent inside a tmux session managed by `internal/tmux` (see §7.1), the SDK adapter calls the opencode library in-process, and direct-API providers POST to the vendor endpoint.

---

## 3. Data model

All times are RFC3339 UTC. IDs use lowercase hex.

### 3.1 Bead

```go
type Bead struct {
    ID             string         // bd-a1f2 (or bd-a1f2.1 for sub-beads)
    ParentID       *string        // nil for roots
    Title          string
    Desc           string         // task instructions, markdown
    Type           IssueType      // bug | feature | task | epic | chore
    Column         Column         // backlog | scheduled | running | review | done
    Priority       int            // 0..4 (0=critical, 4=icebox)  — NOT P0..P3
    Labels         []string
    Branch         *string
    VCS            VCS            // jj | git  — per-bead, immutable once worktree exists
    PinnedAgent    *string        // bd pin <id> --for <agent>
    Steps          []Step
    Skills         []string       // DEPRECATED — skills are per-step only now
    Blocks         []string       // bead IDs this blocks
    BlockedBy      []string
    ExternalDeps   []string       // "external:repo/bd-xxx"
    DiscoveredFrom *string        // parent bead this was discovered from
    Acceptance     []ACItem       // { Text, Done }
    Gates          []Gate
    Comments       int            // derived count from History
    History        []HistoryEvent // lifecycle log
    TokensUsed     int64
    TokensBudget   int64
    Requeued       bool
    CreatedAt      time.Time
    UpdatedAt      time.Time
    LastActivity   time.Time
    SortKey        float64
}

type IssueType string  // "bug" | "feature" | "task" | "epic" | "chore"

type ACItem struct {
    Text string
    Done bool
}

type Gate struct {
    Kind   string // "human" | "timer" | "github"
    Label  string
    Status string // "waiting" | "passed" | "failed"
    Meta   map[string]any
}

type HistoryEvent struct {
    At     time.Time
    Kind   string  // opened | scheduled | claimed | started | paused | split | review | comment | approved | closed | reopened | requeued | blocked | unblocked | failed | discovered
    Actor  string  // "you@yours.dev" | "dispatcher" | "<agent-id>"
    Agent  *string // for claimed: which agent took it
    Note   *string
}

type Step struct {
    Index       int
    Agent       string           // provider ID, e.g. "claude"
    Mode        string           // mode ID OR workflow-stage alias (see §6.1)
    Skills      []string         // per-step skill IDs (no bead-level merge anymore)
    Prompt      *string          // nil = use default for mode+specSkill
    Status      StepStatus       // pending | active | done | failed | blocked
    Note        *string
    LoopBackTo  *int
    LoopMax     int              // ≥1; default 3
    LoopCount   int
    StartedAt   *time.Time
    FinishedAt  *time.Time
    TokensIn    int64
    TokensOut   int64
}

type VCS string // "jj" | "git"

type SubBead struct {
    ID         string  // bd-a1f2.1
    ParentID   string  // bd-a1f2
    Title      string
    Status     SubBeadStatus
    AgentHint  *string
    AutoSplit  bool   // dispatcher-spawned vs human-created
    CreatedAt  time.Time
}
```

### 3.2 Provider

```go
type Provider struct {
    ID            string         // claude | gemini | opencode | codex | <user-added>
    Kind          ProviderKind   // cli | sdk | api
    Name          string
    Color         string         // hex, used by UI step rail
    Mono          string         // 2-char monogram
    Plan          string         // free-form, e.g. "Claude Max"

    // CLI-specific
    Binary        string         // /opt/homebrew/bin/claude
    Version       string         // captured from `<binary> --version`
    Auth          AuthStatus     // LoggedIn | LoggedOut | Expired | NoAuthNeeded
    AuthAs        string         // email/user from `<binary> whoami` if available

    // API-specific
    BaseURL       string         // https://api.anthropic.com/v1
    APIKeyRef     string         // Keychain reference, never the raw key
    DefaultModel  string

    // Limits & native modes
    ParallelMax   int
    RateLimit     string         // free-form, "5h reset window"
    Modes         []Mode

    Quota         Quota          // current window
    Monthly       Quota
    SelfHosted    bool           // local / unmetered
}

type Mode struct {
    ID            string  // canonical mode identifier; agent-specific
    Name          string
    Desc          string
    CLI           string  // exact invocation, e.g. "claude --permission-mode plan"
    Icon          string  // single glyph for the UI
    Native        bool    // true = real provider CLI flag/SDK option;
                          // false = Muster synthesizes the stage via system-prompt shaping
    WorkflowStage string  // "plan" | "build" | "agent" | "review" | "yolo"
                          // Maps a provider-specific mode to Muster's workflow stages.
}

type Quota struct {
    Used       float64
    Limit      float64
    Unit       QuotaUnit  // dollar | token | message
    Window     string     // "today" | "this week" | "this month"
    ResetIn    string     // human ("8h"); also expose ResetAt RFC3339
    ResetAt    time.Time
}
```

### 3.3 Skill

```go
type Skill struct {
    ID           string   // "speckit" | "repo-grep" | …
    Name         string
    Desc         string
    Category     string   // spec | code | web | doc | design | pm | infra
    Icon         string   // single glyph, displayed by the UI
    PromptStub   string   // optional system-prompt fragment merged when skill loads
    MCPServers   []string // names of MCP servers the agent must have enabled for this skill
                          // these must already be registered in the CLI/agent's own config;
                          // Muster does not spawn or manage MCP servers
}
```

Muster never registers, spawns, or proxies MCP servers. When a skill lists `MCPServers`, the prompt assembly step verifies (best-effort) that the named servers appear in the agent's existing MCP configuration and emits a warning in the runlog if one is missing — but dispatch is not blocked.

### 3.4 Constitution

```go
type Constitution struct {
    Markdown   string
    UpdatedAt  time.Time
    Version    int  // monotonic; included in every dispatch
}
```

### 3.5 RunLogLine

```go
type RunLogLine struct {
    BeadID   string
    StepIdx  int
    At       time.Time
    Kind     RunKind // system | tool | thought | output | error
    Msg      string
    // For tool calls, optional structured detail:
    Tool     *ToolCall
}
```

### 3.6 Storage layout

- **Beads** (Dolt): `beads`, `sub_beads`, `steps`, `step_runs`, `bead_links`, `runlog`.
- **Worktrees**: `.muster/wt/<bead-id>/` (one per running bead; backend determined by `Bead.VCS`).
- **Config**: `.muster/config.toml` (providers, capacity, constitution path).
- **Constitution**: `.muster/constitution.md` (single file; UI edits go here).
- **Keys**: macOS Keychain (`com.muster.<provider-id>`) or libsecret on Linux.

---

## 4. HTTP + WS API

Versioned under `/api/v1`. JSON request/response, UTF-8, RFC3339 timestamps, lowercase-hex IDs. Single-user in v1 — the daemon refuses non-localhost connections unless `--bind 0.0.0.0` is set (see §4.7). All endpoints are case-sensitive.

### 4.0 API stability

- The `v1` prefix is a stability promise: no breaking changes to existing fields or status semantics within `v1`. **Additive** changes (new fields, new enum values, new optional query params, new endpoints) are non-breaking and may land in any release. Clients **must** ignore unknown fields and tolerate unknown enum values gracefully.
- New WS event types may be added at any time. Clients must `default:` unknown `type` values to a no-op.
- A removal or semantic change ships under `/api/v2` running side-by-side; `v1` keeps responding until the next major release.
- The daemon advertises its current build in `GET /api/v1/orchestrator/status` (`{ build, schemaVersion, beadsVersion }`). UI builds older than `minClientVersion` (returned in the same payload) get a sticky banner asking for a refresh.

### 4.1 REST surface

```
GET    /api/v1/beads                        list (filterable: column, agent, priority, repo, label, since, cursor, limit)
POST   /api/v1/beads                        create  (Idempotency-Key supported)
GET    /api/v1/beads/{id}                   detail
PATCH  /api/v1/beads/{id}                   partial update (column, title, desc, labels, vcs (pre-worktree), pinnedAgent, …)
DELETE /api/v1/beads/{id}                   delete (refused if running; returns INVALID_STATE)
POST   /api/v1/beads/{id}/move              { toColumn, beforeID?, position? }
POST   /api/v1/beads/{id}/schedule          → scheduled
POST   /api/v1/beads/{id}/dispatch          { agent?, mode? }  force-running; agent override respects PinnedAgent precedence
POST   /api/v1/beads/{id}/pause             → pause running
POST   /api/v1/beads/{id}/approve           review → done
POST   /api/v1/beads/{id}/reopen            done|review → running
POST   /api/v1/beads/{id}/pin               { agent }    or  DELETE to unpin
POST   /api/v1/beads/{id}/comments          { text }     appends a `comment` history event (returns the event)
PATCH  /api/v1/beads/{id}/acceptance        { items: ACItem[] }   replaces the full AC list

GET    /api/v1/beads/{id}/steps             chain
PUT    /api/v1/beads/{id}/steps             replace whole chain
PATCH  /api/v1/beads/{id}/steps/{idx}       update a single step
POST   /api/v1/beads/{id}/steps             append step
DELETE /api/v1/beads/{id}/steps/{idx}       remove

GET    /api/v1/beads/{id}/runlog?since=&limit=     paged log (cursor by line id)
GET    /api/v1/beads/{id}/steps/{idx}/attach       tmux attach command + pane info for a CLI step
POST   /api/v1/beads/{id}/steps/{idx}/send         { keys }     forward keystrokes to the live tmux pane (CLI only)
GET    /api/v1/beads/{id}/worktree                 file list + diff summary
GET    /api/v1/beads/{id}/diff[?path=]             unified diff (whole bead or single file)
GET    /api/v1/beads/{id}/subbeads                 list children
POST   /api/v1/beads/{id}/subbeads                 create sub-bead (inherits parent VCS by default)

GET    /api/v1/providers                    list
POST   /api/v1/providers                    add (CLI / SDK / Direct-API)  (Idempotency-Key supported)
PATCH  /api/v1/providers/{id}               update (parallel_max, default_model, base_url, …)
DELETE /api/v1/providers/{id}               refused while a bead is dispatched on this provider
POST   /api/v1/providers/{id}/login         kick off interactive CLI / SDK login (returns LoginFlow)
POST   /api/v1/providers/{id}/test          test invocation; returns adapter result + parsed quota snapshot
GET    /api/v1/providers/{id}/quota         current quota snapshot
GET    /api/v1/providers/cli-catalog        the known CLI catalogue (each entry { detected, version?, binary? })
GET    /api/v1/providers/sdk-catalog        known SDK-based providers (each entry { linked, version? })
GET    /api/v1/providers/api-catalog        known direct-API providers

GET    /api/v1/skills                       full skill registry
GET    /api/v1/skills/categories
POST   /api/v1/skills                       { url } — import a skill from a URL
DELETE /api/v1/skills/{id}                  only user/url-sourced skills; built-ins are read-only

GET    /api/v1/constitution                 { markdown, updatedAt, version }
PUT    /api/v1/constitution                 { markdown }   bumps version, emits constitution.changed

GET    /api/v1/repos                        list attached repos with per-repo bead counts + last sync
POST   /api/v1/repos                        attach a probed path { path, mode?, name? }
DELETE /api/v1/repos/{id}                   detach (does not delete .beads/)
PATCH  /api/v1/repos/{id}                   update (name, isDefault, vcsBranch override, …)
POST   /api/v1/repos/probe                  { path }  — returns parsed .beads/ config + counts (used by the AddRepoModal)
POST   /api/v1/repos/{id}/rescan            force re-scan; emits repo.scanned WS event
POST   /api/v1/repos/{id}/migrate           run pending bd migrations; emits repo.scanned on completion

GET    /api/v1/memories                     list (?repo=, ?q=)
POST   /api/v1/memories                     { key, value }  upsert (Idempotency-Key supported)
DELETE /api/v1/memories/{key}
POST   /api/v1/memories/prime               { beadID }  load memories into the next dispatch of this bead

GET    /api/v1/formulas                     list (id + name + desc only)
GET    /api/v1/formulas/{id}                full formula with step chain, gates, vars
POST   /api/v1/cook                         { formula, title, vars }  shortcut: create bead + molecule from a formula

GET    /api/v1/routes                       list rules
PUT    /api/v1/routes                       replace all rules (atomic; bumps routes version)
POST   /api/v1/routes                       append rule
DELETE /api/v1/routes/{pattern}             remove rule (pattern URL-encoded)
POST   /api/v1/routes/test                  { title, labels[] } → matched rule (or `*` fallback)

GET    /api/v1/hydrate                      list sibling repos with ahead-count + last-sync
POST   /api/v1/hydrate                      { from: "repo-id" }   pull
POST   /api/v1/hydrate/dry-run              { from: "repo-id" }   preview only, no writes

GET    /api/v1/orchestrator/capacity        per-provider slots + queued
GET    /api/v1/orchestrator/policy          failure policy (see §5.3)
PUT    /api/v1/orchestrator/policy
GET    /api/v1/orchestrator/status          { build, schemaVersion, beadsVersion, online, runningCount, worktreeCount, tmuxAvailable, tmuxVersion, dolt: { branch, status, ahead, behind, writers, port, lastSync } }
POST   /api/v1/orchestrator/reload          hot-reload config from disk (equivalent to SIGHUP)
POST   /api/v1/orchestrator/halt            stop the dispatcher loop (does not kill running beads); kill-switch
POST   /api/v1/orchestrator/resume          resume the dispatcher

POST   /api/v1/dolt/pull                    bd dolt pull (returns updated DOLT snapshot)
POST   /api/v1/dolt/push                    bd dolt push
GET    /api/v1/dolt/status                  DOLT snapshot, separate from /orchestrator/status

GET    /api/v1/cli/commands                 the bd command catalogue used by the command palette
GET    /api/v1/healthz                      liveness probe (200 OK / 503 if dispatcher loop is stuck)
GET    /api/v1/metrics                      Prometheus text-format (see §12)
```

### 4.1.1 Verb conventions
- `GET` is always safe + idempotent + cacheable (clients use ETag where returned).
- `POST` for creates and for state transitions (`/dispatch`, `/pause`, …). State transitions are idempotent under `Idempotency-Key` (see §4.5).
- `PATCH` for partial updates of resources. Body is a sparse object — only included keys are touched.
- `PUT` for total replacement (full step chain, full routes list, constitution markdown).
- `DELETE` returns `204 No Content` on success or `400 INVALID_STATE` if the resource is in use.

### 4.2 WebSocket

`GET /api/v1/stream` — single WS connection per UI; server pushes typed events. Negotiated protocol: JSON text frames, one event per frame, no batching. The server **must** send a `hello` event within 1s of connect carrying the build + schema version; clients reload if the build doesn't match what `/orchestrator/status` returned at boot.

Clients ping every 25s with `{ "type": "ping" }`. Server replies with `{ "type": "pong", "at": "…" }`. Server forcibly closes idle connections after 90s without traffic.

Event envelope:

```json
{ "type": "<event>", "seq": 12345, "at": "2026-05-22T10:14:31Z", ...payload }
```

`seq` is monotonic per connection. On reconnect, clients pass `?since=<seq>` to replay missed events from a 5-minute server-side ring buffer; events older than the buffer fall back to a full re-fetch.

Known event types (clients ignore anything else):

```json
{ "type": "hello",             "build": "…", "schemaVersion": 4, "beadsVersion": "0.9.1", "serverTime": "…" }
{ "type": "bead.created",      "bead": { … } }
{ "type": "bead.updated",      "bead": { … } }
{ "type": "bead.moved",        "id": "bd-a1f2", "fromColumn": "scheduled", "toColumn": "running", "beforeID": null }
{ "type": "bead.deleted",      "id": "bd-a1f2" }
{ "type": "comment.added",     "beadID": "bd-a1f2", "event": { … HistoryEvent … } }
{ "type": "step.status",       "beadID": "bd-a1f2", "stepIdx": 2, "status": "active", "loopCount": 0 }
{ "type": "runlog.line",       "beadID": "bd-a1f2", "stepIdx": 2, "line": { … RunLogLine … } }
{ "type": "worktree.changed",  "beadID": "bd-a1f2", "files": [ … FileChange … ] }
{ "type": "quota.tick",        "providerID": "claude", "quota": { … } }
{ "type": "capacity.changed",  "providerID": "claude", "running": 3, "queued": 2, "limit": 3 }
{ "type": "tmux.session.opened","beadID": "bd-a1f2", "stepIdx": 2, "name": "muster/bd-a1f2/2/0" }
{ "type": "tmux.session.closed","beadID": "bd-a1f2", "stepIdx": 2, "name": "muster/bd-a1f2/2/0" }
{ "type": "subbead.created",   "subbead": { … } }
{ "type": "subbead.completed", "id": "bd-a1f2.1", "parentID": "bd-a1f2" }
{ "type": "constitution.changed", "version": 12 }
{ "type": "provider.added",    "provider": { … } }
{ "type": "provider.removed",  "id": "opencode" }
{ "type": "repo.added",        "repo": { … } }
{ "type": "repo.scanned",      "id": "frontend-repo", "counts": { … } }
{ "type": "repo.detached",     "id": "frontend-repo" }
{ "type": "memory.changed",    "repo": "main", "key": "auth-pattern", "deleted": false }
{ "type": "dolt.tick",         "snapshot": { branch, status, ahead, behind, lastSync, … } }
{ "type": "dispatcher.halted", "reason": "user" }
{ "type": "dispatcher.resumed" }
{ "type": "pong",              "at": "…" }
```

### 4.3 Error shape

```json
{ "error": { "code": "BEAD_NOT_FOUND", "message": "no such bead", "details": {}, "requestID": "…" } }
```

Every response carries an `X-Request-ID` header; the same id is echoed in the error body for grep-ability against `slog` output (§12).

#### Error catalogue

| Code                          | HTTP | Meaning / when emitted                                              |
|-------------------------------|------|---------------------------------------------------------------------|
| `BEAD_NOT_FOUND`              | 404  | id doesn't resolve to any bead or sub-bead                          |
| `STEP_NOT_FOUND`              | 404  | step index out of range for the named bead                          |
| `PROVIDER_NOT_CONFIGURED`     | 400  | step references an agent that no Provider row matches               |
| `PROVIDER_AUTH_REQUIRED`      | 401  | provider exists but is `logged-out` / `expired`                     |
| `PROVIDER_IN_USE`             | 409  | DELETE blocked because a bead is currently dispatched on it         |
| `INVALID_STATE`               | 400  | transition not allowed from current column (e.g. approve from backlog)|
| `INVALID_REQUEST`             | 400  | body schema invalid / required field missing                        |
| `QUOTA_EXHAUSTED`             | 429  | dispatch refused; bead returned to `scheduled` with `Requeued=true` |
| `RATE_LIMITED`                | 429  | client-side rate limit hit (see §4.6)                              |
| `WORKTREE_DIRTY`              | 409  | start blocked: worktree has uncommitted changes from a prior run    |
| `WORKTREE_LOCKED`             | 409  | another process holds the worktree lock                             |
| `VCS_UNAVAILABLE`             | 412  | required `jj`/`git` binary not on `$PATH`                           |
| `VCS_LOCKED`                  | 409  | `Bead.VCS` change rejected because a worktree already exists         |
| `TMUX_UNAVAILABLE`            | 412  | tmux missing; CLI adapter falls back, attach disabled               |
| `ROUTE_CONFLICT`              | 409  | duplicate pattern at the same priority                              |
| `REPO_NOT_FOUND`              | 404  | repo id doesn't match an attached working tree                      |
| `REPO_PROBE_FAILED`           | 422  | `POST /repos/probe` couldn't read `.beads/` at the given path       |
| `SCHEMA_MIGRATION_REQUIRED`   | 412  | repo's `.beads/` schema is older than the daemon; run `bd migrate`  |
| `IDEMPOTENCY_REPLAY_MISMATCH` | 409  | Idempotency-Key reused with a different body (see §4.5)             |
| `INTERNAL`                    | 500  | unexpected; client surfaces the requestID and offers a retry        |

Clients **must** surface unknown codes verbatim rather than guessing at semantics.

### 4.4 Pagination

List endpoints (`/beads`, `/beads/{id}/runlog`, `/memories`) accept:

- `?limit=N` (default 100, max 1000 for `/beads`, max 5000 for `/runlog`)
- `?cursor=<opaque>` returned from the previous page's `nextCursor` field
- `?since=<RFC3339 | seq | line-id>` for incremental fetches

Response envelope for paged GETs:

```json
{ "items": [ … ], "nextCursor": "…" | null, "total": 137 }
```

`total` is best-effort and may be elided for very large sets.

### 4.5 Idempotency

Mutating endpoints that can plausibly be retried (`POST /beads`, `POST /beads/{id}/dispatch`, `POST /beads/{id}/comments`, `POST /providers`, `POST /memories`, `POST /cook`) honour an `Idempotency-Key: <opaque>` request header.

- A key reused within 24h with the **same body hash** returns the original 2xx response. The server stores `(key → (status, body-hash, response))` in Dolt.
- A key reused with a **different body hash** returns `409 IDEMPOTENCY_REPLAY_MISMATCH`.
- Clients should use a UUIDv4 per logical action; the UI generates one per drag, dispatch click, etc.

### 4.6 Rate limiting

The daemon is single-user, so v1 rate limits are advisory and protect the daemon from runaway clients:

- 50 req/s per connection across the whole REST surface.
- 10 WS frames/s per connection (inbound).
- 100 `runlog` page fetches/min per bead (the UI never needs more — it should be using the WS stream).

Exceeding any of these returns `429 RATE_LIMITED` with `Retry-After: <seconds>`.

### 4.7 Authentication & CORS

- v1 is single-user, single-machine. The daemon **refuses** non-localhost binds unless the operator explicitly passes `--allow-external`, and even then only when an API key is configured.
- When `--allow-external` is set, every request must carry `Authorization: Bearer <token>` where the token matches the value in `[server] api_token` (or `muster_API_TOKEN`). The WS connection passes the same token as a `?token=` query param (browsers can't set headers on WS upgrades).
- CORS: in localhost mode the server emits `Access-Control-Allow-Origin: http://localhost:*`. In `--allow-external` mode the allow-list is the `[server] cors_origins` array from config.
- Future: real multi-user, real OIDC. Tracked in §15.

---

## 5. Dispatcher loop

A single goroutine consumes a tickless work queue. Roughly:

```
for {
    select {
    case <-shutdown:
        return
    case ev := <-events:
        applyEvent(ev)            // bead added, step finished, quota replenished
    }

    candidates := beadsIn("scheduled").
        filter(noBlockers).
        filter(noWaitingHumanGates).
        orderBy(priority, sortKey, createdAt)

    for _, bead := range candidates {
        // Respect bd pin: if PinnedAgent set, only dispatch to that agent.
        agent := bead.PinnedAgent
        if agent == nil { agent = agentForNextStep(bead) }

        if !capacityAvailable(agent) { continue }
        if !quotaAllows(agent, estimate(bead)) { continue }
        start(bead, agent)        // moves bead → "running", emits events
    }
}
```

### 5.1 Capacity

Per-provider semaphore (`chan struct{}` sized to `ParallelMax`). `start()` takes a token; step finalisation returns it. Capacity changes propagate via `capacity.changed` events.

### 5.2 Quota

Each provider has a `quota.Tracker` updated after every step from CLI stdout (or API response headers). Tracker fires `quota.tick` events. Quotas are persisted so reboot doesn't lose accounting; the daily/weekly/monthly resets are wall-clock events.

### 5.3 Failure policy

Configurable in `orchestrator/policy`. Defaults:

| Trigger                                | Action                                          |
|----------------------------------------|-------------------------------------------------|
| Token budget exhausted                 | Move bead to `scheduled`, set `Requeued=true`, carry runlog. |
| `tokensUsed/tokensBudget > 0.8` and `stepsDone/stepsTotal < 0.5` | Auto-split into N sub-beads (configurable, default 3). |
| Step failed twice                      | Escalate `agent` to next provider in escalation list, retry. |
| Bead enters `review`                   | Auto-run a VCS skill step beforehand if missing (`jj describe` for jj beads; `git commit` for git beads). |

---

## 6. Step execution

### 6.1 Modes — native vs synthesized

Provider modes are split into two kinds:

- **Native modes** — backed by a real CLI flag or SDK option. Example: `claude --permission-mode plan` is a real Claude Code permission mode. `gemini --yolo` is a real Gemini CLI flag.
- **Synthesized modes** — Muster workflow stages (`plan`, `build`, `review`) that don't correspond to a provider CLI flag. Muster runs the bare invocation and shapes behavior via a system-prompt prefix.

Claude Code's real modes are about **permissions**, not workflow stages:

| Mode ID       | CLI                                              | Native | Maps to stage |
|---------------|--------------------------------------------------|--------|---------------|
| plan          | `claude --permission-mode plan`                  | yes    | plan          |
| acceptEdits   | `claude --permission-mode acceptEdits`           | yes    | build         |
| default       | `claude`                                         | yes    | agent         |
| bypass        | `claude --permission-mode bypassPermissions`     | yes    | yolo          |
| review        | `claude --permission-mode plan` + review prompt  | no     | review        |

**Step references can use either the canonical mode ID or its workflow-stage alias.** `modeMeta("claude", "agent")` resolves to the `default` mode via `_stageAliases`. This lets formulas and seed data refer to workflow stages portably across providers, while the dispatcher still emits the correct provider-specific CLI.

When a step's mode is **synthesized**, the dispatcher prepends a `<system role="muster-stage">` block to the prompt naming the stage (e.g. "This step is a code review. Read-only. Produce inline comments + a summary. Do not write files.") and runs the provider's default invocation.

Non-Claude providers' non-default modes are currently marked synthesized pending verification of upstream CLI surface area.

### 6.2 Step assembly

A step is one invocation. Sequence:

1. **Assemble the prompt.**
   - Header: constitution markdown (versioned).
   - Body: bead `Desc` + previous-step output summaries (when relevant — plan output flows into build).
   - Step prompt (default or override).
   - Skill loadout: `bead.Skills ∪ step.Skills`. Each skill's `PromptStub` is appended. If a skill lists `MCPServers`, those names are referenced in the prompt so the agent uses its already-configured servers.
2. **Resolve mode → CLI invocation.** For provider `claude` mode `plan`: `claude --plan --no-streaming`. The CLI invocation comes from `Provider.Modes[modeID].CLI`.
3. **Create or claim worktree.** First step of a bead calls `wt.For(bead).Create(beadID, srcRepo)`, which branches on `Bead.VCS`: `jj clone --colocate` for jj beads, `git worktree add -b muster/<bead-id>` for git beads. Subsequent steps reuse the existing worktree.
4. **Spawn the worker.**
   - **CLI adapters** (`claude`, `gemini`, `codex`, `cli-generic`): launch the CLI inside a dedicated **tmux session** (see §7.1). The session name is `muster/<bead-id>/<step-idx>`; the working dir is the worktree; the prompt is fed in via stdin (or a temp file the wrapper script `cat`s). Output is read via `tmux pipe-pane`, not by reading stdout of a direct child — this is what lets the user attach to a live agent.
   - **SDK adapters** (`opencode`): call the SDK's session/run API in-process with the same assembled prompt and worktree path. No subprocess, no tmux; events come back via SDK callbacks/channels.
5. **Stream output.** Tee stdout/stderr into:
   - The `runlog` table (append-only, batched flush).
   - The WS hub (`runlog.line` events).
   - A token counter that updates `Step.TokensIn/Out` and the provider's `Quota`.
6. **Parse special markers.** Agents emit lines like `<muster:subbead title="…">` to spawn sub-beads, `<muster:checkpoint>` to checkpoint to Beads memory. Markers are stripped before logging.
7. **On exit code 0:** mark step `done`. Persist files changed via `wt.For(bead).DiffSummary()` (delegates to `jj diff --summary` or `git diff --name-status` per `Bead.VCS`). Emit `worktree.changed`.
8. **On non-zero:** mark step `failed`. Apply failure policy. If `LoopBackTo != nil` and `LoopCount < LoopMax`, increment and reactivate the loop target step.
9. **Advance step pointer.** If last step, move bead → `review` (or `done` if no review step in chain).

### 6.1 Special case: SDK adapters (OpenCode)

The opencode SDK adapter (`internal/sdk/opencode`) implements the same `Adapter` interface but **never shells out**. Instead:

- It maintains a long-lived opencode session per running bead, scoped to the bead's worktree.
- The assembled prompt (constitution + bead ticket + skill loadout) is passed to the SDK as the session input.
- SDK events (tool calls, thoughts, output chunks, completion) are translated into `RunEvent` values and pushed into the same runlog pipeline used by CLI adapters.
- Cancellation cancels the SDK call via the bead's `context.Context`.
- Quota/cost is read from the SDK's usage reporting rather than parsed from stdout.

This means the agent input for opencode is identical to every other provider: **the bead ticket** (title + desc + acceptance criteria) plus the constitution and step prompt. The SDK is only an alternative transport.

### 6.2 Agent input contract

Regardless of adapter kind (CLI, SDK, direct-API), the **input the agent sees is the bead ticket**:

- `Bead.Title` and `Bead.Desc` are the task statement — markdown, free-form, treated as the source of truth.
- `Bead.Skills ∪ Step.Skills` determine the skill loadout merged into the system prompt.
- Earlier-step output summaries are appended so a `build` step sees what `plan` produced.
- The step prompt (default or overridden) frames *what to do* with the ticket in this mode.

Adapters do not invent or reshape the ticket; they only translate the assembled prompt into whatever shape their transport requires (stdin for CLIs, session input for SDKs, `messages` array for direct APIs).

### 6.3 Special case: direct-API providers

The direct-API adapter (`apiprov`) implements the same interface but instead of `exec.Command`:

- Wraps the prompt with synthetic mode framing (`<system>You are in PLAN mode…</system>`).
- POSTs to `Provider.BaseURL/chat/completions` (or `/messages` for Anthropic).
- Streams responses via SSE, feeds them into the same runlog pipeline.

Direct-API providers don't have native modes; the adapter synthesises `plan/build/agent/review` via system-prompt prefixes. The UI already warns about this.

---

## 7. CLI adapters

CLI adapters never `exec.Command` the agent binary directly. They go through `internal/tmux`, which is the canonical transport for any CLI agent. This buys us:

- **Live attach.** The user can `tmux attach -t muster/<bead-id>/<step-idx>` (or click the UI's *Attach* button which prints the command) to watch the agent in real time, page through its TUI, scroll back, or intervene by typing.
- **Survives muster restart.** tmux runs under the user's session, not the daemon. If `muster` crashes or is restarted, the agent keeps running; on reconnect, the dispatcher re-discovers active sessions via `tmux list-sessions -F '#{session_name}'` filtered to the `muster/` prefix and resumes streaming from `tmux pipe-pane`.
- **One source of truth for output.** Both the runlog and the live attach view read the same pane; no risk of the user seeing something the runlog doesn't have.
- **TTY-aware agents work correctly.** Claude Code, Codex CLI etc. detect they're on a TTY (because tmux gives them one) and render their full TUI, ANSI colors, progress UI — instead of falling into a degraded non-TTY mode.

Each adapter implements:

```go
type Adapter interface {
    ID() string
    Detect(ctx context.Context) (DetectResult, error) // is it installed? what version?
    Modes() []Mode
    Invoke(ctx context.Context, req InvokeReq) (<-chan RunEvent, error)
    QuotaSource() QuotaSource    // CLI output parser OR API headers
    Login(ctx context.Context) (LoginFlow, error)
}
```

Built-in CLI adapters: `claude`, `gemini`, `codex`. Plus the generic `cli-generic` adapter for user-added CLIs (no quota tracking, no native modes, synthesised same way as direct API). All of them delegate process management to `internal/tmux`.

**SDK adapters and `Login()`.** The `Adapter` interface includes `Login(ctx) (LoginFlow, error)`. For CLI adapters this shells out to the vendor's interactive login command. For the opencode SDK adapter, `Login()` delegates to the SDK's own auth flow (no subprocess). Adapters that require no auth (e.g. self-hosted direct-API) return `ErrNotSupported`.

### 7.1 tmux integration (`internal/tmux`)

One tmux session per running step. The `tmux` package wraps the binary with these primitives:

```go
type Session struct {
    Name     string  // "muster/bd-a1f2/2"
    BeadID   string
    StepIdx  int
    Pane     string  // "muster/bd-a1f2/2:0.0"
    StartedAt time.Time
}

type Manager interface {
    Detect() error                                        // verify tmux >= 3.2 is installed
    Spawn(name, cwd string, env []string, argv []string) (*Session, error)
    Attach(name string) (cmd string, err error)           // returns the shell command for the user
    Pipe(name string) (io.ReadCloser, error)              // pipe-pane to a fifo; returns the read side
    Send(name, keys string) error                         // forward user input from the UI
    Capture(name string, lines int) (string, error)       // capture-pane for catch-up after restart
    Kill(name string) error
    List() ([]Session, error)                             // sessions with the "muster/" prefix
}
```

**Session naming.** Sessions are named `muster/<bead-id>/<step-idx>/<loop-count>` (e.g. `muster/bd-a1f2/1/0`). Including the loop count prevents name collisions when `LoopBackTo` causes the same step index to be re-run. On each loop iteration, the previous session (if any) is killed before spawning the new one.

**Prompt delivery.** The assembled prompt is written to a temp file in the worktree (`<worktree>/.muster-prompt-<step>.txt`). The tmux session runs a thin wrapper: `sh -c 'cat .muster-prompt-<step>.txt | <agent-cmd> <agent-args…>'`. Using a file avoids shell-escaping hazards with multi-line prompts passed via `-c` arguments.

Spawn flow (CLI adapter, simplified):

```
# Write prompt to temp file
echo "$PROMPT" > <worktree>/.muster-prompt-<step>.txt

tmux new-session -d -s muster/<bead-id>/<step-idx>/<loop-count> \
     -c <worktree-path> \
     -x 220 -y 50 \
     -- sh -c 'cat .muster-prompt-<step>.txt | <agent-cmd> <agent-args…>'

tmux pipe-pane -t muster/<bead-id>/<step-idx>/<loop-count> -o 'cat >>/path/to/runlog.fifo'
```

The dispatcher then reads `runlog.fifo`, ANSI-strips for the runlog table, and forwards the raw bytes to any UI client that opens `GET /api/v1/beads/{id}/steps/{idx}/attach` (see §4.1).

If `tmux` is not installed, CLI adapters fall back to direct `exec.Command` and the *Attach* button is disabled with a tooltip explaining why. tmux is detected once at startup and exposed in `GET /api/v1/orchestrator/status` as `tmuxAvailable: bool`.

**Daemon restart recovery.** On startup, `tmux.Manager.List()` returns every `muster/*` session. For each, the dispatcher looks up the matching bead/step in Beads, marks the step as `active` if it isn't already, reopens the pipe, and resumes streaming. If a session's bead is gone or the step is `done`, the session is killed.

**Session lifecycle.** On step `done`/`failed`, the dispatcher kills the session after a configurable grace period (`[orchestrator] tmux_grace = "30s"`) so the user has time to scroll back if they want. After grace expires, `tmux kill-session` is called and the runlog buffer is flushed.

SDK adapters (opencode) and direct-API adapters do **not** use tmux — they have no terminal-shaped process to host. The UI's *Attach* button is hidden for steps running on those providers.

`InvokeReq` carries: worktree path, mode, prompt, skill loadout, model override.

`RunEvent` is a union: `Stdout`, `Thought`, `ToolCall`, `Subbead`, `Checkpoint`, `Exit`. Adapters do the parsing; the dispatcher consumes a normalised stream.

### Sub-bead worktree ownership

When the dispatcher auto-splits a bead into sub-beads, each sub-bead gets its own worktree. Sub-beads inherit `Bead.VCS` from their parent. Manual sub-beads created via `POST /api/v1/beads/{id}/subbeads` default to the parent's VCS but can override it before their first dispatch.

Sub-bead worktrees are independent; the parent's worktree is untouched while sub-beads run. On sub-bead approval the dispatcher emits a `subbead.completed` WS event and leaves merging to the user (or a dedicated merge step in the parent's chain). Auto-merge is out of scope for v1 (see open question 3).

---

## 8. Worktree management (`internal/wt`)

Each bead picks its own worktree backend via `Bead.VCS` (`jj` or `git`). At startup, `wt` probes both binaries and records which are available; the UI shows only the available ones in the ticket's VCS picker. If a bead's chosen backend isn't installed at dispatch time, the bead is moved back to `scheduled` with a `VCS_UNAVAILABLE` note rather than silently falling back.

Default for new beads comes from `[orchestrator] default_vcs` in `config.toml` (`"jj"` out of the box, or `"git"` if jj isn't detected at `muster init`).

- One worktree per running bead, named `wt-<bead-suffix>`.
- **jj beads** — created via `jj clone --colocate` (or `jj workspace add` for an existing jj repo). On approval: `Finalize` runs `jj describe` → `jj push` if a remote is configured.
- **git beads** — created via `git worktree add -b muster/<bead-id> .muster/wt/<bead-id>/`. On approval: `Finalize` runs `git commit -m "$(spec)"` (or amend) → `git push` if a remote is configured.
- On bead rejection → keep worktree until manual cleanup.
- Worktrees idle for > 7 days are GC'd with the daemon's `muster gc` subcommand. GC removes the worktree using the correct backend (`jj workspace forget` vs `git worktree remove`).
- `Bead.VCS` is **immutable once a worktree exists**. The API rejects `PATCH /beads/{id}` requests that change `vcs` after the bead has been dispatched at least once; switching backends mid-bead would orphan the worktree.

The `wt` package exposes a single interface so the dispatcher and diff endpoints don't branch on backend:

```go
type Backend interface {
    Status(beadID string) (WorktreeStatus, error)   // exists? clean? ahead/behind?
    Create(beadID string, srcRepo string) (path string, err error)
    DiffSummary(beadID string) ([]FileChange, error)
    Diff(beadID string, path string) (io.ReadCloser, error)
    Finalize(beadID string, msg string) error        // jj describe / git commit
    Push(beadID string) error
    Remove(beadID string) error
}
```

Concrete implementations: `wt.JJ{}` and `wt.Git{}`. `wt.For(bead)` returns the right one.

### 8.1 Diff exposure

The UI's "Worktree" tab calls `GET /beads/{id}/worktree` which returns the parsed output of the backend's `DiffSummary` (`jj diff --summary` or `git diff --name-status`). The "Diff" tab streams the backend's per-file diff (`jj diff -r @ <path>` or `git diff <path>`).

---

## 9. Constitution & skill assembly

The full prompt sent to an agent for one step is:

```
<system role="muster">
${constitution.Markdown}

# Step ${i+1} of ${n}: ${mode} mode
Provider: ${provider.Name}
Skills loaded:
  ${for each merged skill: name — promptStub.firstLine}

Bead ${bead.ID}: ${bead.Title}
Acceptance criteria:
${bead.Desc}

Earlier-step summaries:
${for each done step: one-line summary from runlog}
</system>
<user>
${step.Prompt || defaultPromptFor(mode, specSkill)}
</user>
```

Skill `PromptStub` is one short paragraph. If a skill lists `MCPServers`, those server names are referenced by name in the prompt stub so the agent knows to use them — but they must already be registered in the agent's own MCP config. Muster does not manage MCP server lifecycle.

`defaultPromptFor(mode, specSkill)` returns the same text the UI shows in the step editor — the backend and UI share the table.

---

## 9a. Multi-agent routing & work assignment

Per [Beads multi-agent](https://gastownhall.github.io/beads/multi-agent), Muster surfaces three coordination primitives:

### 9a.1 Routes — `.beads/routes.jsonl`

Glob patterns matched against issue title + labels; first-match-wins by priority. Used when creating a bead to auto-route it to the correct repository.

```jsonl
{"pattern": "frontend/**", "target": "frontend-repo", "priority": 10}
{"pattern": "backend/**",  "target": "backend-repo",  "priority": 10}
{"pattern": "*",            "target": "main-repo",     "priority": 0}
```

API surface:

```
GET    /api/v1/routes                     list rules
PUT    /api/v1/routes                     replace rules
POST   /api/v1/routes/test                { title, labels[] } → matched rule
POST   /api/v1/routes                     add rule
DELETE /api/v1/routes/{pattern}           remove rule
```

### 9a.2 Work assignment — `bd pin`

`Bead.PinnedAgent` is set via `POST /api/v1/beads/{id}/pin {agent}`. The dispatcher checks `PinnedAgent` first and only considers that agent for the next step (capacity & quota gating still apply). Unpinning falls back to capacity-based suggestion.

### 9a.3 Hydration — `bd hydrate`

Pull related issues from sibling repos via Dolt remote sync.

```
GET    /api/v1/hydrate                       list known sibling repos + ahead-count
POST   /api/v1/hydrate                       { from: "repo-id" }  — pull
POST   /api/v1/hydrate/dry-run               { from: "repo-id" }  — preview
```

### 9a.4 Cross-repo dependencies

Stored as `"external:<repo>/<bead-id>"` strings in `Bead.ExternalDeps`. They do **not** block `bd ready` in the local repo — they're soft references for the dep graph view and changelog generation.

---

## 10. Config (`.muster/config.toml`)

```toml
[server]
bind = "127.0.0.1:7766"
allow_external = false           # bind on non-localhost; requires api_token
api_token = ""                   # required when allow_external=true; also from $muster_API_TOKEN
cors_origins = []                # additional allowed origins when allow_external=true
log_level = "info"               # debug | info | warn | error
log_format = "json"              # json | text
request_id_header = "X-Request-ID"

[orchestrator]
auto_split = true
escalate_on_double_fail = false
require_vcs_step_before_review = true   # was require_jj_step_before_review
default_vcs = "jj"               # "jj" or "git" — default backend for new beads
tmux_grace = "30s"               # how long to keep a tmux session alive after step completion
runlog_retention = "30d"         # runlog rows older than this are squashed by `muster gc`
worktree_idle_gc = "7d"          # worktrees idle longer than this are removed by `muster gc`
ws_buffer_seconds = 300          # WS replay buffer (seconds) for ?since= reconnects
idempotency_ttl = "24h"          # Idempotency-Key dedup window

[[providers]]
id = "claude"
kind = "cli"
binary = "/opt/homebrew/bin/claude"
parallel_max = 3

[[providers]]
id = "gemini"
kind = "cli"
binary = "/opt/homebrew/bin/gemini"
parallel_max = 2

[[providers]]
id = "anthropic-api"
kind = "api"
base_url = "https://api.anthropic.com/v1"
api_key_ref = "keychain:com.muster.anthropic-api"
default_model = "claude-sonnet-4-5"
parallel_max = 4

[constitution]
path = ".muster/constitution.md"

[telemetry]
metrics_enabled = true           # serves /api/v1/metrics
healthz_grace = "3s"             # dispatcher-loop lag tolerance before /healthz flips 503
```

Hot-reloadable via `SIGHUP` or `POST /api/v1/orchestrator/reload`. Hot-reload covers everything under `[orchestrator]` and `[telemetry]`, the constitution path, and the providers list. It does **not** cover `[server]` (changing the bind address or API token requires a restart).

---

## 11. Concurrency model

- **One** dispatcher goroutine.
- **One** WS hub goroutine.
- **N** active-step goroutines (one per running step). Capped by total `Σ ParallelMax`.
- **One** quota-tick goroutine that fires reset events on wall-clock boundaries.
- All shared state goes through `internal/store` which serialises writes to Beads (Dolt is single-writer in embedded mode).

Cancellation is contextual: each step has a `context.Context` tied to the bead. Pausing or moving a running bead cancels its current step.

---

## 12. Observability

### 12.1 Structured logs

- **slog** JSON to stderr by default (`log_format = "text"` switches to human-readable).
- Every request gets a `request_id` (UUIDv7) propagated as `X-Request-ID` and threaded into every downstream log line.
- Every dispatched step propagates `bead_id`, `step_idx`, `loop_count`, `agent`, `mode` as base log attrs.
- Sensitive fields (api keys, prompt bodies, raw tool args) are **never** logged at info level. They appear at `debug` only, and are redacted (`<REDACTED:<len>>`) when the field name matches a denylist.
- Sampling: `debug` logs are not sampled (operator opt-in only); `info` is full-rate; tool-call lines are sampled at 1:50 when a single step emits >1k tool calls (the runlog table retains them all — logs are for ops, runlog is for users).

### 12.2 Health checks

- `GET /api/v1/healthz` returns 200 when the dispatcher loop has ticked within `telemetry.healthz_grace`, otherwise 503 with `{ "stuck_for_ms": N }`.
- `GET /api/v1/orchestrator/status` is the rich version: includes build, schemaVersion, beadsVersion, online, runningCount, worktreeCount, tmuxAvailable, tmuxVersion, dolt snapshot. Use this for the UI; use `/healthz` for k8s-style probes.

### 12.3 Metrics

`GET /api/v1/metrics` (Prometheus text format). Canonical metric set:

| Metric                                    | Type      | Labels                  | Notes                                              |
|-------------------------------------------|-----------|-------------------------|----------------------------------------------------|
| `muster_dispatcher_loop_seconds`          | histogram | —                       | wall time per loop iteration                       |
| `muster_dispatcher_lag_ms`                | gauge     | —                       | ms since last loop tick — same source as /healthz  |
| `muster_step_duration_seconds`            | histogram | provider, mode, status  | duration per step run; status ∈ done/failed/cancelled |
| `muster_step_tokens_total`                | counter   | provider, mode, direction | direction ∈ in/out                                 |
| `muster_quota_used`                       | gauge     | provider, window        | window ∈ today/this_week/this_month                |
| `muster_quota_limit`                      | gauge     | provider, window        |                                                    |
| `muster_capacity_in_use`                  | gauge     | provider                |                                                    |
| `muster_capacity_queued`                  | gauge     | provider                |                                                    |
| `muster_beads_by_column`                  | gauge     | column                  |                                                    |
| `muster_runlog_lines_total`               | counter   | provider, kind          |                                                    |
| `muster_worktrees_active`                 | gauge     | vcs                     | vcs ∈ jj/git                                       |
| `muster_tmux_sessions_active`             | gauge     | —                       |                                                    |
| `muster_ws_clients_connected`             | gauge     | —                       |                                                    |
| `muster_ws_events_emitted_total`          | counter   | type                    |                                                    |
| `muster_api_request_seconds`              | histogram | method, route, status   | excludes /api/v1/stream                            |
| `muster_api_requests_total`               | counter   | method, route, status   |                                                    |
| `muster_dolt_writes_total`                | counter   | table                   |                                                    |
| `muster_dolt_replication_seconds`         | gauge     | direction               | direction ∈ push/pull; -1 when no remote configured |

---

## 13. Security

### 13.1 Posture

- v1 is **localhost-only by default**. The daemon refuses non-loopback binds unless `--allow-external` (or `[server] allow_external = true`) is set, and even then only when an API token is present.
- API keys live in macOS Keychain (`com.muster.<provider-id>`) or libsecret on Linux. The process resolves them on demand, holds them for the duration of one outbound request, and zeroes the byte slice after.
- Worktrees inherit the user's shell environment by design: agents need real PATH + git config + jj config to do their jobs. The daemon does **not** add to that surface (no `--unsafe-perm`, no setuid escalation).
- The UI surfaces a kill-switch (top-bar status badge → click to `POST /api/v1/orchestrator/halt`; running beads continue, no new dispatches).

### 13.2 Threat model (in scope)

| Threat                                       | Mitigation                                                                 |
|----------------------------------------------|----------------------------------------------------------------------------|
| Browser tab from a malicious site hits localhost | CORS allow-list (`http://localhost:*` only in local mode), `X-Frame-Options: DENY`, `Content-Security-Policy` on the embedded UI |
| API key leak via logs                        | redact-by-name denylist, never log raw POST bodies at info level            |
| API key leak via WS frames                   | provider config returned over WS never includes the resolved value of `api_key_ref` |
| Prompt-injection escaping to other beads     | each bead is one tmux session + one worktree; no shared filesystem state    |
| Bead command exfiltrating Keychain           | agent doesn't see the resolved key; outbound calls go through `apiprov`, not the agent process |
| WS replay-attack on reconnect                | `?since=<seq>` only replays events the server still has in its 5-minute buffer; older requests force a full re-fetch |
| Dolt remote tampering                        | out of scope — the Dolt remote is the user's own infra; we ship `bd dolt verify` as a manual check |

### 13.3 Threat model (out of scope)

- Multi-tenant isolation (one user, one box).
- Sandboxing the agent's process tree (agents must run as the user; that's the product).
- Audit-grade tamper-evident history (Dolt gives us cryptographic verifiability if the user opts in, but we don't enforce it).

---

## 14. Migration & install

### 14.1 Bootstrap

`muster init` does:

1. Creates `.muster/` under the cwd.
2. Initialises Dolt (`bd init`) if not already.
3. Detects installed CLI agents (claude, gemini, codex) and seeds their `Provider` rows (`kind = cli`). Probes the opencode SDK linkage and seeds an opencode `Provider` row (`kind = sdk`) if available. Probes for direct-API credentials and seeds those rows (`kind = api`) if found.
4. Writes a starter `constitution.md` with sensible defaults.
5. Prints `Run muster serve to start the daemon`.

`muster serve` starts everything.
`muster gc` cleans up old worktrees (per `worktree_idle_gc`) and squashes runlog older than `runlog_retention`. Safe to run while the daemon is up — it acquires advisory file locks per worktree.
`muster doctor` runs the same checks as `bd doctor` plus the daemon-specific ones (port free, tmux available, Keychain unlocked, schema versions aligned).
`muster version` prints `{ build, schemaVersion, beadsVersion, goVersion }` — same fields surfaced in `/orchestrator/status`.

### 14.2 Schema migrations

The Dolt schema is versioned (`schema_version` table, single row). On `muster serve` startup, if `current < expected`, the daemon refuses to start and prints `Run muster migrate to upgrade from v<N> to v<expected>`.

`muster migrate` runs forward-only migrations from `migrations/<N>_<name>.sql`, each in its own transaction. Rollback is by `bd dolt reset` to the pre-migration commit (Dolt makes this trivial). Down-migrations are not supported.

Attached repos (§8) keep their **own** schema versions. The daemon surfaces a `SCHEMA_MIGRATION_REQUIRED` error to the UI and lets the user opt into `POST /api/v1/repos/{id}/migrate`. Out-of-date repos stay attached but go **read-only** until migration completes.

---

## 15. Open questions

1. **Verify non-Claude provider CLI flags.** Gemini, OpenCode, Codex non-default modes are marked synthesized; need to verify which (if any) of their workflow stages map to real CLI flags. *Tracked as DESIGN.md §7 (deferred decision) and §12 (production checklist).*
2. **MCP server availability check.** Skills can list `MCPServers` that must be pre-registered in the agent's own config. Passive warning vs. active block at dispatch time? Current plan: passive warning in the runlog + a one-line note in the drawer's Steps tab; never block dispatch.
3. **Token estimation pre-dispatch.** We dispatch and *then* learn the cost. A cheap estimate-pass via the API would double tool calls. Current plan: skip estimation; surface running `tokensUsed/tokensBudget` clearly so the user catches overshoots; rely on auto-split to keep them bounded.
4. **Sub-bead linking back to parent.** When a sub-bead finishes, do we auto-merge its diff into the parent's worktree, or keep them separate? Current plan: keep separate; emit `subbead.completed`; merging is manual or a dedicated step in the parent's chain.
5. **Worktree fan-out for Speckit fleet mode.** Speckit's fleet extension spawns several parallel drafts. Current plan: each becomes a sub-bead with its own worktree, sharing the parent's constitution + skills.
6. **API-provider quota tracking.** Best-effort (response headers vary by vendor). Current plan: a per-provider "hard budget you set" knob lives in `Provider.Quota.Limit`; dispatch halts on `QUOTA_EXHAUSTED` regardless of vendor headers.
7. **Routes hot-reload.** `.beads/routes.jsonl` changes. Current plan: inotify watch on the file; on change, atomically replace the in-memory rules and emit `routes.changed` over WS. `SIGHUP` also covers it.
8. **Cross-repo dep status.** `external:repo/bd-100` deps. Current plan: do not poll the foreign repo's Dolt state; rely on the user to `bd hydrate`. The dep graph view greys out unknown external nodes.
9. **Idempotency-Key TTL.** 24h is the current default. May need to be tunable per-endpoint for long-running flows (e.g. `POST /beads/{id}/dispatch` retries during a long agent run).

---

## 16. Build & dependencies

- Go 1.26+.
- `github.com/dolthub/go-mysql-server` (or `bd` linked as a library) for Beads.
- `github.com/coder/websocket` for WS.
- `github.com/go-chi/chi/v5` for routing.
- `github.com/spf13/viper` for config.
- `github.com/zalando/go-keyring` for keychain.
- `github.com/google/uuid` for request IDs + idempotency keys.
- `github.com/prometheus/client_golang` for `/api/v1/metrics`.
- `github.com/stretchr/testify` + `gotest.tools/v3` for tests.
- `github.com/google/go-cmp` for diffing test fixtures.
- Embedded UI via `go:embed ui/*`.

Third-party CLIs are runtime deps, not build deps: `tmux >= 3.2`, `jj >= 0.20`, `git >= 2.40`. The daemon probes all three at startup and reports availability in `/orchestrator/status`.

---

## 17. Testing strategy

Three layers, each with a clear job. CI runs all three on every PR; nightly runs add the smoke pass against a real macOS runner.

### 17.1 Unit (`internal/<pkg>/*_test.go`)

Table-driven, no I/O. Coverage gates: 80% on `core/` (bead state machine, step transitions, prompt assembly), 70% on `disp/`, `wt/`, `quota/`. Below 60% on a touched package fails CI.

Notable areas:
- `core.applyTransition(bead, action)` — every (column × action) cell.
- `core.assemblePrompt(bead, step, constitution)` — golden files in `testdata/prompts/`.
- `disp.elect(candidates, capacity, quota)` — priority + capacity + quota interactions.
- `wt.JJ` and `wt.Git` — against tmpdir repos with seeded commits.
- `quota.Tracker.parseCLIOutput(...)` — against captured stdout fixtures from each adapter.

### 17.2 Integration (`test/integration/*_test.go`)

Real Dolt (embedded), real tmux (if available), fake adapters. Each test gets a tmpdir for `.muster/` and a tmpdir for the worktree source. Tests run serially by default; flagged tests opt into `t.Parallel()`.

Scenarios:
- `bead.lifecycle` — create → schedule → dispatch → step succeed → review → approve. Asserts every emitted WS event in order.
- `bead.requeue_on_token_exhaustion` — fake adapter exhausts budget; bead must end up back in `scheduled` with `Requeued=true` and runlog preserved.
- `bead.auto_split` — adapter emits `<muster:subbead>` markers; verify sub-beads land in Dolt and the parent's `subBeads` populates.
- `bead.loop` — review step fails twice, hits `LoopMax=3`, escalates per policy.
- `dispatcher.capacity` — N beads, M slots, M > slots; verify queue ordering.
- `tmux.restart_recovery` — spawn step → kill daemon → restart → step keeps running, runlog resumes.
- `repos.probe_and_attach` — against seeded `.beads/` directories in embedded and server modes.
- `idempotency.dispatch_retry` — same `Idempotency-Key` returns the original response; different body returns 409.

### 17.3 Contract (`test/contract/*_test.go`)

Locks the UI ↔ backend contract. Each contract test starts the daemon with a known seed, hits an endpoint, and diffs the response against a fixture in `testdata/contracts/`. The UI ships the **same** fixtures as JSON and loads them into `window.MUSTER_DATA.*` in `data.jsx` — so when the contract test passes, the UI's seed data is by-definition shape-compatible with what the server emits.

Adding a field is allowed (the UI must ignore unknowns); removing or renaming a field fails the contract test and forces an `/api/v2` decision.

### 17.4 Smoke

`scripts/smoke.sh` spins up `muster serve` against a throwaway dir, runs the integration matrix, then drives the UI end-to-end via `chromedp` (open board → create bead → dispatch → wait for runlog → approve). One pass on every PR + nightly against the release branch.

---

## 18. Packaging & release

### 18.1 Distribution

- Single static binary per platform: `muster-{darwin,linux}-{amd64,arm64}`. The embedded UI ships inside via `go:embed`.
- macOS builds are signed + notarised; binaries are Gatekeeper-friendly.
- Linux builds are stripped, glibc ≥ 2.31. A musl variant ships behind a build tag for Alpine.
- Homebrew tap: `brew install muster/tap/muster`.
- Curl installer: `curl -fsSL https://muster.dev/install | sh` resolves the right asset from the latest GitHub release and verifies its SHA256.
- Docker is **not** a primary distribution channel for v1 — the daemon needs host-level tmux + jj + git + Keychain. A devcontainer image exists for evaluation only.

### 18.2 Versioning

Semver with the API-stability clause from §4.0. The `build` string surfaced in `/orchestrator/status` is `<semver>-<short-sha>` for release builds, `dev-<sha>[-dirty]` otherwise.

UI and daemon ship from the same monorepo at the same SHA. The daemon refuses a UI whose `__muster_build` string (injected into `Muster.html` by `go:embed`) doesn't match its own, except in dev mode (`--dev` flag).

### 18.3 Release cadence

- Nightly builds from `main` to a `next` channel.
- Beta releases on a `beta` channel; bug-only churn.
- Stable releases monthly. Each carries a changelog generated by the `changelog-gen` formula (§9a) against the previous tag.

---

## 19. Operational runbook

Minimum surface a competent SRE needs to keep `muster` healthy.

### 19.1 Startup checks

On boot, `muster serve` logs (at info):

```
startup port=7766 build=v1.2.3-abc123 schemaVersion=4 beadsVersion=0.9.1 tmux=3.4 jj=0.21 git=2.43
providers cli=3 sdk=1 api=1 detected=claude,gemini,codex,opencode authStatus=ok
repos attached=4 mode=embedded:3,server:1 schemaDrift=0
dispatcher loopInterval=tickless ready=true
```

Absence of any of these lines is a startup failure; the process exits non-zero rather than half-starting.

### 19.2 Diagnostic commands

| Symptom                                | Command                                                  | What to look for                                  |
|----------------------------------------|----------------------------------------------------------|---------------------------------------------------|
| Beads stuck in `scheduled`             | `curl /api/v1/orchestrator/capacity`                     | every provider at limit or `auth.status != logged-in` |
| Drawer shows old data after action     | `curl /api/v1/orchestrator/status` then check WS clients | `muster_ws_clients_connected==0` → UI lost the WS, ask user to refresh |
| `/healthz` returning 503               | `tail -f stderr \| jq 'select(.level=="warn")'`          | dispatcher loop lag; long-running `applyEvent`     |
| Worktree disk filling up               | `muster gc --dry-run`                                   | candidates list; run `muster gc` to reclaim       |
| Runlog table large                     | `bd sql "SELECT count(*) FROM runlog"`                   | run `muster gc` (squashes runlog past retention)  |
| Tmux sessions orphaned                 | `tmux ls \| grep ^muster/`                               | reconcile via `muster doctor --reconcile-tmux`    |
| Quota tracker drift                    | `curl /api/v1/providers/<id>/quota`                      | compare to vendor dashboard; reset via `bd dolt sql 'UPDATE provider_quota …'` (rare) |

### 19.3 Recovery procedures

- **Daemon crashed mid-step.** On restart, `tmux.Manager.List()` enumerates surviving sessions; matched beads resume streaming. Unmatched sessions are killed after the grace period. No manual action needed.
- **Dolt write conflict.** Should be impossible in single-writer embedded mode, but if reported, `bd dolt status` shows the conflict; resolve with `bd dolt reset --hard HEAD` (loses uncommitted runlog — acceptable).
- **All providers exhausted.** `dispatcher.halted reason=no_capacity` event emitted; UI banner. Users should set higher budgets or wait for the quota window to roll over.
- **Schema drift on attached repo.** Surfaced as `SCHEMA_MIGRATION_REQUIRED`; user clicks the warning in /repos to call `POST /api/v1/repos/{id}/migrate`.
- **WS replay buffer overflow.** Clients passing `?since=<seq>` older than `ws_buffer_seconds` get `410 Gone` and reload — by design.

### 19.4 Backups

- `bd dolt push` (or `POST /api/v1/dolt/push`) is the canonical backup mechanism — it pushes the full bead history to a Dolt remote (DoltHub, self-hosted, S3-backed).
- Worktrees are **not** backed up by Muster; they're transient and reproducible from the bead's runlog + the source repo.
- The `[server] api_token` and any in-Keychain API keys are **not** backed up. Operators should manage these separately (1Password, sealed-secrets, etc.).

---

## 20. Milestones

Order-of-implementation for getting from skeleton to shippable. Each milestone is a self-contained PR series gated by the test layer named in parentheses.

1. **M0 — Skeleton.** (unit) Single binary serves the embedded UI, `/api/v1/beads` is in-memory, WS pushes optimistic mutations.
2. **M1 — Beads-backed.** (integration) Replace in-memory store with Dolt. `bd init` integration. Persistence across restarts. Schema-migration plumbing.
3. **M2 — One CLI adapter.** (integration) Claude Code adapter end-to-end: detect, login flow, plan mode, agent mode, runlog streaming via `tmux pipe-pane`.
4. **M3 — Worktrees.** (integration) Per-bead VCS (jj + git), `wt.Backend` interface, diff exposure, file list.
5. **M4 — Dispatcher.** (integration) Real scheduler, capacity gating, quota tracking from CLI output, idempotency on `/dispatch`.
6. **M5 — Multi-provider.** (contract) Gemini + Codex CLI adapters. OpenCode SDK adapter. Direct-API adapter. Verify the synthesized-vs-native split in `data.jsx`.
7. **M6 — Skills & constitution.** (unit) Skill registry, prompt assembly, constitution merge. Memories CRUD.
8. **M7 — Repos & routing.** (contract) `/repos` CRUD with the probe flow, routes hot-reload, hydration. Replace `mockProbe` in the UI.
9. **M8 — Sub-beads & policy.** (integration) Auto-split, escalation, loop control, gates.
10. **M9 — Observability + GC.** (smoke) `/healthz`, `/metrics`, runlog compaction, worktree GC, audit log.
11. **M10 — Polish + harden.** (smoke) Reduced-motion CSS, focus restoration, error boundary, `data-testid` coverage, signed releases.

---

## 21. v2 roadmap (post-v1)

Intentionally **not** in v1. Listed here so v1 decisions don't paint v2 into corners.

- **Multi-user, multi-machine.** Real OIDC + per-user beads + presence in the UI.
- **Hosted variant.** `muster.dev` SaaS: same daemon, S3 + Postgres for worktrees + history, sandboxed agent runtime.
- **Realtime co-editing of beads.** Y.js or Automerge on the drawer's Overview tab.
- **Auto-merge sub-beads.** Once the verifier formula matures.
- **Audit-grade tamper-evidence.** Sign every history event with a per-user key, store in a Dolt branch the user can't rewrite.
- **Mobile composing.** New-bead from phone, with voice-to-text on title + desc.
- **GPU-aware capacity.** For local providers (Ollama), gate `parallel_max` on `nvidia-smi` / `metal-stats` so we don't OOM the host.
