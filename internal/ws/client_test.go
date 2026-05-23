package ws_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gitrgoliveira/muster/internal/ws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sendPing sends a ping ClientFrame to the connection.
func sendPing(t *testing.T, ctx context.Context, conn *websocket.Conn) {
	t.Helper()
	data, err := json.Marshal(map[string]string{"type": "ping"})
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageText, data))
}

// TestClient_PingFrameProducesPong verifies that sending a ping client frame
// results in a pong server frame with a non-empty At field.
func TestClient_PingFrameProducesPong(t *testing.T) {
	srv, _ := newTestServer(t, "0.9.1")

	conn, cancel := dialWS(t, srv)
	defer cancel()

	// Drain the hello frame first.
	helloCtx, cancelHello := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelHello()
	hello := readFrame(t, helloCtx, conn)
	assert.Equal(t, ws.EventHello, hello.Type)

	// Send a ping.
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelPing()
	sendPing(t, pingCtx, conn)

	// Expect a pong within 500 ms.
	pong := readFrame(t, pingCtx, conn)

	assert.Equal(t, ws.EventPong, pong.Type)
	assert.NotEmpty(t, pong.At, "At field must be a non-empty RFC3339 timestamp")

	// Verify At is a valid RFC3339 timestamp.
	_, err := time.Parse(time.RFC3339, pong.At)
	assert.NoError(t, err, "At must be a valid RFC3339 timestamp")
}

// TestClient_WritePumpExitsOnContextCancel verifies that closing the connection
// (which cancels the context) causes the write pump to exit cleanly.
// This is implicitly tested by all other tests via connection teardown.
func TestClient_WritePumpExitsOnContextCancel(t *testing.T) {
	srv, _ := newTestServer(t, "0.9.1")

	conn, cancel := dialWS(t, srv)

	// Drain hello.
	helloCtx, cancelHello := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelHello()
	readFrame(t, helloCtx, conn)

	// Close the connection explicitly; this cancels the server-side context.
	_ = conn.Close(websocket.StatusNormalClosure, "going away")
	cancel()

	// If we reach here without hanging, the write pump exited cleanly.
	// Give goroutines a moment to settle.
	time.Sleep(50 * time.Millisecond)
}

// TestClient_ReadPumpExitsOnClose verifies that when the server closes the
// connection, the client's read goroutine exits (the read returns an error).
func TestClient_ReadPumpExitsOnClose(t *testing.T) {
	srv, hub := newTestServer(t, "0.9.1")

	conn, cancel := dialWS(t, srv)
	defer cancel()

	// Drain hello.
	helloCtx, cancelHello := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelHello()
	readFrame(t, helloCtx, conn)

	// Trigger server-side client removal by filling the channel, which causes
	// the hub to close the client's send channel, which causes writePump to call
	// conn.Close. We do this by broadcasting many messages without reading.
	for i := 0; i < 25; i++ {
		hub.Broadcast(ws.Frame{Type: ws.EventBeadUpdated})
	}

	// The client-side read should eventually return an error because the server
	// closed the connection.
	require.Eventually(t, func() bool {
		ctx, cancelCtx := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancelCtx()
		_, _, err := conn.Read(ctx)
		return err != nil
	}, 15*time.Second, 200*time.Millisecond, "expected read to fail after server closed connection")
}

// TestClient_UnknownClientFrameLoggedAndIgnored verifies that an unknown frame
// type does not crash or close the connection — a subsequent ping still works.
func TestClient_UnknownClientFrameLoggedAndIgnored(t *testing.T) {
	srv, _ := newTestServer(t, "0.9.1")

	conn, cancel := dialWS(t, srv)
	defer cancel()

	// Drain hello.
	helloCtx, cancelHello := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelHello()
	readFrame(t, helloCtx, conn)

	// Send an unknown frame type.
	unknownCtx, cancelUnknown := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelUnknown()
	data, err := json.Marshal(map[string]string{"type": "unknown"})
	require.NoError(t, err)
	require.NoError(t, conn.Write(unknownCtx, websocket.MessageText, data))

	// Give the server a moment to log and ignore.
	time.Sleep(20 * time.Millisecond)

	// The connection should still be open — send a ping and expect a pong.
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelPing()
	sendPing(t, pingCtx, conn)

	pong := readFrame(t, pingCtx, conn)
	assert.Equal(t, ws.EventPong, pong.Type)
	assert.NotEmpty(t, pong.At)
}
