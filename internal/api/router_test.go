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
	"github.com/gitrgoliveira/muster/internal/api/health"
	"github.com/gitrgoliveira/muster/internal/api/middleware"
	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/gitrgoliveira/muster/internal/store"
	"github.com/gitrgoliveira/muster/internal/ws"
	chi "github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRouterTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	// Minimal UI FS — the router expects a "ui/" sub-directory.
	uiFS := fstest.MapFS{
		"ui/index.html": &fstest.MapFile{Data: []byte("<!DOCTYPE html><body>test</body>")},
	}

	backend := store.NewMemoryBackend(store.SeedIssues())
	pub := services.Publisher(func(f ws.Frame) {})
	svc := services.NewBeadService(backend, nil, pub)

	hub := ws.NewHub("0.9.1")
	go hub.Run()

	handler := api.NewRouter(svc, hub, uiFS, health.StatusConfig{BeadsVersion: "0.9.1", SchemaVersion: 1}, api.M6Services{})

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

// TestRouter_PanicRecovered_Returns500JSON verifies that a panicking handler
// returns a 500 INTERNAL JSON response instead of crashing the server.
func TestRouter_PanicRecovered_Returns500JSON(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recovery)
	r.Get("/panic", func(w http.ResponseWriter, req *http.Request) {
		panic("simulated handler panic")
	})
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/panic")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "INTERNAL", body.Error.Code)
}

// ── T034: Additive-surface regression (SC-007) ────────────────────────────
//
// Assert every M0/M1/M2 route is still reachable (returns a non-5xx status)
// and that M0/M1/M2 status fields are still present in the status response.
// M3 routes and fields are validated separately; this test must not fail when
// M3 adds new surface.

func TestRouter_AdditiveRegression_RoutesPresent(t *testing.T) {
	srv := newRouterTestServer(t)

	// Table of M0/M1/M2/M3 routes: (method, path, expected-status-not-5xx).
	// Some routes return 404 BEAD_NOT_FOUND or 400 for a missing bead — that's
	// expected; the point is the route exists (not 405 METHOD_NOT_ALLOWED).
	routes := []struct {
		method string
		path   string
	}{
		// M0/M1 routes.
		{http.MethodGet, "/api/v1/healthz"},
		{http.MethodGet, "/api/v1/orchestrator/status"},
		{http.MethodGet, "/api/v1/beads"},
		{http.MethodGet, "/api/v1/beads/mp-aaa"}, // seeded bead
		// M2 routes — these may 404 BEAD_NOT_FOUND or 422 but must NOT 405.
		{http.MethodGet, "/api/v1/beads/mp-aaa/steps/0/attach"},
		// M3 routes (additive).
		{http.MethodGet, "/api/v1/beads/mp-aaa/worktree"},
		{http.MethodGet, "/api/v1/beads/mp-aaa/diff"},
	}

	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			req, err := http.NewRequest(rt.method, srv.URL+rt.path, nil)
			require.NoError(t, err)
			resp, err := srv.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusMethodNotAllowed {
				t.Errorf("route %s %s got 405 METHOD_NOT_ALLOWED — route registration regressed", rt.method, rt.path)
			}
			if resp.StatusCode >= 500 {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("route %s %s got unexpected 5xx: %d\n%s", rt.method, rt.path, resp.StatusCode, body)
			}
		})
	}
}

func TestRouter_AdditiveRegression_StatusFieldsPresent(t *testing.T) {
	srv := newRouterTestServer(t)

	resp, err := srv.Client().Get(srv.URL + "/api/v1/orchestrator/status")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var raw map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&raw))

	// M0 fields — must never be removed.
	for _, key := range []string{
		"build", "schemaVersion", "beadsVersion", "online", "serverTime",
	} {
		if _, ok := raw[key]; !ok {
			t.Errorf("M0 field %q missing from /orchestrator/status", key)
		}
	}
	// M2 fields — must never be removed.
	for _, key := range []string{
		"tmuxAvailable", "runningCount",
	} {
		if _, ok := raw[key]; !ok {
			t.Errorf("M2 field %q missing from /orchestrator/status", key)
		}
	}
	// M3 additions — should be present (additive).
	for _, key := range []string{
		"vcs", "worktreeCount",
	} {
		if _, ok := raw[key]; !ok {
			t.Errorf("M3 field %q missing from /orchestrator/status", key)
		}
	}
}

func TestRouter_AdditiveRegression_WSEventTypes(t *testing.T) {
	// Assert the WS event type string values from M0/M1/M2 are unchanged.
	// These are wire-format values — changing them would break connected clients.
	cases := []struct {
		event ws.EventType
		want  string
	}{
		{ws.EventHello, "hello"},
		{ws.EventBeadCreated, "bead.created"},
		{ws.EventBeadUpdated, "bead.updated"},
		{ws.EventBeadMoved, "bead.moved"},
		{ws.EventBeadDeleted, "bead.deleted"},
		{ws.EventCommentAdded, "comment.added"},
		{ws.EventPong, "pong"},
		// M2 event types.
		{ws.EventRunlogLine, "runlog.line"},
		{ws.EventTmuxOpened, "tmux.session.opened"},
		{ws.EventTmuxClosed, "tmux.session.closed"},
	}
	for _, tc := range cases {
		if string(tc.event) != tc.want {
			t.Errorf("EventType %q has value %q, want %q (wire format regressed)", tc.event, string(tc.event), tc.want)
		}
	}
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
