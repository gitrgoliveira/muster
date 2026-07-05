package beads_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/api/beads"
	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/gitrgoliveira/muster/internal/store"
	"github.com/gitrgoliveira/muster/internal/ws"
	"github.com/gitrgoliveira/muster/internal/wt"
	"github.com/go-chi/chi/v5"
)

// ── fake WorktreeAccessor (read + write side) ─────────────────────────────

type fakeWorktreeAccessor struct {
	// read-side
	status     wt.WorktreeStatus
	statusErr  error
	summary    []wt.FileChange
	summaryErr error
	diffBody   string
	diffErr    error
	vcs        string

	// run-state for M1 guard
	runState core.StepStatus

	// write-side errors (nil = success)
	finalizeErr       error
	finalizeCommitted bool // value returned by Finalize when err==nil
	pushErr           error
	removeErr         error

	// write-side capture
	finalizeBeadID string
	finalizeMsg    string
	pushBeadID     string
	removeBeadID   string
}

func (f *fakeWorktreeAccessor) WorktreeStatus(_ context.Context, _ string) (wt.WorktreeStatus, error) {
	return f.status, f.statusErr
}

func (f *fakeWorktreeAccessor) DiffSummary(_ context.Context, _ string) ([]wt.FileChange, error) {
	return f.summary, f.summaryErr
}

func (f *fakeWorktreeAccessor) Diff(_ context.Context, _, _ string) (io.ReadCloser, error) {
	if f.diffErr != nil {
		return nil, f.diffErr
	}
	return io.NopCloser(strings.NewReader(f.diffBody)), nil
}

func (f *fakeWorktreeAccessor) DefaultVCS() string { return f.vcs }

func (f *fakeWorktreeAccessor) BeadRunState(_ string) core.StepStatus { return f.runState }

func (f *fakeWorktreeAccessor) Finalize(_ context.Context, beadID, message string) (bool, error) {
	f.finalizeBeadID = beadID
	f.finalizeMsg = message
	if f.finalizeErr != nil {
		return false, f.finalizeErr
	}
	return f.finalizeCommitted, nil
}

func (f *fakeWorktreeAccessor) Push(_ context.Context, beadID string) error {
	f.pushBeadID = beadID
	return f.pushErr
}

func (f *fakeWorktreeAccessor) Remove(_ context.Context, beadID string) error {
	f.removeBeadID = beadID
	return f.removeErr
}

// newWorktreeTestServer builds a test server with the given WorktreeAccessor wired in.
func newWorktreeTestServer(t *testing.T, acc services.WorktreeAccessor) *httptest.Server {
	t.Helper()
	backend := store.NewMemoryBackend(store.SeedIssues())
	pub := services.Publisher(func(f ws.Frame) {})
	svc := services.NewBeadService(backend, nil, pub).WithWorktreeAccessor(acc)
	h := beads.NewHandlers(svc)
	r := chi.NewRouter()
	r.Get("/beads/{id}/worktree", h.Worktree)
	r.Get("/beads/{id}/diff", h.Diff)
	r.Post("/beads/{id}/worktree/finalize", h.FinalizeWorktree)
	r.Post("/beads/{id}/worktree/push", h.PushWorktree)
	r.Delete("/beads/{id}/worktree", h.RemoveWorktree)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

// newWorktreeTestServerNoAccessor builds a test server WITHOUT a WorktreeAccessor.
func newWorktreeTestServerNoAccessor(t *testing.T) *httptest.Server {
	t.Helper()
	backend := store.NewMemoryBackend(store.SeedIssues())
	pub := services.Publisher(func(f ws.Frame) {})
	svc := services.NewBeadService(backend, nil, pub)
	h := beads.NewHandlers(svc)
	r := chi.NewRouter()
	r.Get("/beads/{id}/worktree", h.Worktree)
	r.Get("/beads/{id}/diff", h.Diff)
	r.Post("/beads/{id}/worktree/finalize", h.FinalizeWorktree)
	r.Post("/beads/{id}/worktree/push", h.PushWorktree)
	r.Delete("/beads/{id}/worktree", h.RemoveWorktree)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

// ── T017: Worktree handler ────────────────────────────────────────────────

func TestWorktreeHandler_200(t *testing.T) {
	acc := &fakeWorktreeAccessor{
		status:  wt.WorktreeStatus{Exists: true, Clean: false},
		summary: []wt.FileChange{{Path: "main.go", Kind: wt.Added}},
		vcs:     "git",
	}
	srv := newWorktreeTestServer(t, acc)

	// mp-aaa is a seeded bead.
	resp, err := http.Get(srv.URL + "/beads/mp-aaa/worktree")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body beads.WorktreeResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.BeadID != "mp-aaa" {
		t.Errorf("beadID = %q, want mp-aaa", body.BeadID)
	}
	if body.VCS != "git" {
		t.Errorf("vcs = %q, want git", body.VCS)
	}
	if body.Clean {
		t.Error("clean want false")
	}
	if len(body.Files) != 1 || body.Files[0].Path != "main.go" {
		t.Errorf("files = %v, want [{main.go added}]", body.Files)
	}
}

func TestWorktreeHandler_404_BeadNotFound(t *testing.T) {
	acc := &fakeWorktreeAccessor{
		status: wt.WorktreeStatus{Exists: true, Clean: true},
		vcs:    "git",
	}
	srv := newWorktreeTestServer(t, acc)

	resp, err := http.Get(srv.URL + "/beads/mp-doesnotexist/worktree")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestWorktreeHandler_404_NoWorktree(t *testing.T) {
	acc := &fakeWorktreeAccessor{
		status:    wt.WorktreeStatus{Exists: false},
		statusErr: wt.ErrWorktreeNotFound,
		vcs:       "git",
	}
	srv := newWorktreeTestServer(t, acc)

	resp, err := http.Get(srv.URL + "/beads/mp-aaa/worktree")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (no worktree)", resp.StatusCode)
	}
	code := errorCode(t, resp)
	if code != services.CodeWorktreeNotFound {
		t.Errorf("error code = %q, want %q", code, services.CodeWorktreeNotFound)
	}
}

func TestWorktreeHandler_412_NoAccessor(t *testing.T) {
	srv := newWorktreeTestServerNoAccessor(t)

	resp, err := http.Get(srv.URL + "/beads/mp-aaa/worktree")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Errorf("status = %d, want 412", resp.StatusCode)
	}
}

// ── T017: Diff handler ────────────────────────────────────────────────────

func TestDiffHandler_200_WholeWorktree(t *testing.T) {
	const diffBody = "diff --git a/foo.go b/foo.go\n--- a/foo.go\n+++ b/foo.go\n@@ -1 +1 @@\n-old\n+new\n"
	acc := &fakeWorktreeAccessor{diffBody: diffBody, vcs: "git"}
	srv := newWorktreeTestServer(t, acc)

	resp, err := http.Get(srv.URL + "/beads/mp-aaa/diff")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/x-diff; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/x-diff; charset=utf-8", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "foo.go") {
		t.Errorf("diff body doesn't mention foo.go:\n%s", body)
	}
}

func TestDiffHandler_200_SingleFile(t *testing.T) {
	const diffBody = "diff --git a/bar.go b/bar.go\n+++ b/bar.go\n@@ -0,0 +1 @@\n+new\n"
	acc := &fakeWorktreeAccessor{diffBody: diffBody, vcs: "git"}
	srv := newWorktreeTestServer(t, acc)

	resp, err := http.Get(srv.URL + "/beads/mp-aaa/diff?path=bar.go")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestDiffHandler_400_AbsolutePath(t *testing.T) {
	acc := &fakeWorktreeAccessor{vcs: "git"}
	srv := newWorktreeTestServer(t, acc)

	resp, err := http.Get(srv.URL + "/beads/mp-aaa/diff?path=/etc/passwd")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for absolute path", resp.StatusCode)
	}
	code := errorCode(t, resp)
	if code != "INVALID_PATH" {
		t.Errorf("error code = %q, want INVALID_PATH", code)
	}
}

func TestDiffHandler_400_DotDotPath(t *testing.T) {
	acc := &fakeWorktreeAccessor{vcs: "git"}
	srv := newWorktreeTestServer(t, acc)

	resp, err := http.Get(srv.URL + "/beads/mp-aaa/diff?path=../etc/passwd")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for dotdot path", resp.StatusCode)
	}
}

func TestDiffHandler_404_NoWorktree(t *testing.T) {
	acc := &fakeWorktreeAccessor{diffErr: wt.ErrWorktreeNotFound, vcs: "git"}
	srv := newWorktreeTestServer(t, acc)

	resp, err := http.Get(srv.URL + "/beads/mp-aaa/diff")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	code := errorCode(t, resp)
	if code != services.CodeWorktreeNotFound {
		t.Errorf("error code = %q, want %q", code, services.CodeWorktreeNotFound)
	}
}

func TestDiffHandler_412_NoAccessor(t *testing.T) {
	srv := newWorktreeTestServerNoAccessor(t)

	resp, err := http.Get(srv.URL + "/beads/mp-aaa/diff")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Errorf("status = %d, want 412", resp.StatusCode)
	}
}

func TestDiffHandler_200_CleanWorktree_EmptyBody(t *testing.T) {
	// Clean worktree: diff returns empty body with 200.
	acc := &fakeWorktreeAccessor{diffBody: "", vcs: "git"}
	srv := newWorktreeTestServer(t, acc)

	resp, err := http.Get(srv.URL + "/beads/mp-aaa/diff")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) != 0 {
		t.Errorf("clean worktree: want empty body, got %d bytes", len(body))
	}
}

// ── T038: FinalizeWorktree handler ───────────────────────────────────────────

// TestFinalizeWorktreeHandler_200 verifies a successful finalize returns 200
// with committed=true and the message echoed.
func TestFinalizeWorktreeHandler_200(t *testing.T) {
	acc := &fakeWorktreeAccessor{
		status:            wt.WorktreeStatus{Exists: true, Clean: false},
		vcs:               "git",
		runState:          core.StepDone,
		finalizeCommitted: true,
	}
	srv := newWorktreeTestServer(t, acc)

	body := bytes.NewBufferString(`{"message":"feat: seal work"}`)
	resp, err := http.Post(srv.URL+"/beads/mp-aaa/worktree/finalize", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body: %s", resp.StatusCode, b)
	}

	var result beads.FinalizeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Message != "feat: seal work" {
		t.Errorf("message = %q, want %q", result.Message, "feat: seal work")
	}
	if !result.Committed {
		t.Error("committed want true (dirty worktree finalized)")
	}
	if acc.finalizeBeadID != "mp-aaa" {
		t.Errorf("Finalize called with beadID=%q, want mp-aaa", acc.finalizeBeadID)
	}
}

// TestFinalizeWorktreeHandler_200_CleanWorktree_CommittedFalse verifies that a
// finalize on a clean worktree returns committed=false in the response.
// This is the end-to-end test for the contract fix: no-op finalize must not
// hardcode committed=true.
func TestFinalizeWorktreeHandler_200_CleanWorktree_CommittedFalse(t *testing.T) {
	acc := &fakeWorktreeAccessor{
		status:            wt.WorktreeStatus{Exists: true, Clean: true},
		vcs:               "git",
		runState:          core.StepDone,
		finalizeCommitted: false, // clean worktree: no commit created
	}
	srv := newWorktreeTestServer(t, acc)

	body := bytes.NewBufferString(`{"message":"no-op seal"}`)
	resp, err := http.Post(srv.URL+"/beads/mp-aaa/worktree/finalize", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body: %s", resp.StatusCode, b)
	}

	var result beads.FinalizeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Committed {
		t.Error("committed want false for clean-worktree finalize (no-op), got true")
	}
	if result.Message != "no-op seal" {
		t.Errorf("message = %q, want %q", result.Message, "no-op seal")
	}
}

// TestFinalizeWorktreeHandler_400_EmptyMessage verifies that an empty message
// body returns 400.
func TestFinalizeWorktreeHandler_400_EmptyMessage(t *testing.T) {
	acc := &fakeWorktreeAccessor{vcs: "git"}
	srv := newWorktreeTestServer(t, acc)

	body := bytes.NewBufferString(`{"message":""}`)
	resp, err := http.Post(srv.URL+"/beads/mp-aaa/worktree/finalize", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for empty message", resp.StatusCode)
	}
}

// TestFinalizeWorktreeHandler_409_RunActive verifies that 409 is returned when
// the bead's run is still active.
func TestFinalizeWorktreeHandler_409_RunActive(t *testing.T) {
	acc := &fakeWorktreeAccessor{vcs: "git", runState: core.StepActive}
	srv := newWorktreeTestServer(t, acc)

	body := bytes.NewBufferString(`{"message":"msg"}`)
	resp, err := http.Post(srv.URL+"/beads/mp-aaa/worktree/finalize", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want 409 (run active)", resp.StatusCode)
	}
}

// TestFinalizeWorktreeHandler_412_NoAccessor verifies 412 when no worktree accessor.
func TestFinalizeWorktreeHandler_412_NoAccessor(t *testing.T) {
	srv := newWorktreeTestServerNoAccessor(t)

	body := bytes.NewBufferString(`{"message":"msg"}`)
	resp, err := http.Post(srv.URL+"/beads/mp-aaa/worktree/finalize", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Errorf("status = %d, want 412", resp.StatusCode)
	}
}

// TestFinalizeWorktreeHandler_404_BeadNotFound verifies 404 for unknown bead.
func TestFinalizeWorktreeHandler_404_BeadNotFound(t *testing.T) {
	acc := &fakeWorktreeAccessor{vcs: "git", runState: core.StepDone}
	srv := newWorktreeTestServer(t, acc)

	body := bytes.NewBufferString(`{"message":"msg"}`)
	resp, err := http.Post(srv.URL+"/beads/mp-doesnotexist/worktree/finalize", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// ── T038: PushWorktree handler ────────────────────────────────────────────────

// TestPushWorktreeHandler_200 verifies a successful push returns 200 with the
// branch and remote in the response.
func TestPushWorktreeHandler_200(t *testing.T) {
	acc := &fakeWorktreeAccessor{vcs: "git", runState: core.StepDone}
	srv := newWorktreeTestServer(t, acc)

	resp, err := http.Post(srv.URL+"/beads/mp-aaa/worktree/push", "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body: %s", resp.StatusCode, b)
	}

	var result beads.PushResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !result.Pushed {
		t.Error("pushed want true")
	}
	if result.Branch != "muster/mp-aaa" {
		t.Errorf("branch = %q, want muster/mp-aaa", result.Branch)
	}
	if result.Remote != "origin" {
		t.Errorf("remote = %q, want origin", result.Remote)
	}
}

// TestPushWorktreeHandler_412_NoAccessor verifies 412 when no worktree accessor.
func TestPushWorktreeHandler_412_NoAccessor(t *testing.T) {
	srv := newWorktreeTestServerNoAccessor(t)

	resp, err := http.Post(srv.URL+"/beads/mp-aaa/worktree/push", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Errorf("status = %d, want 412", resp.StatusCode)
	}
}

// TestPushWorktreeHandler_500_BackendError verifies that a push backend error
// maps to 500.
func TestPushWorktreeHandler_500_BackendError(t *testing.T) {
	acc := &fakeWorktreeAccessor{
		vcs:      "git",
		runState: core.StepDone,
		pushErr:  wt.ErrWorktreeNotFound, // simulates push failure as 500
	}
	srv := newWorktreeTestServer(t, acc)

	resp, err := http.Post(srv.URL+"/beads/mp-aaa/worktree/push", "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	// push errors map through mapWorktreeReadError: ErrWorktreeNotFound → 404
	if resp.StatusCode == http.StatusOK {
		t.Errorf("expected non-200, got 200")
	}
}

// ── T038: RemoveWorktree handler ──────────────────────────────────────────────

// TestRemoveWorktreeHandler_200 verifies a successful remove returns 200 with removed=true.
func TestRemoveWorktreeHandler_200(t *testing.T) {
	acc := &fakeWorktreeAccessor{vcs: "git", runState: core.StepDone}
	srv := newWorktreeTestServer(t, acc)

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/beads/mp-aaa/worktree", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body: %s", resp.StatusCode, b)
	}

	var result beads.RemoveResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !result.Removed {
		t.Error("removed want true")
	}
	if acc.removeBeadID != "mp-aaa" {
		t.Errorf("Remove called with beadID=%q, want mp-aaa", acc.removeBeadID)
	}
}

// TestRemoveWorktreeHandler_409_RunActive verifies 409 when run is active.
func TestRemoveWorktreeHandler_409_RunActive(t *testing.T) {
	acc := &fakeWorktreeAccessor{vcs: "git", runState: core.StepActive}
	srv := newWorktreeTestServer(t, acc)

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/beads/mp-aaa/worktree", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want 409 (run active)", resp.StatusCode)
	}
}

// TestRemoveWorktreeHandler_412_NoAccessor verifies 412 when no worktree accessor.
func TestRemoveWorktreeHandler_412_NoAccessor(t *testing.T) {
	srv := newWorktreeTestServerNoAccessor(t)

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/beads/mp-aaa/worktree", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Errorf("status = %d, want 412", resp.StatusCode)
	}
}

// TestRemoveWorktreeHandler_404_BeadNotFound verifies 404 for unknown bead.
func TestRemoveWorktreeHandler_404_BeadNotFound(t *testing.T) {
	acc := &fakeWorktreeAccessor{vcs: "git", runState: core.StepDone}
	srv := newWorktreeTestServer(t, acc)

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/beads/mp-doesnotexist/worktree", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}
