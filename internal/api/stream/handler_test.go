package stream_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gitrgoliveira/muster/internal/api/stream"
	"github.com/gitrgoliveira/muster/internal/ws"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T) (*httptest.Server, *ws.Hub) {
	t.Helper()
	hub := ws.NewHub("0.9.1")
	go hub.Run()

	r := chi.NewRouter()
	r.Get("/stream", stream.StreamHandler(hub))

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, hub
}

// wsURL converts an httptest server HTTP URL to ws://.
func wsURL(srv *httptest.Server, path string) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + path
}

// dialWS dials the test server WebSocket endpoint and returns the connection.
func dialWS(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, resp, err := websocket.Dial(ctx, wsURL(srv, "/stream"), nil)
	require.NoError(t, err, "websocket dial")
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	t.Cleanup(func() { conn.CloseNow() }) //nolint:errcheck
	return conn
}

// readFrame reads and unmarshals one WebSocket frame from conn within timeout.
func readFrame(t *testing.T, conn *websocket.Conn, timeout time.Duration) map[string]interface{} {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_, msg, err := conn.Read(ctx)
	require.NoError(t, err, "read ws frame")

	var frame map[string]interface{}
	require.NoError(t, json.Unmarshal(msg, &frame), "unmarshal ws frame")
	return frame
}

// TestStream_UpgradesToWS verifies that the endpoint performs a WebSocket upgrade.
// A successful dial implies the server returned 101 Switching Protocols.
func TestStream_UpgradesToWS(t *testing.T) {
	srv, _ := newTestServer(t)
	conn := dialWS(t, srv)
	// Drain hello so the hub doesn't block on a slow client.
	_ = readFrame(t, conn, 1*time.Second)
	// Close cleanly.
	_ = conn.Close(websocket.StatusNormalClosure, "done")
}

// TestStream_SendsHelloWithinOneSecond verifies the hello frame is received on connect.
func TestStream_SendsHelloWithinOneSecond(t *testing.T) {
	srv, _ := newTestServer(t)
	conn := dialWS(t, srv)

	frame := readFrame(t, conn, 1*time.Second)

	assert.Equal(t, "hello", frame["type"], "frame type should be hello")
	assert.NotEmpty(t, frame["build"], "build should be non-empty")

	schemaVersion, ok := frame["schemaVersion"].(float64)
	require.True(t, ok, "schemaVersion should be a number")
	assert.Greater(t, schemaVersion, float64(0), "schemaVersion should be > 0")

	assert.NotEmpty(t, frame["serverTime"], "serverTime should be non-empty")
	assert.Equal(t, "0.9.1", frame["beadsVersion"], "beadsVersion should match hub config")
}

// TestStream_PingProducesPong verifies that sending a ping frame yields a pong.
func TestStream_PingProducesPong(t *testing.T) {
	srv, _ := newTestServer(t)
	conn := dialWS(t, srv)

	// Drain the hello frame first.
	_ = readFrame(t, conn, 1*time.Second)

	// Send ping.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pingMsg, _ := json.Marshal(map[string]string{"type": "ping"})
	err := conn.Write(ctx, websocket.MessageText, pingMsg)
	require.NoError(t, err, "write ping")

	// Read pong within 500ms.
	frame := readFrame(t, conn, 500*time.Millisecond)
	assert.Equal(t, "pong", frame["type"], "response type should be pong")
	assert.NotEmpty(t, frame["at"], "pong at field should be non-empty")
}

// TestStream_BroadcastDelivered verifies that a direct hub.Broadcast reaches a connected client.
func TestStream_BroadcastDelivered(t *testing.T) {
	srv, hub := newTestServer(t)
	conn := dialWS(t, srv)

	// Drain hello frame.
	_ = readFrame(t, conn, 1*time.Second)

	// Broadcast a test event.
	hub.Broadcast(ws.Frame{Type: "test.event"})

	// Expect the event within 500ms.
	frame := readFrame(t, conn, 500*time.Millisecond)
	assert.Equal(t, "test.event", frame["type"])
}

// TestStream_DisconnectUnregistersClient verifies a closed connection is cleaned up
// so that subsequent broadcasts do not block.
func TestStream_DisconnectUnregistersClient(t *testing.T) {
	srv, hub := newTestServer(t)
	conn := dialWS(t, srv)

	// Drain hello.
	_ = readFrame(t, conn, 1*time.Second)

	// Close the WS connection (client side).
	_ = conn.Close(websocket.StatusNormalClosure, "done")

	// Give the hub time to unregister the client.
	time.Sleep(100 * time.Millisecond)

	// Broadcast should not block — send via Broadcast and verify it returns.
	done := make(chan struct{})
	go func() {
		hub.Broadcast(ws.Frame{Type: "test.event"})
		close(done)
	}()

	select {
	case <-done:
		// OK — broadcast did not block.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Broadcast blocked after client disconnect")
	}
}
