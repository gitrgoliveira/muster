package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

// Client represents a single WebSocket connection managed by the Hub.
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan Frame

	// drop tracking for slow-client policy
	drops    int
	lastDrop time.Time
}

// ServeWS upgrades an HTTP connection to WebSocket, creates a Client, starts
// its read/write pumps, and registers it with the hub.
func ServeWS(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // allow cross-origin in dev
	})
	if err != nil {
		slog.Error("ws: upgrade failed", "err", err)
		return
	}
	conn.SetReadLimit(1 << 20)

	c := &Client{
		hub:  hub,
		conn: conn,
		send: make(chan Frame, sendBufSize),
	}
	hub.register <- c

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go c.writePump(ctx)
	c.readPump(ctx) // blocks until connection closes
}

// writePump serialises frames from the send channel to the WebSocket connection.
func (c *Client) writePump(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case f, ok := <-c.send:
			if !ok {
				// Hub closed the channel.
				_ = c.conn.Close(websocket.StatusNormalClosure, "")
				return
			}
			data, err := json.Marshal(f)
			if err != nil {
				slog.Error("ws: marshal frame", "err", err)
				continue
			}
			if err := c.conn.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		}
	}
}

// readPump reads client frames and handles ping/unknown.
func (c *Client) readPump(ctx context.Context) {
	defer func() {
		c.hub.unregister <- c
	}()

	for {
		_, msg, err := c.conn.Read(ctx)
		if err != nil {
			return
		}
		var cf ClientFrame
		if err := json.Unmarshal(msg, &cf); err != nil {
			slog.Warn("ws: malformed client frame", "err", err)
			continue
		}
		switch cf.Type {
		case "ping":
			pong := Frame{Type: EventPong, At: time.Now().UTC().Format(time.RFC3339)}
			data, _ := json.Marshal(pong)
			if err := c.conn.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		default:
			slog.Warn("ws: unknown client frame type", "type", cf.Type)
		}
	}
}
