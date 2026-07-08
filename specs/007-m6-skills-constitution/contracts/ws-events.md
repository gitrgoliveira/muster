# Contract: M6 WebSocket events (additive)

**Feature**: `specs/007-m6-skills-constitution` | **Phase**: 1 | Stream: `GET /api/v1/stream`

New event types added to `internal/ws/event.go` const block (`:7-53`). Clients `default:` unknown types to no-op (`spec.md:299`), so additions are non-breaking (Principle V, FR-027). No existing event type/shape changes (SC-008).

---

## `constitution.changed`  *(new — FR-007)*

Emitted after a successful `PUT /api/v1/constitution`.

```json
{ "type": "constitution.changed", "version": 3 }
```
- Envelope: existing `ws.Frame`. Carries the new monotonic `version` (optional additive `Frame.Version *int omitempty`, or a nested payload — impl picks the minimal additive form).
- Broadcast via the existing `Publisher`/`hub.Broadcast` path (blocking ingress — not dropped).

---

## `runlog.warning`  *(new — FR-020, FR-021)*

Emitted when assembly skips an unresolvable `skill:<id>` or finds a skill's `MCPServers` entry absent from the agent's own MCP config. **Never blocks dispatch.**

```json
{ "type": "runlog.warning", "beadID": "muster-ep0", "stepIdx": 0,
  "reason": "skill \"typo-skill\" not found; skipped" }
```
- Distinct from `runlog.line` deliberately: `runlog.line` is best-effort **dropped** under backpressure (`hub.go:117`); a warning must not be silently lost, so it uses a non-dropped ingress path (research §5).
- Reuses the existing `Frame.Reason` field (currently used by `run.failed`) for the message; `beadID`/`stepIdx` locate it.

> Impl note: an equally-additive alternative is a `Kind`/`Level *string omitempty` field on the existing `runlog.line` frame. Either is additive; the distinct-event form is preferred so warnings survive backpressure. Pin one in tasks.

---

## Unchanged (for reference — must stay green, SC-008)

Existing event types are untouched: `hello`, `bead.created/updated/moved/deleted`, `comment.added`, `pong`, `runlog.line`, `tmux.session.opened/closed`, `dispatch.queued/admitted`, `step.advanced/loopedback`, `worktree.finalized/pushed/removed`, `run.quota`, `run.failed`.
