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
	r.Get("/beads/{id}/steps/{idx}/attach", h.Attach)
	r.Post("/beads/{id}/steps/{idx}/send", h.Send)
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

// TestGet_404_MultiHyphenID confirms a multi-hyphen bead ID (e.g. a real bd
// molecule bead like mp-mol-4gl) passes the ID-format allow-list rather than
// being rejected as malformed — it reaches the store and returns 404 (not
// found), not 400 (invalid format).
func TestGet_404_MultiHyphenID(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doGet(t, srv, "/beads/mp-mol-4gl")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "BEAD_NOT_FOUND", errorCode(t, resp))
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
	joined      bool
	queued      bool
}

func (f *fakeOrchestratorDispatcher) Dispatch(_ context.Context, _ services.OrchestratorDispatchRequest) (services.OrchestratorDispatchResult, error) {
	if f.dispatchErr != nil {
		return services.OrchestratorDispatchResult{}, f.dispatchErr
	}
	bead := f.result
	if bead == nil {
		bead = &core.Bead{ID: "mp-aaa", Column: core.ColRunning}
	}
	return services.OrchestratorDispatchResult{
		Bead:   bead,
		Joined: f.joined,
		Queued: f.queued,
	}, nil
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

// TestDispatch_200_JoinedRun asserts the idempotent dispatch contract (M4 US4 T048
// migration): a duplicate dispatch of an in-flight bead returns 200 OK with the
// existing run and joined:true, NOT 409. This test replaces the former
// TestDispatch_409_RunAlreadyActive.
func TestDispatch_200_JoinedRun(t *testing.T) {
	activeBead := &core.Bead{
		ID:     "mp-aaa",
		Column: core.ColRunning,
	}
	orch := &fakeOrchestratorDispatcher{
		result: activeBead,
		joined: true,
	}
	srv := newTestServerWithOrchestrator(t, orch)

	resp := doPost(t, srv, "/beads/mp-aaa/dispatch", map[string]interface{}{
		"agent":          "claude",
		"mode":           "agent",
		"permissionMode": "acceptEdits",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, true, body["joined"], "joined field must be true for idempotent dispatch")
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

// ── Attach / Send ───────────────────────────────────────────────────────

func TestAttach_200_NoAttacher(t *testing.T) {
	orch := &fakeOrchestratorDispatcher{}
	srv := newTestServerWithOrchestrator(t, orch)

	resp := doGet(t, srv, "/beads/mp-aaa/steps/0/attach")
	// Bead exists, but this server wires an OrchestratorDispatcher and NO
	// SessionAttacher (WithOrchestrator only), so GetAttach degrades to
	// available:false ("attach not available") — a non-error 200. This asserts
	// the graceful-degradation path, not a "run not active" case.
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestAttach_404_WrongBead(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doGet(t, srv, "/beads/mp-zzzz/steps/0/attach")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestAttach_200_NonZeroIdx verifies that idx≥1 is accepted by the widened
// parseStepIdx (T045). With no orchestrator/attacher wired, the service returns
// {available:false} (200 OK) — the 404 only fires for an unknown bead, not an
// unknown step index (step index validation is service-layer, not handler-layer).
func TestAttach_200_NonZeroIdx(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doGet(t, srv, "/beads/mp-aaa/steps/1/attach")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestAttach_404_NonCanonicalZeroIdx verifies the step-index route requires
// the canonical "0" and rejects non-canonical zero forms (Atoi would accept
// "-0"/"+0"/"00" as 0).
func TestAttach_404_NonCanonicalZeroIdx(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	for _, idx := range []string{"-0", "+0", "00"} {
		resp := doGet(t, srv, "/beads/mp-aaa/steps/"+idx+"/attach")
		assert.Equalf(t, http.StatusNotFound, resp.StatusCode, "idx %q should 404", idx)
	}
}

func TestSend_400_BadBody(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/beads/mp-aaa/steps/0/send",
		strings.NewReader("{invalid json"))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestSend_501_NonZeroIdx verifies that idx≥1 is accepted by the widened
// parseStepIdx (T045). With no attacher, the service returns 501 ATTACH_UNAVAILABLE.
func TestSend_501_NonZeroIdx(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/mp-aaa/steps/2/send", map[string]string{"keys": "y\n"})
	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
}

func TestSend_400_EmptyKeys(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/mp-aaa/steps/0/send", map[string]string{"keys": ""})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSend_400_KeysTooLarge(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := doPost(t, srv, "/beads/mp-aaa/steps/0/send", map[string]string{"keys": strings.Repeat("y", 4097)})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ── Advance / LoopBack (T046) ─────────────────────────────────────────────────

// fakeStepAdvancer is a minimal services.StepAdvancer for handler tests.
type fakeStepAdvancer struct {
	advanceErr  error
	loopbackErr error
	stepIdx     int
	chainLen    int
}

func (f *fakeStepAdvancer) Advance(_ context.Context, beadID string) (stepIdx, chainLen int, err error) {
	if f.advanceErr != nil {
		return 0, 0, f.advanceErr
	}
	return f.stepIdx, f.chainLen, nil
}

func (f *fakeStepAdvancer) LoopBack(_ context.Context, beadID string, toIdx int) (stepIdx, chainLen int, err error) {
	if f.loopbackErr != nil {
		return 0, 0, f.loopbackErr
	}
	return toIdx, f.chainLen, nil
}

// newTestServerWithStepAdvancer creates a test server wired with a fake step advancer.
func newTestServerWithStepAdvancer(t *testing.T, adv services.StepAdvancer) *httptest.Server {
	t.Helper()
	backend := store.NewMemoryBackend(store.SeedIssues())
	pub := services.Publisher(func(f ws.Frame) {})
	svc := services.NewBeadService(backend, nil, pub).WithStepAdvancer(adv)
	h := beads.NewHandlers(svc)
	r := chi.NewRouter()
	r.Post("/beads/{id}/steps/advance", h.AdvanceStep)
	r.Post("/beads/{id}/steps/loopback", h.LoopBackStep)
	r.Get("/beads/{id}/steps/{idx}/attach", h.Attach)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

// TestAdvance_200 verifies the happy path: 200 with {stepIdx, chainLen}.
func TestAdvance_200(t *testing.T) {
	adv := &fakeStepAdvancer{stepIdx: 1, chainLen: 3}
	srv := newTestServerWithStepAdvancer(t, adv)

	resp := doPost(t, srv, "/beads/mp-aaa/steps/advance", map[string]interface{}{})
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		StepIdx  int `json:"stepIdx"`
		ChainLen int `json:"chainLen"`
	}
	decodeBody(t, resp, &body)
	assert.Equal(t, 1, body.StepIdx)
	assert.Equal(t, 3, body.ChainLen)
}

// TestAdvance_400_OutOfRange verifies that ErrStepOutOfRange maps to 400 STEP_OUT_OF_RANGE.
func TestAdvance_400_OutOfRange(t *testing.T) {
	adv := &fakeStepAdvancer{
		advanceErr: &services.ServiceError{Code: services.CodeStepOutOfRange, Message: "already at last step"},
	}
	srv := newTestServerWithStepAdvancer(t, adv)

	resp := doPost(t, srv, "/beads/mp-aaa/steps/advance", map[string]interface{}{})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, services.CodeStepOutOfRange, errorCode(t, resp))
}

// TestAdvance_404_UnknownBead verifies that an unknown bead returns 404.
func TestAdvance_404_UnknownBead(t *testing.T) {
	adv := &fakeStepAdvancer{stepIdx: 1, chainLen: 3}
	srv := newTestServerWithStepAdvancer(t, adv)

	resp := doPost(t, srv, "/beads/mp-zzzz/steps/advance", map[string]interface{}{})
	// bead not found in the store → 404 before advancer is called
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestLoopBack_200 verifies the happy path: 200 with {stepIdx, chainLen}.
func TestLoopBack_200(t *testing.T) {
	adv := &fakeStepAdvancer{chainLen: 3}
	srv := newTestServerWithStepAdvancer(t, adv)

	resp := doPost(t, srv, "/beads/mp-aaa/steps/loopback", map[string]interface{}{"toIdx": 0})
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		StepIdx  int `json:"stepIdx"`
		ChainLen int `json:"chainLen"`
	}
	decodeBody(t, resp, &body)
	assert.Equal(t, 0, body.StepIdx)
	assert.Equal(t, 3, body.ChainLen)
}

// TestLoopBack_400_MissingToIdx verifies that a missing/invalid toIdx returns 400.
func TestLoopBack_400_MissingToIdx(t *testing.T) {
	adv := &fakeStepAdvancer{chainLen: 3}
	srv := newTestServerWithStepAdvancer(t, adv)

	// Body with no toIdx field.
	resp := doPost(t, srv, "/beads/mp-aaa/steps/loopback", map[string]interface{}{})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestLoopBack_400_OutOfRange verifies that ErrStepOutOfRange maps to 400.
func TestLoopBack_400_OutOfRange(t *testing.T) {
	adv := &fakeStepAdvancer{
		loopbackErr: &services.ServiceError{Code: services.CodeStepOutOfRange, Message: "toIdx out of range"},
	}
	srv := newTestServerWithStepAdvancer(t, adv)

	resp := doPost(t, srv, "/beads/mp-aaa/steps/loopback", map[string]interface{}{"toIdx": 5})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, services.CodeStepOutOfRange, errorCode(t, resp))
}
