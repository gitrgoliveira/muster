package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	api "github.com/gitrgoliveira/muster/internal/api"
	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/gitrgoliveira/muster/internal/store"
	"github.com/gitrgoliveira/muster/internal/ws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRouterTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	// Minimal UI FS — the router expects a "ui/" sub-directory.
	uiFS := fstest.MapFS{
		"ui/index.html": &fstest.MapFile{Data: []byte("<!DOCTYPE html><body>test</body>")},
	}

	ms := store.NewMemStore(store.SeedBeads())
	pub := services.Publisher(func(f ws.Frame) {})
	svc := services.NewBeadService(ms, pub)

	hub := ws.NewHub("0.9.1")
	go hub.Run()

	seedDolt := store.SeedDolt()
	handler := api.NewRouter(svc, ms, hub, uiFS, seedDolt, "0.9.1")

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

// TestRouter_StaticUIServed verifies GET / returns 200 with HTML content.
func TestRouter_StaticUIServed(t *testing.T) {
	srv := newRouterTestServer(t)

	resp, err := srv.Client().Get(srv.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.True(t, strings.Contains(strings.ToLower(string(body)), "html"),
		"response body should contain 'html'")
}

// TestRouter_APINotFound_ReturnsJSON verifies that unknown API paths return 404 JSON.
func TestRouter_APINotFound_ReturnsJSON(t *testing.T) {
	srv := newRouterTestServer(t)

	resp, err := srv.Client().Get(srv.URL + "/api/v1/nonexistent")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	errObj, ok := body["error"].(map[string]interface{})
	require.True(t, ok, "response should have an 'error' object")
	assert.Equal(t, "NOT_FOUND", errObj["code"], "error code should be NOT_FOUND")
}

// TestRouter_MethodNotAllowed_ReturnsJSON verifies that disallowed methods return 405 JSON.
func TestRouter_MethodNotAllowed_ReturnsJSON(t *testing.T) {
	srv := newRouterTestServer(t)

	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/beads", nil)
	require.NoError(t, err)

	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	errObj, ok := body["error"].(map[string]interface{})
	require.True(t, ok, "response should have an 'error' object")
	assert.Equal(t, "METHOD_NOT_ALLOWED", errObj["code"], "error code should be METHOD_NOT_ALLOWED")
}
