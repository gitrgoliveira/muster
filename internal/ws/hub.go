package ws

import (
	"log/slog"
	"sync/atomic"
	"time"
)

const (
	sendBufSize = 16
	maxDrops    = 3
	dropWindow  = 10 * time.Second
)

// Hub maintains the set of active clients and fans out broadcast frames.
type Hub struct {
	register   chan *Client
	unregister chan *Client
	broadcast  chan Frame

	clients      map[*Client]bool
	beadsVersion string

	// dropped counts frames dropped because the ingress buffer was full (see
	// Broadcast). Atomic: incremented from arbitrary producer goroutines.
	dropped atomic.Uint64
}

// NewHub constructs a Hub. beadsVersion is injected from seed data and
// included in the hello frame sent to each connecting client.
func NewHub(beadsVersion string) *Hub {
	return &Hub{
		register:     make(chan *Client),
		unregister:   make(chan *Client),
		broadcast:    make(chan Frame, 256),
		clients:      make(map[*Client]bool),
		beadsVersion: beadsVersion,
	}
}

// Run is the hub's main event loop. It must be started in a goroutine.
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.clients[c] = true
			go h.sendHello(c)

		case c := <-h.unregister:
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}

		case f := <-h.broadcast:
			for c := range h.clients {
				select {
				case c.send <- f:
					// Delivered — the client is keeping up. Clear any drop streak
					// so a brief earlier stall (including a burst of high-volume
					// runlog) doesn't accumulate toward eviction across successful
					// sends. Only CONSECUTIVE drops (below) count.
					c.drops = 0
				default:
					// Buffer full — frame dropped for this client. This counts for
					// ALL frame types, including runlog.line: a client that stops
					// reading during a long, runlog-only run must still eventually
					// be evicted, or its socket + writePump goroutine + full send
					// buffer leak indefinitely. But because the counter resets on
					// every successful send above, a client that's merely slow-
					// but-reading (draining between bursts) is NOT evicted by
					// runlog volume alone — only a genuinely stuck one, whose
					// buffer stays full across maxDrops consecutive sends within
					// dropWindow, is. The window guards against evicting on drops
					// spread across unrelated slow moments.
					now := time.Now()
					if now.Sub(c.lastDrop) > dropWindow {
						c.drops = 0
					}
					c.drops++
					c.lastDrop = now
					if c.drops >= maxDrops {
						slog.Warn("ws: client too slow, unregistering",
							"drops", c.drops, "window", dropWindow)
						delete(h.clients, c)
						close(c.send)
					}
				}
			}
		}
	}
}

// Broadcast enqueues f for fan-out to all connected clients.
//
// Frame handling is deliberately split by type:
//
//   - runlog.line is high-volume, best-effort pane output produced by the tmux
//     transport-reader goroutine. A blocking send there could stall the reader
//     and, via the tmux FIFO, apply backpressure to the agent itself. So these
//     are enqueued non-blocking: if the ingress buffer is full the frame is
//     dropped (and counted). Clients recover scrollback via capture-pane.
//   - every other type is a low-volume lifecycle/state event (bead.updated,
//     tmux.session.closed, …) where a dropped frame would leave clients stale.
//     These use a blocking send so they are never dropped at the *ingress*
//     buffer (unlike runlog). Because runlog frames never queue past the
//     buffer, the buffer drains continuously and a lifecycle send almost always
//     finds room immediately — and its producers are never the transport
//     reader, so a brief block can't stall the agent.
//
// This ingress guarantee is NOT end-to-end delivery: Run's per-client fan-out
// still drops to any individual client whose send buffer is full (select
// default), and unregisters a persistently-slow client. So a slow client can
// still miss lifecycle frames and must recover via re-fetch; the blocking send
// only removes the *shared* ingress buffer as a drop point for those frames.
func (h *Hub) Broadcast(f Frame) {
	if f.Type == EventRunlogLine {
		select {
		case h.broadcast <- f:
		default:
			if n := h.dropped.Add(1); n == 1 || n%256 == 0 {
				slog.Warn("ws: broadcast ingress buffer full, dropping runlog frame(s)", "totalDropped", n)
			}
		}
		return
	}
	h.broadcast <- f
}

// DroppedFrames returns the number of frames dropped by Broadcast because the
// ingress buffer was full. Exposed for observability (e.g. status/metrics).
func (h *Hub) DroppedFrames() uint64 { return h.dropped.Load() }

func (h *Hub) sendHello(c *Client) {
	hello := Frame{
		Type:          EventHello,
		Build:         "dev",
		SchemaVersion: 1,
		ServerTime:    time.Now().UTC().Format(time.RFC3339),
		BeadsVersion:  h.beadsVersion,
	}
	select {
	case c.send <- hello:
	case <-time.After(1 * time.Second):
		slog.Warn("ws: hello not delivered within 1s, unregistering client")
		h.unregister <- c
	}
}
