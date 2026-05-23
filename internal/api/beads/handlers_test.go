package beads_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/api/beads"
	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/gitrgoliveira/muster/internal/store"
	"github.com/gitrgoliveira/muster/internal/ws"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T) (*httptest.Server, *store.MemStore) {
	t.Helper()
	srv, ms, _ := newTestServerCapture(t)
	return srv, ms
}

// newTestServerCapture is like newTestServer but also returns a pointer to the
// last published WS frame so event-emission tests can inspect it.
func newTestServerCapture(t *testing.T) (*httptest.Server, *store.MemStore, *ws.Frame) {
	t.Helper()
	ms := store.NewMemStore(store.SeedBeads())
	var captured ws.Frame
	pub := services.Publisher(func(f ws.Frame) { captured = f })
	svc := services.NewBeadService(ms, pub)
	h := beads.NewHandlers(svc, ms)
	r := chi.NewRouter()
	r.Get("/beads", h.List)
	r.Post("/beads", h.Create)
	r.Get("/beads/{id}", h.Get)
	r.Patch("/beads/{id}", h.Patch)
	r.Post("/beads/{id}/move", h.Move)
	r.Post("/beads/{id}/dispatch", h.Dispatch)
	r.Post("/beads/{id}/comments", h.Comment)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, ms, &captured
}

// helpers

func doGet(t *testing.T, srv *httptest.Server, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(srv.URL + path)
	require.NoError(t, err)
	return resp
}

func doPost(t *testing.T, srv *httptest.Server, path string, body interface{}) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := http.Post(srv.URL+path, "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	return resp
}

func doPatch(t *testing.T, srv *httptest.Server, path string, body interface{}) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPatch, srv.URL+path, bytes.NewReader(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func decodeBody(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	defer resp.Body.Close()
	require.NoError(t, json.NewDecoder(resp.Body).Decode(v))
}

func errorCode(t *testing.T, resp *http.Response) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeBody(t, resp, &env)
	return env.Error.Code
}

// ── T026 List ──────────────────────────────────────────────────────────────

func TestList_NoFilter(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doGet(t, srv, "/beads")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var lr beads.ListResponse
	decodeBody(t, resp, &lr)
	assert.Equal(t, 14, len(lr.Items))
	assert.Nil(t, lr.NextCursor)
	assert.Equal(t, 14, lr.Total)
}

func TestList_FilterByColumn(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doGet(t, srv, "/beads?column=running")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var lr beads.ListResponse
	decodeBody(t, resp, &lr)

	for _, b := range lr.Items {
		assert.Equal(t, core.ColRunning, b.Column)
	}
	assert.Equal(t, 2, len(lr.Items)) // bd-a1f2, bd-c411
	assert.Equal(t, 2, lr.Total)
}

func TestList_InvalidColumn_400(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doGet(t, srv, "/beads?column=invalid")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

// ── T027 Create ────────────────────────────────────────────────────────────

func TestCreate_201_WithDefaults(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads", map[string]interface{}{
		"title": "Test bead",
	})
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var b core.Bead
	decodeBody(t, resp, &b)
	assert.Regexp(t, `^bd-[0-9a-f]{4}$`, b.ID)
	assert.Equal(t, core.ColBacklog, b.Column)
	assert.Equal(t, core.TypeTask, b.Type)
	assert.Equal(t, core.Priority(2), b.Priority)
}

func TestCreate_LocationHeader(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads", map[string]interface{}{
		"title": "Test bead",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var b core.Bead
	decodeBody(t, resp, &b)
	assert.Equal(t, "/api/v1/beads/"+b.ID, resp.Header.Get("Location"))
}

func TestCreate_400_MissingTitle(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads", map[string]interface{}{})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestCreate_400_InvalidEnum(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads", map[string]interface{}{
		"title": "x",
		"type":  "invalid",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestCreate_400_UnknownField(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads", map[string]interface{}{
		"title": "x",
		"foo":   "bar",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

// ── T028 Get ───────────────────────────────────────────────────────────────

func TestGet_200(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doGet(t, srv, "/beads/bd-a1f2")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var b core.Bead
	decodeBody(t, resp, &b)
	assert.Equal(t, "bd-a1f2", b.ID)
}

func TestGet_404(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doGet(t, srv, "/beads/bd-0000")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "BEAD_NOT_FOUND", errorCode(t, resp))
}

func TestGet_400_BadIDFormat(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doGet(t, srv, "/beads/notanid")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

// ── T029 Patch ─────────────────────────────────────────────────────────────

func TestPatch_PartialUpdate(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doPatch(t, srv, "/beads/bd-a1f2", map[string]interface{}{
		"title": "New title",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var b core.Bead
	decodeBody(t, resp, &b)
	assert.Equal(t, "New title", b.Title)
}

func TestPatch_404(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doPatch(t, srv, "/beads/bd-0000", map[string]interface{}{
		"title": "X",
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "BEAD_NOT_FOUND", errorCode(t, resp))
}

func TestPatch_400_EmptyBody(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doPatch(t, srv, "/beads/bd-a1f2", map[string]interface{}{})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestPatch_400_UnknownField(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	// Use raw JSON to ensure the field goes through.
	body := strings.NewReader(`{"foo":"bar"}`)
	req, err := http.NewRequest(http.MethodPatch, srv.URL+"/beads/bd-a1f2", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

// ── T030 Move ──────────────────────────────────────────────────────────────

func TestMove_200_ToColumn(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/bd-a1f2/move", map[string]interface{}{
		"toColumn": "review",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var b core.Bead
	decodeBody(t, resp, &b)
	assert.Equal(t, core.ColReview, b.Column)
}

func TestMove_400_MissingToColumn(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/bd-a1f2/move", map[string]interface{}{})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestMove_400_UnknownColumn(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/bd-a1f2/move", map[string]interface{}{
		"toColumn": "invalid",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestMove_200_WithBeforeID_Reorders(t *testing.T) {
	srv, ms, _ := newTestServerCapture(t)

	// Move bd-a1f2 (running) to backlog, inserting it before bd-4f12 (backlog).
	// Seed order of backlog beads: bd-b210, bd-4f12, bd-2d55, bd-7e21.
	resp := doPost(t, srv, "/beads/bd-a1f2/move", map[string]interface{}{
		"toColumn": "backlog",
		"beforeID": "bd-4f12",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var b core.Bead
	decodeBody(t, resp, &b)
	assert.Equal(t, core.ColBacklog, b.Column)

	// bd-a1f2 must appear before bd-4f12 in the global order.
	all, err := ms.List(context.Background(), "")
	require.NoError(t, err)
	var a1f2Idx, f12Idx int
	for i, bead := range all {
		if bead.ID == "bd-a1f2" {
			a1f2Idx = i
		}
		if bead.ID == "bd-4f12" {
			f12Idx = i
		}
	}
	assert.Less(t, a1f2Idx, f12Idx, "bd-a1f2 should appear before bd-4f12 after move with beforeID")
}

func TestMove_400_UnknownBeforeID(t *testing.T) {
	srv, _, _ := newTestServerCapture(t)

	resp := doPost(t, srv, "/beads/bd-a1f2/move", map[string]interface{}{
		"toColumn": "backlog",
		"beforeID": "bd-ffff",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestMove_400_BeforeIDDifferentColumn(t *testing.T) {
	srv, _, _ := newTestServerCapture(t)

	// bd-3e80 is in review; moving bd-a1f2 to backlog with a beforeID from review is invalid.
	resp := doPost(t, srv, "/beads/bd-a1f2/move", map[string]interface{}{
		"toColumn": "backlog",
		"beforeID": "bd-3e80",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestMove_400_BeforeIDEqualsMovedBead(t *testing.T) {
	srv, _, _ := newTestServerCapture(t)

	resp := doPost(t, srv, "/beads/bd-a1f2/move", map[string]interface{}{
		"toColumn": "running",
		"beforeID": "bd-a1f2",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestMove_404(t *testing.T) {
	srv, _, _ := newTestServerCapture(t)

	resp := doPost(t, srv, "/beads/bd-0000/move", map[string]interface{}{
		"toColumn": "backlog",
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "BEAD_NOT_FOUND", errorCode(t, resp))
}

func TestMove_EmitsBeadMovedEvent(t *testing.T) {
	srv, _, captured := newTestServerCapture(t)

	resp := doPost(t, srv, "/beads/bd-a1f2/move", map[string]interface{}{
		"toColumn": "review",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	assert.Equal(t, ws.EventBeadMoved, captured.Type)
	assert.Equal(t, "bd-a1f2", captured.ID)
	assert.Equal(t, core.ColRunning, captured.FromColumn)
	assert.Equal(t, core.ColReview, captured.ToColumn)
}

// ── T031 Dispatch ──────────────────────────────────────────────────────────

func TestDispatch_200_FromScheduled(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/bd-7c0d/dispatch", map[string]interface{}{
		"agent": "claude",
		"mode":  "build",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var b core.Bead
	decodeBody(t, resp, &b)
	assert.Equal(t, core.ColRunning, b.Column)
}

func TestDispatch_400_InvalidState_FromBacklog(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/bd-b210/dispatch", map[string]interface{}{
		"agent": "claude",
		"mode":  "build",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_STATE", errorCode(t, resp))
}

func TestDispatch_400_InvalidState_FromRunning(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	// bd-a1f2 is in running (not scheduled).
	resp := doPost(t, srv, "/beads/bd-a1f2/dispatch", map[string]interface{}{
		"agent": "claude",
		"mode":  "build",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_STATE", errorCode(t, resp))
}

func TestDispatch_400_InvalidAgent(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/bd-7c0d/dispatch", map[string]interface{}{
		"agent": "unknown",
		"mode":  "build",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestDispatch_400_InvalidMode(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/bd-7c0d/dispatch", map[string]interface{}{
		"agent": "claude",
		"mode":  "unknown",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestDispatch_404(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/bd-0000/dispatch", map[string]interface{}{
		"agent": "claude",
		"mode":  "build",
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "BEAD_NOT_FOUND", errorCode(t, resp))
}

// ── T032 Comment ───────────────────────────────────────────────────────────

func TestComment_201_AppendsHistory(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/bd-a1f2/comments", map[string]interface{}{
		"actor": "you",
		"note":  "LGTM",
	})
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var b core.Bead
	decodeBody(t, resp, &b)
	// Verify a comment event was appended.
	found := false
	for _, ev := range b.History {
		if ev.Kind == core.EvComment && ev.Actor == "you" && ev.Note == "LGTM" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected comment event in history")
}

func TestComment_400_MissingActor(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/bd-a1f2/comments", map[string]interface{}{
		"actor": "",
		"note":  "hi",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestComment_400_MissingNote(t *testing.T) {
	srv, _, _ := newTestServerCapture(t)

	resp := doPost(t, srv, "/beads/bd-a1f2/comments", map[string]interface{}{
		"actor": "tester",
		"note":  "",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestComment_IncrementsCount(t *testing.T) {
	srv, ms, _ := newTestServerCapture(t)

	initial, err := ms.Get(context.Background(), "bd-a1f2")
	require.NoError(t, err)
	initialCount := initial.Comments

	resp := doPost(t, srv, "/beads/bd-a1f2/comments", map[string]interface{}{
		"actor": "tester",
		"note":  "LGTM",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var b core.Bead
	decodeBody(t, resp, &b)
	assert.Equal(t, initialCount+1, b.Comments)
}

func TestComment_EmitsCommentAddedEvent(t *testing.T) {
	srv, _, captured := newTestServerCapture(t)

	resp := doPost(t, srv, "/beads/bd-a1f2/comments", map[string]interface{}{
		"actor": "tester",
		"note":  "event check",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	assert.Equal(t, ws.EventCommentAdded, captured.Type)
	assert.Equal(t, "bd-a1f2", captured.ID)
	require.NotNil(t, captured.Event)
	assert.Equal(t, core.EvComment, captured.Event.Kind)
	assert.Equal(t, "tester", captured.Event.Actor)
}

func TestComment_404(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/bd-0000/comments", map[string]interface{}{
		"actor": "you",
		"note":  "hi",
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "BEAD_NOT_FOUND", errorCode(t, resp))
}
