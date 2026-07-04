# Contract — WebSocket Events (M4)

**Purely additive** new `EventType` constants in `internal/ws/event.go`, following the existing dotted-namespace convention (`bead.*`, `tmux.session.*`, `runlog.line`). No existing event type/shape changes (Constitution V, FR-024a). All ride the existing `Envelope` (`{ "type", ... }`) on the existing `/api/v1/stream` hub; low-volume lifecycle events use the reliable (non-drop) path, unlike `runlog.line`.

| New EventType | Emitted when | Payload (additive to Envelope) |
|---|---|---|
| `dispatch.queued` | a dispatch is accepted but at capacity (waiting) | `beadID`, `waitingPos` (0-based index in the FIFO at emit time; `Snapshot().waiting` preserves FIFO order) |
| `dispatch.admitted` | a waiting dispatch is admitted (slot freed) and its agent launches | `beadID`, `stepIdx` |
| `step.advanced` | the step pointer advances +1 and the next step's run starts | `beadID`, `stepIdx`, `chainLen` |
| `step.loopedback` | the step pointer moves to an earlier index and that step's run starts | `beadID`, `stepIdx`, `chainLen` |
| `worktree.finalized` | Finalize commits (or reports no-op) | `beadID`, `committed` (bool) |
| `worktree.pushed` | Push succeeds | `beadID`, `branch`, `remote` |
| `worktree.removed` | Remove tears down the worktree | `beadID` |
| `run.quota` | quota captured at run end (best-effort) | `beadID`, `quota` `{ known, inputTokens, outputTokens, costUSD }` |

Notes:
- `run.quota` with `known:false` is still emitted (advisory unknown) so clients can distinguish "captured unknown" from "not yet finished."
- Existing `tmux.session.opened/closed` and `runlog.line` continue to fire exactly as in M2 for each step's agent run — a multi-step chain simply produces one such lifecycle per step.
- Field naming follows the existing envelope JSON tags; `beadID` mirrors the existing `bead`/`id` usage in `event.go`.
