//go:build !windows

// Integration test: real git operations → /worktree and /diff HTTP endpoints.
// Skipped on Windows (Unix path conventions; /dev/null usage in git diff --no-index).

package wt_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/api/beads"
	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/gitrgoliveira/muster/internal/store"
	"github.com/gitrgoliveira/muster/internal/ws"
	"github.com/gitrgoliveira/muster/internal/wt"
	"github.com/go-chi/chi/v5"
)

// worktreeTestAccessor is a real wt-backed WorktreeAccessor for integration tests.
type worktreeTestAccessor struct {
	worktreesDir string
	defaultVCS   string
}

func (a *worktreeTestAccessor) WorktreeStatus(ctx context.Context, beadID string) (wt.WorktreeStatus, error) {
	return wt.GitStatusAt(ctx, a.worktreesDir, beadID)
}

func (a *worktreeTestAccessor) DiffSummary(ctx context.Context, beadID string) ([]wt.FileChange, error) {
	return wt.GitDiffSummaryAt(ctx, a.worktreesDir, beadID)
}

func (a *worktreeTestAccessor) Diff(ctx context.Context, beadID, path string) (io.ReadCloser, error) {
	return wt.GitDiffAt(ctx, a.worktreesDir, beadID, path)
}

func (a *worktreeTestAccessor) DefaultVCS() string { return a.defaultVCS }

// TestIntegration_WorktreeAndDiff is the T020 end-to-end integration test.
// It:
//  1. Creates a real git repo and a linked worktree keyed to a seeded bead ID.
//  2. Writes a modified tracked file and an untracked new file (simulating claude agent output).
//  3. Calls GET /beads/{id}/worktree and asserts the file list has correct kinds.
//  4. Calls GET /beads/{id}/diff and asserts the diff body mentions both files.
//  5. Calls GET /beads/{id}/diff?path=new.go (single-file untracked).
//  6. Calls GET /beads/{id}/diff?path=../escape to verify 400 INVALID_PATH.
func TestIntegration_WorktreeAndDiff(t *testing.T) {
	ctx := context.Background()

	// mp-aaa is the first seeded bead in store.SeedIssues() — use it so the
	// handler's bead-existence check (store.Get) succeeds.
	const beadID = "mp-aaa"

	// ── create a real git repo and a linked worktree ────────────────────────
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()

	backend, err := wt.For(wt.VCSGit)
	if err != nil {
		t.Fatalf("For(git): %v", err)
	}

	wtPath, err := backend.Create(ctx, worktreesDir, repoPath, beadID)
	if err != nil {
		t.Fatalf("Create worktree: %v", err)
	}

	// ── simulate claude agent edits ─────────────────────────────────────────
	// 1. Modify the tracked README.md that initGitRepo seeds.
	if err := os.WriteFile(filepath.Join(wtPath, "README.md"), []byte("# modified by agent\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	// 2. Add a new untracked file.
	if err := os.WriteFile(filepath.Join(wtPath, "new.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write new.go: %v", err)
	}

	// ── wire up HTTP server with a real wt-backed WorktreeAccessor ──────────
	acc := &worktreeTestAccessor{worktreesDir: worktreesDir, defaultVCS: "git"}
	storeBackend := store.NewMemoryBackend(store.SeedIssues())
	pub := services.Publisher(func(f ws.Frame) {})
	svc := services.NewBeadService(storeBackend, nil, pub).WithWorktreeAccessor(acc)

	h := beads.NewHandlers(svc)
	r := chi.NewRouter()
	r.Get("/beads/{id}/worktree", h.Worktree)
	r.Get("/beads/{id}/diff", h.Diff)
	srv := httptest.NewServer(r)
	defer srv.Close()

	// ── 3. GET /worktree ────────────────────────────────────────────────────
	t.Run("worktree lists files with correct kinds", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/beads/" + beadID + "/worktree")
		if err != nil {
			t.Fatalf("GET /worktree: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("/worktree status = %d, body = %s", resp.StatusCode, body)
		}

		var wtResp beads.WorktreeResponse
		if err := json.NewDecoder(resp.Body).Decode(&wtResp); err != nil {
			t.Fatalf("decode /worktree: %v", err)
		}
		if wtResp.VCS != "git" {
			t.Errorf("vcs = %q, want git", wtResp.VCS)
		}
		if wtResp.Clean {
			t.Error("want clean=false (there are uncommitted changes)")
		}

		// Build map of path→kind from the response.
		fileKinds := make(map[string]wt.ChangeKind)
		for _, fc := range wtResp.Files {
			fileKinds[fc.Path] = fc.Kind
		}
		if k, ok := fileKinds["README.md"]; !ok || k != wt.Modified {
			t.Errorf("README.md: want kind=modified, got %v (present=%v)", k, ok)
		}
		if k, ok := fileKinds["new.go"]; !ok || k != wt.Added {
			t.Errorf("new.go: want kind=added, got %v (present=%v)", k, ok)
		}
	})

	// ── 4. GET /diff (whole worktree) ───────────────────────────────────────
	t.Run("diff whole worktree mentions both files", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/beads/" + beadID + "/diff")
		if err != nil {
			t.Fatalf("GET /diff: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("/diff status = %d, body = %s", resp.StatusCode, body)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "text/x-diff; charset=utf-8" {
			t.Errorf("Content-Type = %q, want text/x-diff; charset=utf-8", ct)
		}

		diffBody, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read diff body: %v", err)
		}
		diffStr := string(diffBody)
		if !strings.Contains(diffStr, "README.md") {
			t.Errorf("whole diff: missing README.md\n%s", diffStr)
		}
		if !strings.Contains(diffStr, "new.go") {
			t.Errorf("whole diff: missing new.go (untracked)\n%s", diffStr)
		}
	})

	// ── 5. GET /diff?path=new.go (single untracked file) ───────────────────
	t.Run("diff single untracked file", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/beads/" + beadID + "/diff?path=new.go")
		if err != nil {
			t.Fatalf("GET /diff?path=new.go: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("/diff?path=new.go status = %d, body = %s", resp.StatusCode, body)
		}
		singleBody, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(singleBody), "new.go") {
			t.Errorf("single-file diff: missing new.go\n%s", singleBody)
		}
	})

	// ── 6. GET /diff?path=../escape (path traversal → 400) ─────────────────
	t.Run("diff path traversal returns 400", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/beads/" + beadID + "/diff?path=../escape")
		if err != nil {
			t.Fatalf("GET /diff?path=../escape: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("path traversal: status = %d, want 400", resp.StatusCode)
		}
	})
}
