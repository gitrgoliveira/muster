# Contract Quality Checklist: M0 — Skeleton

**Purpose**: Validate that `contracts/rest-api.md` and `contracts/ws-events.md` are complete, 
unambiguous, and consistent with `spec.md` and `data-model.md`.
**Created**: 2026-05-22
**Feature**: [spec.md](../spec.md)

> This checklist tests the **contract documents themselves**. It does not test handler code.

## REST Contract Completeness

- [ ] CHK001 Does every endpoint in the spec FRs (FR-004 through FR-016) appear in `contracts/rest-api.md` with a request shape, response shape, and error matrix? [Completeness, contracts/rest-api.md]
- [ ] CHK002 Is the response body for `GET /api/v1/beads` specified to **always** include `items`, `nextCursor`, `total` — never omit any of them, even when empty? [Completeness, contracts/rest-api.md §GET /beads]
- [ ] CHK003 Are the response status codes documented for every endpoint including all 4xx variants (400, 404, 405) and the 500 path? [Completeness, contracts/rest-api.md §Error code matrix]
- [ ] CHK004 Is `Content-Type: application/json; charset=utf-8` documented as a normative response header — or is it informative? [Clarity, contracts/rest-api.md §Cross-cutting]
- [ ] CHK005 Are `Location` headers documented for all 201-returning endpoints (currently only POST /beads)? [Completeness, contracts/rest-api.md §POST /beads]
- [ ] CHK006 Is the empty-PATCH-body behaviour ("200 OK, bead unchanged, WS event still fires") consistent with the spec's edge case ("PATCH sends an empty body → 400 INVALID_REQUEST")? **Known conflict — pick one.** [Conflict, contracts/rest-api.md vs spec.md §Edge Cases]
- [ ] CHK007 Is the same-column move behaviour ("200 OK, no-op; WS event fires") consistent with the spec, which does not address this case? [Gap, contracts/rest-api.md §POST /move]
- [ ] CHK008 Are the query parameters for `GET /beads` exhaustively listed (currently only `column`), and is the behaviour for unrecognised query params specified (ignore vs 400)? [Completeness, contracts/rest-api.md §GET /beads]
- [ ] CHK009 Is the dispatch endpoint's behaviour fully specified — does it transition column, append a history event, append a step, or all three? **Conflicting interpretations between spec and contract.** [Conflict, contracts/rest-api.md §POST /dispatch vs spec.md §US8/§FR-010]
- [ ] CHK010 Is the comments endpoint's return status pinned (`200` per contract vs `201` per spec US9)? [Conflict, contracts/rest-api.md §POST /comments vs spec.md §US9]

## REST Contract Clarity

- [ ] CHK011 Is the meaning of "field present" in PATCH semantics defined unambiguously for slice fields (pointer-to-slice = nil/[] distinguishable)? [Clarity, contracts/rest-api.md §PATCH]
- [ ] CHK012 Is the difference between `INVALID_REQUEST` (400) and `NOT_FOUND` (404) drawn clearly for malformed-but-route-matching paths like `GET /beads/notabeadid`? [Clarity, contracts/rest-api.md §Error code matrix]
- [ ] CHK013 Is `title.trim().length === 0 → 400` unambiguously specified (i.e., whitespace-only is rejected)? [Clarity, contracts/rest-api.md §POST /beads]
- [ ] CHK014 Is the 255-char title limit a hard requirement, a soft limit, or a UTF-8-bytes vs runes question? [Clarity, contracts/rest-api.md §POST /beads]
- [ ] CHK015 Is "ID collision exhausted → 500" specified with the retry count (3) so consumers know the retry behaviour? Currently the contract mentions `>3 collisions` but the data model and research mention `up to 3` and "one retry loop, max 3 attempts" — pick a consistent number. [Conflict, contracts/rest-api.md vs research.md Decision 4]
- [ ] CHK016 Is the meaning of `nextCursor: null` documented (never a non-null value in M0, regardless of `total`)? [Clarity, contracts/rest-api.md §GET /beads]
- [ ] CHK017 Are validation order and short-circuit rules defined? E.g., if both `title` is missing AND `column` is invalid, which error is returned first? [Clarity, Gap]

## REST Contract Consistency

- [ ] CHK018 Does the error code matrix in `contracts/rest-api.md` include every code referenced by handler-section error tables? Cross-check `BEAD_NOT_FOUND`, `INVALID_REQUEST`, `METHOD_NOT_ALLOWED`, `NOT_FOUND`, `INTERNAL_ERROR`. [Consistency, contracts/rest-api.md §Error code matrix]
- [ ] CHK019 Does every "Required" field documented per endpoint match the DTO struct definition in `data-model.md §3`? [Consistency, contracts/rest-api.md vs data-model.md §3]
- [ ] CHK020 Does every default-value documented per endpoint match the defaults table in `data-model.md §6`? [Consistency, contracts/rest-api.md §POST /beads vs data-model.md §6]
- [ ] CHK021 Do the example JSON bodies in the contract use **all** required fields exactly once, with correct JSON tag names matching `data-model.md`? E.g., `desc` (not `description`), `tokensBudget` (not `tokens_budget`). [Consistency, contracts/rest-api.md examples vs data-model.md]
- [ ] CHK022 Does the response example for `POST /beads` show every field that a real response will include (including derived fields like `estimate`, `assignee`, `comments`, `lastActivity`)? [Consistency, contracts/rest-api.md vs data-model.md §1, §2]

## WS Contract Completeness

- [ ] CHK023 Does `contracts/ws-events.md` document **every** event type referenced in `spec.md §FR-012` (`hello`, `bead.created`, `bead.updated`, `bead.moved`, `bead.deleted`, `comment.added`, `pong`)? **Currently only 3 of 7 are documented.** [Completeness, contracts/ws-events.md vs spec.md §FR-012]
- [ ] CHK024 Is the `hello` event payload shape specified (per spec US7: `{type, build, schemaVersion, ...}`)? [Gap, spec.md §US7]
- [ ] CHK025 Is the ping/pong protocol specified (client `{"type":"ping"}` → server `{"type":"pong","at":"..."}`), and reconciled with the current contract's "server ignores all application frames"? [Conflict, contracts/ws-events.md vs spec.md §FR-014]
- [ ] CHK026 Are the WS close codes documented for normal disconnect, server shutdown (1001), abnormal close (1006), and slow-client unregister? [Completeness, contracts/ws-events.md §Race / Edge Cases]
- [ ] CHK027 Is the maximum frame size documented? [Gap]
- [ ] CHK028 Is the maximum number of concurrent WS clients supported by M0 documented? [Gap]
- [ ] CHK029 Is the connection-acceptance behaviour specified for Origin / WebSocket subprotocol negotiation (currently "no subprotocol negotiated" — is that normative)? [Clarity, contracts/ws-events.md §Connection Lifecycle]

## WS Contract Clarity

- [ ] CHK030 Is "at-most-once delivery per client" qualified — does it mean per-event, per-mutation, or per-connection? [Clarity, contracts/ws-events.md §Delivery Guarantees]
- [ ] CHK031 Is the slow-client policy (`send` buffer = 16, drop on full, unregister after 3 drops in 10 s) reconciled with any spec requirement, or is it a contract-only choice? [Clarity, contracts/ws-events.md §Concurrency Model]
- [ ] CHK032 Is "within a single hub instance ordering IS preserved" sufficient to give consumers an ordering guarantee, or does the spec need a normative "per-bead causal order" statement? [Clarity, contracts/ws-events.md §Delivery Guarantees]
- [ ] CHK033 Is the "events emitted after the store mutation commits and after the HTTP response is being written" guarantee testable (i.e., can a client observe a mutation via WS *before* the REST response returns)? [Clarity, contracts/ws-events.md §Reconnection pattern]

## WS Contract Consistency

- [ ] CHK034 Does every event type emitted in `contracts/rest-api.md` (Side effects sections) appear in `contracts/ws-events.md`'s event type table? Cross-check that `dispatch` emits `bead.updated` per contract — does WS doc list this trigger? [Consistency, rest-api.md vs ws-events.md]
- [ ] CHK035 Does the `payload: Bead` field of WS events reference the same `Bead` shape as `data-model.md §2.1`? [Consistency, contracts/ws-events.md vs data-model.md]
- [ ] CHK036 Are the side-effects ordering guarantees consistent between REST contract ("emits bead.updated") and WS contract ("commit then write then emit")? [Consistency]

## Data Model Quality

- [ ] CHK037 Does `data-model.md §1` enumerate every enum value used in any DTO or domain type? Spot-check `Estimate` — is it ever set in a DTO, or only derived? [Completeness, data-model.md §1]
- [ ] CHK038 Are the "Field rules" in `data-model.md §2.1` exhaustive — covers required, optional, derived, default? Cross-check `Repo` (currently has default `"main"` but no required/optional declaration). [Completeness, data-model.md §2.1]
- [ ] CHK039 Is every "never null" slice field listed (`Labels`, `Skills`, `Blocks`, `BlockedBy`, `Steps`, `SubBeads`, `History`, `Acceptance`, `Log`, `Files`)? Any missing — e.g., `Gates`? [Completeness, data-model.md §2.1]
- [ ] CHK040 Are the timestamp formats (`createdAt`, `openedAt`, `lastActivity`, `closedAt`) specified as RFC3339, or do they remain the prototype's calendar-ish strings ("Mon 09:14")? [Conflict, data-model.md §2.1 vs prototype/data.jsx]
- [ ] CHK041 Is the `History` event's `at` field format pinned (RFC3339 in new events vs prototype's "Mon 08:55" strings in seed data)? [Conflict, data-model.md §2.3 vs prototype/data.jsx]
- [ ] CHK042 Is the `NowPlaying.Kind` enum value set declared (currently free-form string in data-model)? [Clarity, data-model.md §2.5]
- [ ] CHK043 Is the `LogEntry.Kind` enum value set declared (currently free-form string)? [Clarity, data-model.md §2.5]
- [ ] CHK044 Is `FileChange.Status` an enum (`A|M|D`) or free-form string? [Clarity, data-model.md §2.5]

## Error & Edge Case Coverage

- [ ] CHK045 Is the error-matrix row for every endpoint cross-checkable against the corresponding "Error cases" table? [Coverage, contracts/rest-api.md]
- [ ] CHK046 Are the edge cases in `data-model.md §8` reflected in `contracts/rest-api.md` (e.g., empty PATCH body, null vs absent labels, ID collision)? [Consistency]
- [ ] CHK047 Are the WS race/edge cases in `contracts/ws-events.md §Race / Edge Cases` reflected in `data-model.md` so the implementation has a single source of truth? [Consistency]
- [ ] CHK048 Are server-shutdown semantics specified end-to-end (REST drain, WS close, store flush — though M0 has no persistence so no flush)? [Coverage, Gap]

## Traceability

- [ ] CHK049 Does every contract section reference back to the FR it satisfies? Currently the contract is structurally complete but not annotated with FR IDs. [Traceability]
- [ ] CHK050 Is there a single normative table mapping FR → endpoint → contract section → test, so any FR change surfaces all downstream impact? [Traceability]
