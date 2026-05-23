package ws

import (
	"log/slog"
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
				default:
					// Channel full — record drop.
					c.drops++
					c.lastDrop = time.Now()
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

// Broadcast sends f to all connected clients asynchronously.
func (h *Hub) Broadcast(f Frame) {
	h.broadcast <- f
}

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
