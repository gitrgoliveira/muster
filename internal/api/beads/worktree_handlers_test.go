package beads_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/api/beads"
	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/gitrgoliveira/muster/internal/store"
	"github.com/gitrgoliveira/muster/internal/ws"
	"github.com/gitrgoliveira/muster/internal/wt"
	"github.com/go-chi/chi/v5"
)

// ── fake WorktreeAccessor ──────────────────────────────────────────────────

type fakeWorktreeAccessor struct {
	status     wt.WorktreeStatus
	statusErr  error
	summary    []wt.FileChange
	summaryErr error
	diffBody   string
	diffErr    error
	vcs        string
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
