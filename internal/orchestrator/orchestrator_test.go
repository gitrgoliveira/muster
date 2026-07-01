package orchestrator_test

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gitrgoliveira/muster/internal/adapter"
	"github.com/gitrgoliveira/muster/internal/adapter/claude"
	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/orchestrator"
	"github.com/gitrgoliveira/muster/internal/tmux"
)

// completion captures an OnComplete invocation.
type completion struct {
	beadID   string
	exitCode int
	success  bool
}

// fakeTransport is a minimal tmux.Manager that records calls and returns
// controllable results.
type fakeTransport struct {
	spawnErr       error
	deadCode       int
	deadDead       bool
	deadErr        error
	spawnedSession *tmux.Session

	killCalled  bool
	pipeCalled  bool
	listReturns []tmux.Session

	spawnCount atomic.Int32
	spawnDelay time.Duration // optional: widen the dispatch race window
}

func (f *fakeTransport) Detect() (string, error) { return "3.6b", nil }
func (f *fakeTransport) Kill(name string) error  { f.killCalled = true; return nil }
func (f *fakeTransport) Attach(name string) (string, error) {
	return "tmux attach -t " + name, nil
}
func (f *fakeTransport) Send(name, keys string) error                  { return nil }
func (f *fakeTransport) Capture(name string, esc bool) (string, error) { return "", nil }
func (f *fakeTransport) Pipe(name string) (io.ReadCloser, error) {
	f.pipeCalled = true
	return io.NopCloser(strings.NewReader("")), nil
}
func (f *fakeTransport) List() ([]tmux.Session, error) { return f.listReturns, nil }
func (f *fakeTransport) DeadStatus(name string) (int, bool, error) {
	return f.deadCode, f.deadDead, f.deadErr
}
func (f *fakeTransport) Spawn(name, cwd string, env, argv []string) (*tmux.Session, error) {
	f.spawnCount.Add(1)
	if f.spawnDelay > 0 {
		time.Sleep(f.spawnDelay)
	}
	if f.spawnErr != nil {
		return nil, f.spawnErr
	}
	sess := &tmux.Session{
		Name:      name,
		StartedAt: time.Now(),
	}
	f.spawnedSession = sess
	return sess, nil
}

// Verify fakeTransport satisfies the Manager interface.
var _ tmux.Manager = (*fakeTransport)(nil)

// initGitRepo creates a temp git repo with an initial commit.
func initGitRepo(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("git worktrees not tested on Windows")
	}
	dir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@test.com")
	runGit("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit("add", ".")
	runGit("commit", "-m", "init")
	return dir
}

// setupFakeClaude puts the fake_claude.sh on PATH.
func setupFakeClaude(t *testing.T) {
	t.Helper()
	scriptPath := filepath.Join("..", "adapter", "claude", "testdata", "fake_claude.sh")
	abs, err := filepath.Abs(scriptPath)
	if err != nil {
		t.Fatalf("abs fake_claude: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Skipf("fake_claude.sh not found at %s: %v", abs, err)
	}
	binDir := t.TempDir()
	dest := filepath.Join(binDir, "claude")
	if err := os.Symlink(abs, dest); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)
}

func newOrchestratorForTest(t *testing.T, repoPath string) (*orchestrator.Orchestrator, *fakeTransport) {
	t.Helper()
	setupFakeClaude(t)
	transport := &fakeTransport{}
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))
	worktreesDir := t.TempDir()
	o := orchestrator.New(orchestrator.Config{
		Adapters:     reg,
		Transport:    transport,
		RepoMap:      orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir: worktreesDir,
	})
	return o, transport
}

func TestDispatch_HappyPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	o, transport := newOrchestratorForTest(t, repoPath)

	req := orchestrator.DispatchRequest{
		BeadID:         "mp-abc",
		BeadTitle:      "Test task",
		BeadDesc:       "Do the thing",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	}

	_, err := o.Dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if transport.spawnedSession == nil {
		t.Error("expected transport.Spawn to have been called")
	}

	run := o.GetRun("mp-abc")
	if run == nil {
		t.Fatal("GetRun returned nil after Dispatch")
	}
	if run.State != core.StepActive {
		t.Errorf("run state want active got %q", run.State)
	}
}

func TestDispatch_ClientDisconnect_DoesNotAbortDetectOrEnsure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	// Dispatch derives its internal Detect/Ensure timeouts from the caller's
	// ctx (the HTTP request context in production, which net/http cancels on
	// client disconnect) via context.WithoutCancel. If that detachment
	// regresses, a caller-cancelled ctx would propagate into the derived
	// timeout contexts and exec.CommandContext would SIGKILL the in-flight
	// `claude`/`git` subprocess — indistinguishable from a genuinely hung
	// probe, but triggerable by any client just closing its connection.
	// Simulate that by passing an already-cancelled ctx straight into
	// Dispatch and asserting it still succeeds.
	repoPath := initGitRepo(t)
	o, transport := newOrchestratorForTest(t, repoPath)

	req := orchestrator.DispatchRequest{
		BeadID:         "mp-abc",
		BeadTitle:      "Test task",
		BeadDesc:       "Do the thing",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	}

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := o.Dispatch(cancelledCtx, req)
	if err != nil {
		t.Fatalf("Dispatch with an already-cancelled parent ctx should still succeed (Detect/Ensure must not inherit cancellation), got: %v", err)
	}
	if transport.spawnedSession == nil {
		t.Error("expected transport.Spawn to have been called")
	}
}

func TestDispatch_409_DuplicateRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	o, _ := newOrchestratorForTest(t, repoPath)

	req := orchestrator.DispatchRequest{
		BeadID:         "mp-abc",
		BeadTitle:      "Test",
		BeadDesc:       "",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	}

	if _, err := o.Dispatch(context.Background(), req); err != nil {
		t.Fatalf("first Dispatch: %v", err)
	}

	_, err := o.Dispatch(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for duplicate run, got nil")
	}
	if !errors.Is(err, orchestrator.ErrRunAlreadyActive) {
		t.Errorf("want ErrRunAlreadyActive, got %v", err)
	}
}

func TestDispatch_422_UnmappedPrefix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	setupFakeClaude(t)
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))

	o := orchestrator.New(orchestrator.Config{
		Adapters:     reg,
		Transport:    &fakeTransport{},
		RepoMap:      orchestrator.RepoMap{}, // no mappings
		WorktreesDir: t.TempDir(),
	})

	req := orchestrator.DispatchRequest{
		BeadID:         "mp-abc",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	}

	_, err := o.Dispatch(context.Background(), req)
	if !errors.Is(err, orchestrator.ErrUnmappedPrefix) {
		t.Errorf("want ErrUnmappedPrefix, got %v", err)
	}
}

func TestDispatch_PermissionModeResolution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	setupFakeClaude(t)
	transport := &fakeTransport{}
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))

	o := orchestrator.New(orchestrator.Config{
		Adapters:        reg,
		Transport:       transport,
		RepoMap:         orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir:    t.TempDir(),
		DefaultPermMode: core.PermDontAsk,
	})

	req := orchestrator.DispatchRequest{
		BeadID:         "mp-xyz",
		BeadTitle:      "Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: "", // empty → use default
	}

	_, err := o.Dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("Dispatch with default permMode: %v", err)
	}

	run := o.GetRun("mp-xyz")
	if run == nil {
		t.Fatal("GetRun returned nil")
	}
	if run.PermissionMode != core.PermDontAsk {
		t.Errorf("want dontAsk got %q", run.PermissionMode)
	}
}

func TestDispatch_NoPermissionMode_Error(t *testing.T) {
	setupFakeClaude(t)
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))

	o := orchestrator.New(orchestrator.Config{
		Adapters:     reg,
		Transport:    &fakeTransport{},
		RepoMap:      orchestrator.RepoMap{"mp": t.TempDir()},
		WorktreesDir: t.TempDir(),
		// No DefaultPermMode
	})

	req := orchestrator.DispatchRequest{
		BeadID:         "mp-abc",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: "", // empty, no default → error
	}

	_, err := o.Dispatch(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when no permissionMode, got nil")
	}
}

func TestGetAttach_NotRunning(t *testing.T) {
	o := orchestrator.New(orchestrator.Config{
		Adapters:     adapter.NewRegistry(),
		Transport:    &fakeTransport{},
		RepoMap:      orchestrator.RepoMap{},
		WorktreesDir: t.TempDir(),
	})
	resp, err := o.GetAttach("mp-abc", 0)
	if err != nil {
		t.Fatalf("GetAttach: %v", err)
	}
	if resp.Available {
		t.Error("available should be false when no run active")
	}
}

func TestGetAttach_NonZeroIdx(t *testing.T) {
	o := orchestrator.New(orchestrator.Config{
		Adapters:     adapter.NewRegistry(),
		Transport:    &fakeTransport{},
		RepoMap:      orchestrator.RepoMap{},
		WorktreesDir: t.TempDir(),
	})
	resp, err := o.GetAttach("mp-abc", 1)
	if err != nil {
		t.Fatalf("GetAttach: %v", err)
	}
	if resp.Available {
		t.Error("available should be false for non-zero step idx")
	}
}

func TestSendKeys_NotRunning(t *testing.T) {
	o := orchestrator.New(orchestrator.Config{
		Adapters:     adapter.NewRegistry(),
		Transport:    &fakeTransport{},
		RepoMap:      orchestrator.RepoMap{},
		WorktreesDir: t.TempDir(),
	})
	err := o.SendKeys("mp-abc", 0, "y\n")
	if err == nil {
		t.Error("SendKeys should return error when no run active")
	}
}

// TestDispatch_OnComplete_Success verifies that when a run's pane exits 0, the
// OnComplete callback fires with success=true and exitCode 0 (FR-013/SC-007).
func TestDispatch_OnComplete_Success(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	setupFakeClaude(t)

	// Pane is dead with exit 0 the first time DeadStatus is polled.
	transport := &fakeTransport{deadDead: true, deadCode: 0}
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))

	var mu sync.Mutex
	var got *completion
	done := make(chan struct{})
	o := orchestrator.New(orchestrator.Config{
		Adapters:     reg,
		Transport:    transport,
		RepoMap:      orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir: t.TempDir(),
		OnComplete: func(beadID string, exitCode int, success bool) {
			mu.Lock()
			got = &completion{beadID: beadID, exitCode: exitCode, success: success}
			mu.Unlock()
			close(done)
		},
	})

	_, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         "mp-done",
		BeadTitle:      "Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("OnComplete was not called within timeout")
	}

	mu.Lock()
	defer mu.Unlock()
	if got == nil {
		t.Fatal("OnComplete not invoked")
	}
	if got.beadID != "mp-done" {
		t.Errorf("beadID want mp-done got %q", got.beadID)
	}
	if !got.success {
		t.Error("success want true for exit 0")
	}
	if got.exitCode != 0 {
		t.Errorf("exitCode want 0 got %d", got.exitCode)
	}
}

// TestDispatch_RunEviction verifies a finished run stays visible via GetRun
// immediately after completion (so debugging/tests/the API can observe the
// outcome), but is evicted from the registry once RunRetention elapses — the
// registry must not accumulate one entry per bead ever dispatched, unbounded.
func TestDispatch_RunEviction(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	setupFakeClaude(t)

	transport := &fakeTransport{deadDead: true, deadCode: 0}
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))

	done := make(chan struct{})
	o := orchestrator.New(orchestrator.Config{
		Adapters:     reg,
		Transport:    transport,
		RepoMap:      orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir: t.TempDir(),
		RunRetention: 20 * time.Millisecond,
		OnComplete: func(beadID string, exitCode int, success bool) {
			close(done)
		},
	})

	_, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         "mp-evict",
		BeadTitle:      "Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("OnComplete was not called within timeout")
	}

	// Immediately after completion, the run is still visible.
	if run := o.GetRun("mp-evict"); run == nil {
		t.Fatal("GetRun should return the finished run immediately after completion")
	}

	// After RunRetention elapses, it must be evicted.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if o.GetRun("mp-evict") == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("run should have been evicted from the registry after RunRetention elapsed")
}

// TestDispatch_OnComplete_Timeout verifies that the run-timeout/cancel branch
// calls OnComplete with exitCode -1 and success=false (FR-013).
func TestDispatch_OnComplete_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	setupFakeClaude(t)

	transport := &fakeTransport{deadDead: false} // pane never dies on its own
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))

	var mu sync.Mutex
	var got *completion
	done := make(chan struct{})
	o := orchestrator.New(orchestrator.Config{
		Adapters:     reg,
		Transport:    transport,
		RepoMap:      orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir: t.TempDir(),
		RunTimeout:   50 * time.Millisecond,
		OnComplete: func(beadID string, exitCode int, success bool) {
			mu.Lock()
			got = &completion{beadID: beadID, exitCode: exitCode, success: success}
			mu.Unlock()
			close(done)
		},
	})

	_, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         "mp-timeout2",
		BeadTitle:      "Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("OnComplete was not called after timeout")
	}

	mu.Lock()
	defer mu.Unlock()
	if got == nil {
		t.Fatal("OnComplete not invoked")
	}
	if got.success {
		t.Error("success want false on timeout")
	}
	if got.exitCode != -1 {
		t.Errorf("exitCode want -1 got %d", got.exitCode)
	}
}

// TestDispatch_ConcurrentDuplicate verifies FIX 2: two goroutines dispatching
// the same bead → exactly one succeeds, one gets ErrRunAlreadyActive, and only
// one tmux session is spawned (no TOCTOU double-spawn).
func TestDispatch_ConcurrentDuplicate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	setupFakeClaude(t)

	// Pane stays alive so the run remains active during the race.
	transport := &fakeTransport{deadDead: false, spawnDelay: 50 * time.Millisecond}
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))

	o := orchestrator.New(orchestrator.Config{
		Adapters:     reg,
		Transport:    transport,
		RepoMap:      orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir: t.TempDir(),
	})

	req := orchestrator.DispatchRequest{
		BeadID:         "mp-race",
		BeadTitle:      "Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	}

	var wg sync.WaitGroup
	var successes, conflicts atomic.Int32
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := o.Dispatch(context.Background(), req)
			switch {
			case err == nil:
				successes.Add(1)
			case errors.Is(err, orchestrator.ErrRunAlreadyActive):
				conflicts.Add(1)
			default:
				t.Errorf("unexpected dispatch error: %v", err)
			}
		}()
	}
	wg.Wait()

	if s := successes.Load(); s != 1 {
		t.Errorf("want exactly 1 success, got %d", s)
	}
	if c := conflicts.Load(); c != 1 {
		t.Errorf("want exactly 1 conflict, got %d", c)
	}
	if sc := transport.spawnCount.Load(); sc != 1 {
		t.Errorf("want exactly 1 Spawn, got %d", sc)
	}
}

// TestDispatch_UnsupportedMode verifies FIX 4: an unsupported mode is rejected
// before any side effects — ErrUnsupportedMode is returned, no worktree is
// created, and no session is spawned.
func TestDispatch_UnsupportedMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	setupFakeClaude(t)

	transport := &fakeTransport{}
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))

	worktreesDir := t.TempDir()
	o := orchestrator.New(orchestrator.Config{
		Adapters:     reg,
		Transport:    transport,
		RepoMap:      orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir: worktreesDir,
	})

	_, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         "mp-badmode",
		BeadTitle:      "Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeBuild, // claude adapter only supports plan + agent
		PermissionMode: core.PermAcceptEdits,
	})
	if !errors.Is(err, orchestrator.ErrUnsupportedMode) {
		t.Fatalf("want ErrUnsupportedMode, got %v", err)
	}

	// No session spawned.
	if sc := transport.spawnCount.Load(); sc != 0 {
		t.Errorf("want 0 Spawn calls, got %d", sc)
	}
	// No worktree created.
	if entries, _ := os.ReadDir(worktreesDir); len(entries) != 0 {
		t.Errorf("want no worktree created, found %d entries", len(entries))
	}
	// No run registered (reservation released).
	if run := o.GetRun("mp-badmode"); run != nil {
		t.Errorf("want no run registered, got %+v", run)
	}
}

func TestAsServiceDispatcher_NotNil(t *testing.T) {
	o := orchestrator.New(orchestrator.Config{
		Adapters:     adapter.NewRegistry(),
		Transport:    &fakeTransport{},
		RepoMap:      orchestrator.RepoMap{},
		WorktreesDir: t.TempDir(),
	})
	d := o.AsServiceDispatcher()
	if d == nil {
		t.Error("AsServiceDispatcher should not return nil")
	}
}
