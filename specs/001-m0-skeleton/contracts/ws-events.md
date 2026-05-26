# WebSocket Event Contract: M0 — Skeleton

**Endpoint**: `ws://127.0.0.1:7766/api/v1/stream`
**Frame type**: Text (UTF-8 JSON)
**Library**: `github.com/coder/websocket`

This contract matches spec.md §FR-012, §FR-013, §FR-014, and §US7. Seven event types are
defined: one server-initiated greeting (`hello`), four mutation broadcasts (`bead.created`,
`bead.updated`, `bead.moved`, `bead.deleted`), one comment append (`comment.added`), and one
ping-pong response (`pong`).

---

## Connection Lifecycle

1. Client sends `GET /api/v1/stream` with `Upgrade: websocket` and `Sec-WebSocket-Key`.
2. Server upgrades via `websocket.Accept(w, r, nil)` (no subprotocol negotiated).
3. Server creates a `Client` wrapping the connection, with a buffered `send chan ws.Frame` (capacity 16); the `writePump` goroutine serialises each Frame to JSON and writes it to the connection.
4. Server registers the client into the Hub via `hub.register <- client`.
5. **Within 1 second of registration**, the server sends a `hello` frame (per spec FR-013).
6. Two goroutines run for the client:
   - `writePump`: drains `client.send` and writes to the WS connection.
   - `readPump`: reads application frames; routes `{"type":"ping"}` frames to a `pong` response and ignores any other application-layer frames at WARN.
7. Server holds the HTTP handler open until either pump exits.
8. On exit, the client is unregistered (`hub.unregister <- client`) and the connection is closed.

---

## Frame Shapes

All server-→-client frames carry a `type` field. The remaining fields depend on `type`.

### `hello` — server → client, sent once on connect

Spec FR-013: must arrive within 1s of connection registration.

```json
{
  "type": "hello",
  "build": "dev",
  "schemaVersion": 1,
  "serverTime": "2026-05-22T17:42:11Z",
  "beadsVersion": "0.9.1"
}
```

### `bead.created` — server → client, broadcast after `POST /api/v1/beads` 2xx

```json
{
  "type": "bead.created",
  "bead": { /* full Bead */ }
}
```

### `bead.updated` — server → client, broadcast after `PATCH /beads/{id}`, `POST /dispatch` 2xx

```json
{
  "type": "bead.updated",
  "bead": { /* full Bead */ }
}
```

### `bead.moved` — server → client, broadcast after `POST /beads/{id}/move` 2xx

```json
{
  "type": "bead.moved",
  "id": "bd-a1f2",
  "fromColumn": "scheduled",
  "toColumn": "running",
  "beforeID": "bd-c411",
  "bead": { /* full Bead — convenience so clients don't refetch */ }
}
```

`beforeID` is included only if supplied by the request; otherwise omitted.

### `bead.deleted` — server → client, reserved

Not emitted in M0. Reserved for forward compatibility.

```json
{ "type": "bead.deleted", "id": "bd-a1f2" }
```

### `comment.added` — server → client, broadcast after `POST /beads/{id}/comments` 201

```json
{
  "type": "comment.added",
  "id": "bd-a1f2",
  "event": {
    "at": "2026-05-22T17:42:11Z",
    "kind": "comment",
    "actor": "claude",
    "note": "requested deterministic clock test"
  },
  "bead": { /* full Bead with updated history + comments count */ }
}
```

### `pong` — server → client, response to a `ping` frame

```json
{ "type": "pong", "at": "2026-05-22T17:42:11Z" }
```

---

## Client → Server Frames

### `ping` — client → server, server responds with `pong`

```json
{ "type": "ping" }
```

The server responds with a `pong` frame containing the server's current `serverTime`. Per
spec FR-014.

Any other application-layer frame from the client is logged at WARN and discarded. Control
frames (ping/pong/close at the WebSocket protocol level, distinct from the application
ping/pong above) are handled by the `coder/websocket` library automatically.

---

## Concurrency Model

The Hub runs a single `Run()` goroutine that owns the `clients` map. All map mutations happen
inside this loop:

```text
Hub.Run() loop:
  select {
    case c := <-register:    add to clients; spawn writePump + readPump for c; send hello to c
    case c := <-unregister:  remove from clients, close c.send
    case msg := <-broadcast: forEach client: try-send to c.send (drop on full)
  }
```

Each `Client` has its own `writePump` goroutine that drains `c.send`. `coder/websocket`
prohibits concurrent writes to the same connection — serialising writes through the pump is
essential.

### Slow-client policy

If a client's `send` channel is full (capacity 16) when the hub tries to fan out, the hub drops
that message **for that client only** and logs a WARN. After 3 consecutive drops within 10 s,
the hub unregisters the slow client.

---

## Delivery Guarantees (M0)

- **At-most-once delivery** per client. No retransmission.
- **Per-hub ordering preserved** (single goroutine serialises broadcasts).
- **No `seq` field**, no replay buffer, no `?since=` parameter. (All deferred to M1.)
- **No subscription filtering** — every connected client receives every broadcast.
- **The `hello` frame is the only event guaranteed for newly connected clients.** Any
  mutations occurring before `hello` is sent are not retroactively delivered.

### Reconnection pattern (client side)

```text
1. Open WS connection.
2. Receive `hello`.
3. Fetch full state via GET /api/v1/beads (snapshot).
4. Apply incoming WS events as deltas.
5. On disconnect, GOTO 1.
```

---

## Race / Edge Cases

| Case | Behaviour |
|---|---|
| Client connects mid-mutation | Hub registers after the mutation's broadcast may have started. Client sees only `hello` + subsequent events; missed event recovered via re-fetch on next reload. |
| Client disconnects mid-broadcast | Hub's send-to-channel is non-blocking with drop policy; broadcast continues to other clients without delay. |
| Multiple mutations within same RTT | All emit events; client receives them in commit order. |
| Server shutdown | Hub closes all client `send` channels; pumps exit; clients see WS close frame with status 1001 (going away). |
| `panic` in handler | chi recoverer catches; 500 response; **no WS event emitted** for the failed mutation. |
| Client sends oversized frame | `coder/websocket` read limit set to **1 MiB** (matches REST body cap). Exceeded → library closes connection with status 1009 (message too big). |
| Client sends binary frame | readPump logs WARN and continues (frame ignored). |

---

## Smoke test

```bash
# Terminal A — start server
./muster serve

# Terminal B — open WS stream (websocat installed via `brew install websocat`)
websocat ws://127.0.0.1:7766/api/v1/stream
# → within 1s: {"type":"hello","build":"dev","schemaVersion":1,...}

# Send a ping
echo '{"type":"ping"}' | websocat ws://127.0.0.1:7766/api/v1/stream
# → {"type":"hello",...} then {"type":"pong","at":"..."}

# Terminal C — mutate
curl -X POST http://127.0.0.1:7766/api/v1/beads/bd-a1f2/move \
  -H 'Content-Type: application/json' \
  -d '{"toColumn":"review"}'

# Terminal B should print:
# {"type":"bead.moved","id":"bd-a1f2","fromColumn":"running","toColumn":"review","bead":{...}}
```
