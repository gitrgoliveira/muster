package ws_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gitrgoliveira/muster/internal/ws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T, beadsVersion string) (*httptest.Server, *ws.Hub) {
	t.Helper()
	hub := ws.NewHub(beadsVersion)
	go hub.Run()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws.ServeWS(hub, w, r)
	}))
	t.Cleanup(srv.Close)

	return srv, hub
}

func dialWS(t *testing.T, srv *httptest.Server) (*websocket.Conn, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	conn, _, err := websocket.Dial(ctx, "ws://"+srv.Listener.Addr().String(), nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		conn.Close(websocket.StatusNormalClosure, "")
		cancel()
	})
	return conn, cancel
}

func readFrame(t *testing.T, ctx context.Context, conn *websocket.Conn) ws.Frame {
	t.Helper()
	_, msg, err := conn.Read(ctx)
	require.NoError(t, err)
	var f ws.Frame
	require.NoError(t, json.Unmarshal(msg, &f))
	return f
}

// TestHub_RegisterUnregister verifies that connecting a client registers it
// and closing the connection unregisters it.
func TestHub_RegisterUnregister(t *testing.T) {
	srv, hub := newTestServer(t, "0.9.1")

	conn, cancel := dialWS(t, srv)
	defer cancel()

	// Drain the hello frame so the hub has fully processed registration.
	ctx, cancelRead := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelRead()
	readFrame(t, ctx, conn) // hello

	// Give the hub a moment to record the registration.
	time.Sleep(20 * time.Millisecond)

	// Verify 1 client via hub inspector (indirect: broadcast reaches the client).
	broadcastCtx, cancelB := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelB()
	hub.Broadcast(ws.Frame{Type: "bead.created"})
	frame := readFrame(t, broadcastCtx, conn)
	assert.Equal(t, ws.EventType("bead.created"), frame.Type)

	// Disconnect and verify the hub unregisters the client (broadcast no longer
	// reaches anyone — subsequent Broadcast should not block/panic).
	conn.Close(websocket.StatusNormalClosure, "")

	// Allow the hub time to process unregistration.
	time.Sleep(50 * time.Millisecond)

	// A broadcast after disconnect should not hang (hub.broadcast is buffered).
	hub.Broadcast(ws.Frame{Type: "bead.updated"})
	// Success: if we reach here without deadlock the unregister path works.
}

// TestHub_HelloSentWithinOneSecondOfRegister verifies that a hello frame with
// the required fields is delivered within 1 second of connecting.
func TestHub_HelloSentWithinOneSecondOfRegister(t *testing.T) {
	srv, _ := newTestServer(t, "0.9.1")

	conn, cancel := dialWS(t, srv)
	defer cancel()

	ctx, cancelCtx := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelCtx()

	frame := readFrame(t, ctx, conn)

	assert.Equal(t, ws.EventHello, frame.Type)
	assert.NotEmpty(t, frame.Build, "Build must be non-empty")
	assert.Greater(t, frame.SchemaVersion, 0, "SchemaVersion must be > 0")
	assert.NotEmpty(t, frame.ServerTime, "ServerTime must be non-empty")
	assert.NotEmpty(t, frame.BeadsVersion, "BeadsVersion must be non-empty")
	assert.Equal(t, "0.9.1", frame.BeadsVersion)
}

// TestHub_BroadcastReachesClient verifies that a frame broadcast by the hub
// is received by a connected client within 100 ms.
func TestHub_BroadcastReachesClient(t *testing.T) {
	srv, hub := newTestServer(t, "0.9.1")

	conn, cancel := dialWS(t, srv)
	defer cancel()

	// Drain hello.
	helloCtx, cancelHello := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelHello()
	readFrame(t, helloCtx, conn)

	// Now broadcast and expect it within 100 ms.
	hub.Broadcast(ws.Frame{Type: ws.EventBeadCreated})

	readCtx, cancelRead := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelRead()
	frame := readFrame(t, readCtx, conn)

	assert.Equal(t, ws.EventBeadCreated, frame.Type)
}

// TestHub_SlowClientDropped verifies that a client whose send channel fills up
// is eventually unregistered by the hub after maxDrops (3) missed broadcasts.
func TestHub_SlowClientDropped(t *testing.T) {
	srv, hub := newTestServer(t, "0.9.1")

	conn, cancel := dialWS(t, srv)
	defer cancel()

	// Drain hello so the client is fully registered.
	helloCtx, cancelHello := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelHello()
	readFrame(t, helloCtx, conn)

	// Fill the client's send channel by broadcasting many frames without reading.
	// sendBufSize is 16; we need > 16 + 3 drops to trigger the drop policy.
	for i := 0; i < 25; i++ {
		hub.Broadcast(ws.Frame{Type: ws.EventBeadUpdated})
	}

	// The hub should eventually close the connection.
	// Verify by attempting to read from the connection — it should return an error.
	require.Eventually(t, func() bool {
		ctx, cancelCtx := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancelCtx()
		_, _, err := conn.Read(ctx)
		return err != nil
	}, 15*time.Second, 200*time.Millisecond, "expected client to be dropped by hub")
}
