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

// newTestServer creates an httptest.Server backed by SeedIssues with a nil CLI.
// Write operations will return 501 BD_CLI_MISSING.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	backend := store.NewMemoryBackend(store.SeedIssues())
	pub := services.Publisher(func(f ws.Frame) {})
	svc := services.NewBeadService(backend, nil, pub)
	h := beads.NewHandlers(svc)
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
	return srv
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

// ── List ──────────────────────────────────────────────────────────────

func TestList_NoFilter(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doGet(t, srv, "/beads")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var lr beads.ListResponse
	decodeBody(t, resp, &lr)
	// SeedIssues returns 4 issues.
	assert.Equal(t, 4, len(lr.Items))
	assert.Nil(t, lr.NextCursor)
	assert.Equal(t, 4, lr.Total)
}

func TestList_FilterByColumn(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// "running" maps to status "in_progress"; mp-bbb has status in_progress.
	resp := doGet(t, srv, "/beads?column=running")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var lr beads.ListResponse
	decodeBody(t, resp, &lr)

	for _, b := range lr.Items {
		assert.Equal(t, core.ColRunning, b.Column)
	}
	assert.Equal(t, 1, len(lr.Items))
	assert.Equal(t, 1, lr.Total)
}

func TestList_FilterByColumn_Done(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// "done" maps to statuses "closed" and "cancelled"; mp-ccc has status closed.
	resp := doGet(t, srv, "/beads?column=done")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var lr beads.ListResponse
	decodeBody(t, resp, &lr)

	assert.Equal(t, 1, len(lr.Items))
	assert.Equal(t, core.ColDone, lr.Items[0].Column)
}

func TestList_InvalidColumn_400(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doGet(t, srv, "/beads?column=invalid")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

// ── Get ───────────────────────────────────────────────────────────────

func TestGet_200(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doGet(t, srv, "/beads/mp-aaa")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var b core.Bead
	decodeBody(t, resp, &b)
	assert.Equal(t, "mp-aaa", b.ID)
}

func TestGet_404(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doGet(t, srv, "/beads/mp-zzzz")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "BEAD_NOT_FOUND", errorCode(t, resp))
}

func TestGet_400_BadIDFormat(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doGet(t, srv, "/beads/notanid")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

// ── Create — validation passes before CLI check ───────────────────────

func TestCreate_400_MissingTitle(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads", map[string]interface{}{})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestCreate_400_InvalidEnum(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads", map[string]interface{}{
		"title": "x",
		"type":  "invalid",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestCreate_400_UnknownField(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads", map[string]interface{}{
		"title": "x",
		"foo":   "bar",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestCreate_501_CLIMissing(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads", map[string]interface{}{
		"title": "Test bead",
		"type":  "task",
	})
	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
	assert.Equal(t, "BD_CLI_MISSING", errorCode(t, resp))
}

func TestCreate_400_MissingType(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads", map[string]interface{}{
		"title": "Test bead",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

// ── Patch — validation passes before CLI check ────────────────────────

func TestPatch_400_EmptyBody(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPatch(t, srv, "/beads/mp-aaa", map[string]interface{}{})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestPatch_400_UnknownField(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	body := strings.NewReader(`{"foo":"bar"}`)
	req, err := http.NewRequest(http.MethodPatch, srv.URL+"/beads/mp-aaa", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestPatch_501_CLIMissing(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPatch(t, srv, "/beads/mp-aaa", map[string]interface{}{
		"title": "New title",
	})
	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
	assert.Equal(t, "BD_CLI_MISSING", errorCode(t, resp))
}

func TestPatch_DescriptionAlias_Accepted(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// "description" is a documented alias for "desc"; it must not be rejected
	// as an unknown field. With a nil CLI it passes decode/validation and
	// reaches the CLI-missing check (501).
	resp := doPatch(t, srv, "/beads/mp-aaa", map[string]interface{}{
		"description": "via alias",
	})
	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
	assert.Equal(t, "BD_CLI_MISSING", errorCode(t, resp))
}

func TestPatch_400_DescAndDescription(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPatch(t, srv, "/beads/mp-aaa", map[string]interface{}{
		"desc":        "one",
		"description": "two",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestPatch_Assignee_Accepted(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// assignee is a documented PATCH body field; it must pass decode/validation
	// and reach the CLI-missing check (501) rather than being rejected as unknown.
	resp := doPatch(t, srv, "/beads/mp-aaa", map[string]interface{}{
		"assignee": "alice",
	})
	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
	assert.Equal(t, "BD_CLI_MISSING", errorCode(t, resp))
}

// ── Move — validation passes before CLI check ─────────────────────────

func TestMove_400_MissingToColumn(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/mp-aaa/move", map[string]interface{}{})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestMove_400_UnknownColumn(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/mp-aaa/move", map[string]interface{}{
		"toColumn": "invalid",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestMove_501_CLIMissing(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/mp-aaa/move", map[string]interface{}{
		"toColumn": "review",
	})
	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
	assert.Equal(t, "BD_CLI_MISSING", errorCode(t, resp))
}

// ── Dispatch — validation passes before CLI check ─────────────────────

func TestDispatch_400_InvalidAgent(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/mp-aaa/dispatch", map[string]interface{}{
		"agent": "unknown",
		"mode":  "build",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestDispatch_400_InvalidMode(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/mp-aaa/dispatch", map[string]interface{}{
		"agent": "claude",
		"mode":  "unknown",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestDispatch_501_CLIMissing(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/mp-aaa/dispatch", map[string]interface{}{
		"agent": "claude",
		"mode":  "build",
	})
	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
	assert.Equal(t, "BD_CLI_MISSING", errorCode(t, resp))
}

// ── Comment — validation passes before CLI check ──────────────────────

func TestComment_400_MissingActor(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/mp-aaa/comments", map[string]interface{}{
		"actor": "",
		"note":  "hi",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestComment_400_MissingNote(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/mp-aaa/comments", map[string]interface{}{
		"actor": "tester",
		"note":  "",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}

func TestComment_501_CLIMissing(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/mp-aaa/comments", map[string]interface{}{
		"actor": "you",
		"note":  "LGTM",
	})
	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
	assert.Equal(t, "BD_CLI_MISSING", errorCode(t, resp))
}

// ── Dispatch M2 — orchestrator-backed error codes ─────────────────────

// fakeOrchestratorDispatcher implements services.OrchestratorDispatcher for tests.
type fakeOrchestratorDispatcher struct {
	dispatchErr error
	result      *core.Bead
}

func (f *fakeOrchestratorDispatcher) Dispatch(_ context.Context, _ services.OrchestratorDispatchRequest) (*core.Bead, error) {
	if f.dispatchErr != nil {
		return nil, f.dispatchErr
	}
	if f.result != nil {
		return f.result, nil
	}
	return &core.Bead{ID: "mp-aaa", Column: core.ColRunning}, nil
}

// newTestServerWithOrchestrator creates a test server wired with a fake orchestrator.
func newTestServerWithOrchestrator(t *testing.T, orch services.OrchestratorDispatcher) *httptest.Server {
	t.Helper()
	backend := store.NewMemoryBackend(store.SeedIssues())
	pub := services.Publisher(func(f ws.Frame) {})
	svc := services.NewBeadService(backend, nil, pub).WithOrchestrator(orch)
	h := beads.NewHandlers(svc)
	r := chi.NewRouter()
	r.Post("/beads/{id}/dispatch", h.Dispatch)
	r.Get("/beads/{id}", h.Get)
	r.Get("/beads/{id}/steps/{idx}/attach", h.Attach)
	r.Post("/beads/{id}/steps/{idx}/send", h.Send)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func TestDispatch_202_WithOrchestrator(t *testing.T) {
	orch := &fakeOrchestratorDispatcher{result: &core.Bead{
		ID:     "mp-aaa",
		Column: core.ColRunning,
	}}
	srv := newTestServerWithOrchestrator(t, orch)

	resp := doPost(t, srv, "/beads/mp-aaa/dispatch", map[string]interface{}{
		"agent":          "claude",
		"mode":           "agent",
		"permissionMode": "acceptEdits",
	})
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
}

func TestDispatch_409_RunAlreadyActive(t *testing.T) {
	orch := &fakeOrchestratorDispatcher{
		dispatchErr: &services.ServiceError{Code: services.CodeRunAlreadyActive, Message: "run already active for bead"},
	}
	srv := newTestServerWithOrchestrator(t, orch)

	resp := doPost(t, srv, "/beads/mp-aaa/dispatch", map[string]interface{}{
		"agent":          "claude",
		"mode":           "agent",
		"permissionMode": "acceptEdits",
	})
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	assert.Equal(t, "RUN_ALREADY_ACTIVE", errorCode(t, resp))
}

func TestDispatch_422_UnmappedPrefix(t *testing.T) {
	orch := &fakeOrchestratorDispatcher{
		dispatchErr: &services.ServiceError{Code: services.CodeUnmappedPrefix, Message: "bead prefix has no repo mapping"},
	}
	srv := newTestServerWithOrchestrator(t, orch)

	resp := doPost(t, srv, "/beads/mp-aaa/dispatch", map[string]interface{}{
		"agent":          "claude",
		"mode":           "agent",
		"permissionMode": "acceptEdits",
	})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	assert.Equal(t, "UNMAPPED_PREFIX", errorCode(t, resp))
}

func TestDispatch_501_AdapterNotFound(t *testing.T) {
	orch := &fakeOrchestratorDispatcher{
		dispatchErr: &services.ServiceError{Code: services.CodeAdapterNotFound, Message: "adapter not registered"},
	}
	srv := newTestServerWithOrchestrator(t, orch)

	resp := doPost(t, srv, "/beads/mp-aaa/dispatch", map[string]interface{}{
		"agent":          "claude",
		"mode":           "agent",
		"permissionMode": "acceptEdits",
	})
	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
	assert.Equal(t, "ADAPTER_NOT_FOUND", errorCode(t, resp))
}

func TestDispatch_400_InvalidPermissionMode(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/mp-aaa/dispatch", map[string]interface{}{
		"agent":          "claude",
		"mode":           "agent",
		"permissionMode": "badmode",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
}
