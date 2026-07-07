# Contract: M6 REST API (additive)

**Feature**: `specs/007-m6-skills-constitution` | **Phase**: 1 | Base path: `/api/v1`

All routes below are **new** (verified absent from `internal/api/router.go:45-76`). No existing route/shape/error changes (Principle V, FR-027, SC-008). Errors use the existing envelope `{"error":{"code","message","requestID"}}` (`internal/api/render/errors.go`). `PUT /constitution` and `POST /skills` carry `middleware.BodyLimit` (1 MiB).

---

## Constitution (US2)

### `GET /api/v1/constitution`
→ `200` `{ "markdown": string, "version": int, "updatedAt": RFC3339 }`
- Fresh install (no file): `{ "markdown": "", "version": 0, "updatedAt": null }` — **not** 404 (AS1).

### `PUT /api/v1/constitution`
Body: `{ "markdown": string }`
- `200` `{ "markdown", "version", "updatedAt" }` — version incremented monotonically; `.muster/constitution.md` overwritten; emits `constitution.changed` (WS).
- `400 INVALID_REQUEST` — malformed JSON / unknown fields / oversize (body-limit) (AS5).
- Effect applies to the **next** dispatch only; a running step is unaffected (AS/FR-009).

---

## Skills (US3)

### `GET /api/v1/skills`
→ `200` `{ "skills": [ Skill ] }` where `Skill = {id,name,desc,category,icon,promptStub,mcpServers[]}`.
- Always includes the full built-in catalog even with zero imports (AS1, SC-003).

### `GET /api/v1/skills/categories`
→ `200` `{ "categories": [string] }` — distinct categories across built-in + imported (AS2).

### `POST /api/v1/skills`
Body: `{ "url": string }` (body-limit applies)
- `201` `{ Skill }` — imported skill persisted under `.muster/skills/<id>.md`, visible in subsequent `GET` (incl. after restart) (AS3, SC-003).
- `400 INVALID_REQUEST` — malformed/unreachable/oversize URL or bad skill doc; **no** partial registration (AS4, FR-017).
- `409 SKILL_ID_CONFLICT` — `id` collides with a **built-in** (rejected, not shadowed) (research §4). Colliding an existing *imported* id ⇒ `200`/`201` upsert.
- URL fetch: `https` only (`http` allowed for loopback), 10 s timeout, 1 MiB cap (research §5).

### `DELETE /api/v1/skills/{id}`
- `204` — previously-imported skill removed from `.muster/skills/` (AS6).
- `403 SKILL_READONLY` — `id` is a built-in (never a silent no-op, never 404-as-if-absent) (AS5, FR-015).
- `404 NOT_FOUND` — unknown id (neither built-in nor imported).

---

## Memories (US5) — thin wrapper over `bd`

### `GET /api/v1/memories?q=<term>`
→ `200` `{ "memories": [ {key,value} ] }` — shells `bd memories [term]`. No `q` ⇒ full list (AS3).

### `POST /api/v1/memories`
Body: `{ "key"?: string, "value": string }`
- `200/201` `{ key, value }` — upsert via `bd remember [--key K] "value"`; no key ⇒ auto-derived key returned (AS1); existing key ⇒ update-in-place, not duplicated (AS2).

### `DELETE /api/v1/memories/{key}`
- `204` — removed via `bd forget <key>` (AS4).
- `404 NOT_FOUND` — non-existent key (clear not-found, not a false success) (AS4).

### `POST /api/v1/memories/prime`
Body: `{ "beadID": string }`
- `200` `{ "primed": int }` — snapshots current memories against the bead; its **next** dispatch's assembled prompt includes a "Primed memories" section (AS5, FR-024).

**Memories error rule (FR-025)**: any underlying `bd` failure (missing binary, non-zero exit, unparseable output) ⇒ typed error surfaced to the client (e.g. `500 BD_UNAVAILABLE` / `502`), **never** an empty-list masquerade (AS6).

---

## New error codes

| Code | HTTP | Meaning |
|---|---|---|
| `SKILL_READONLY` | 403 | delete/modify a built-in skill |
| `SKILL_ID_CONFLICT` | 409 | import id collides a built-in |
| `SKILL_INVALID_ID` | 400 | skill id fails validation (empty, separators, `.`/`..` traversal) |
| `BD_UNAVAILABLE` | 500/502 | `bd` invocation failed on a `/memories*` route |

(Added to `internal/api/render/errors.go`; existing codes unchanged.)
