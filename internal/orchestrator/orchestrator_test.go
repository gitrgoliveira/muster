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
	spawnErr  error
	spawnPane string // optional: Session.Pane to report from Spawn
	deadCode  int
	deadDead  bool
	deadErr   error

	// spawnedSession is protected by spawnMu because Spawn can be called
	// concurrently (e.g. in T012 ConcurrentDispatch) and -race would detect
	// the unsynchronized write.
	spawnMu        sync.Mutex
	spawnedSession *tmux.Session

	// killCalled/pipeCalled are written from watcher goroutines
	// (watchRun→finishRun→Kill, Dispatch→Pipe) that can run concurrently across
	// multiple runs, so they're atomic to keep -race clean.
	killCalled  atomic.Bool
	pipeCalled  atomic.Bool
	listReturns []tmux.Session

	// killedNames records every session name passed to Kill (under killMu), so
	// tests can assert the OLD session is torn down on a step transition rather
	// than an empty/wrong name (Copilot #355 regression).
	killMu      sync.Mutex
	killedNames []string

	spawnCount atomic.Int32
	spawnDelay time.Duration // optional: widen the dispatch race window

	// forceDead is a test hook: when set, DeadStatus reports the pane as dead
	// regardless of deadDead. Tests that recover a *live* session (deadDead
	// false) use t.Cleanup to flip this so the background watchRun goroutine
	// (rooted in context.Background(), polling every 500ms) can observe death
	// and exit instead of leaking for the rest of the suite. atomic because
	// watchRun reads it from a goroutine while Cleanup writes it.
	forceDead atomic.Bool
}

func (f *fakeTransport) Detect() (string, error) { return "3.6b", nil }
func (f *fakeTransport) Kill(name string) error {
	f.killCalled.Store(true)
	f.killMu.Lock()
	f.killedNames = append(f.killedNames, name)
	f.killMu.Unlock()
	return nil
}
func (f *fakeTransport) killed() []string {
	f.killMu.Lock()
	defer f.killMu.Unlock()
	return append([]string(nil), f.killedNames...)
}
func (f *fakeTransport) Attach(name string) (string, error) {
	return "tmux attach -t " + name, nil
}
func (f *fakeTransport) Send(name, keys string) error                  { return nil }
func (f *fakeTransport) Capture(name string, esc bool) (string, error) { return "", nil }
func (f *fakeTransport) Pipe(name string) (io.ReadCloser, error) {
	f.pipeCalled.Store(true)
	return io.NopCloser(strings.NewReader("")), nil
}
func (f *fakeTransport) List() ([]tmux.Session, error) { return f.listReturns, nil }
func (f *fakeTransport) DeadStatus(name string) (int, bool, error) {
	if f.forceDead.Load() {
		return f.deadCode, true, nil
	}
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
		Pane:      f.spawnPane,
		StartedAt: time.Now(),
	}
	f.spawnMu.Lock()
	f.spawnedSession = sess
	f.spawnMu.Unlock()
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
	// The fake adapter is a Unix shell script surfaced via a symlink named
	// "claude"; neither works on Windows (no #! interpreter, symlinks often
	// disallowed). Skip here so every caller is guarded centrally, rather than
	// failing in os.Symlink before the caller's own t.Skip can run.
	if runtime.GOOS == "windows" {
		t.Skip("fake claude adapter uses a unix shell script + symlink; not supported on Windows")
	}
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

// TestDispatch_RejectsInvalidBeadID verifies Dispatch validates req.BeadID
// (defense in depth) before it becomes a repo-map key, tmux session name, and
// worktree path/branch. An unsafe value like "../x" must be rejected with
// ErrInvalidBeadID and must not register a run or spawn anything.
func TestDispatch_RejectsInvalidBeadID(t *testing.T) {
	repoPath := initGitRepo(t)
	o, transport := newOrchestratorForTest(t, repoPath)

	for _, bad := range []string{"../x", "MP-abc", "notanid", "mp-", "a/b"} {
		_, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
			BeadID:         bad,
			Agent:          core.AgentClaude,
			Mode:           core.ModeAgent,
			PermissionMode: core.PermAcceptEdits,
		})
		if !errors.Is(err, orchestrator.ErrInvalidBeadID) {
			t.Errorf("Dispatch(beadID=%q) err = %v, want ErrInvalidBeadID", bad, err)
		}
	}
	if transport.spawnedSession != nil {
		t.Error("no session should be spawned for invalid bead IDs")
	}
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

// TestGetAttach_PopulatesPane verifies GetAttach surfaces the tmux pane ID
// the transport reported at Spawn time — services.AttachResponse.Pane was
// previously always empty (see the Copilot finding this fixes).
func TestGetAttach_PopulatesPane(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	o, transport := newOrchestratorForTest(t, repoPath)
	transport.spawnPane = "%3"

	req := orchestrator.DispatchRequest{
		BeadID:         "mp-pane",
		BeadTitle:      "Test task",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	}
	if _, err := o.Dispatch(context.Background(), req); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	resp, err := o.GetAttach("mp-pane", 0)
	if err != nil {
		t.Fatalf("GetAttach: %v", err)
	}
	if !resp.Available {
		t.Fatalf("Available want true got false, reason: %q", resp.Reason)
	}
	if resp.Pane != "%3" {
		t.Errorf("Pane want %%3 got %q", resp.Pane)
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

// TestDispatch_200_JoinedRun is the M4 US4 T048 migration of the former
// TestDispatch_409_DuplicateRun.  A duplicate dispatch of an active bead must
// return 200 + joined:true, NOT ErrRunAlreadyActive.
func TestDispatch_200_JoinedRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	o, transport := newOrchestratorForTest(t, repoPath)
	// Keep the watcher alive so the run stays active.
	t.Cleanup(func() { transport.forceDead.Store(true) })

	req := orchestrator.DispatchRequest{
		BeadID:         "mp-abc",
		BeadTitle:      "Test",
		BeadDesc:       "",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	}

	first, err := o.Dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("first Dispatch: %v", err)
	}
	if first.Joined {
		t.Fatal("first dispatch must not be joined")
	}

	// Second dispatch of the same in-flight bead must join, not error.
	second, err := o.Dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("second (duplicate) Dispatch: %v", err)
	}
	if !second.Joined {
		t.Error("second dispatch of an in-flight bead must return Joined:true")
	}
	if second.Bead == nil {
		t.Fatal("second dispatch result must carry the existing bead")
	}
	if second.Bead.ID != "mp-abc" {
		t.Errorf("joined bead ID want mp-abc got %q", second.Bead.ID)
	}
	// Exactly one run must exist (no new run registered).
	if count := o.RunCount(); count != 1 {
		t.Errorf("RunCount want 1 after join, got %d", count)
	}
}

// TestDispatch_JoinWaitingBead verifies that a duplicate dispatch of a bead
// that is already waiting (StepPending, queued) also joins (Joined:true) rather
// than returning an error. This covers the case where capacity is full and the
// first dispatch is queued, not yet active. (M4 US4 T049)
func TestDispatch_JoinWaitingBead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	setupFakeClaude(t)
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))

	// Set capacity=1 and dispatch one run first so the second is queued.
	transport := &fakeTransport{}
	t.Cleanup(func() { transport.forceDead.Store(true) })

	o := orchestrator.New(orchestrator.Config{
		Adapters:      reg,
		Transport:     transport,
		RepoMap:       orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir:  t.TempDir(),
		MaxConcurrent: 1,
	})

	reqA := orchestrator.DispatchRequest{
		BeadID:         "mp-aaa",
		BeadTitle:      "First",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	}
	reqB := orchestrator.DispatchRequest{
		BeadID:         "mp-bbb",
		BeadTitle:      "Second",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	}

	// Fill the capacity slot.
	if _, err := o.Dispatch(context.Background(), reqA); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}

	// Dispatch mp-bbb — it goes into the queue (capacity full).
	first, err := o.Dispatch(context.Background(), reqB)
	if err != nil {
		t.Fatalf("queued dispatch: %v", err)
	}
	if !first.Queued {
		t.Fatal("expected first dispatch of mp-bbb to be queued")
	}
	if first.Joined {
		t.Fatal("first dispatch of mp-bbb must not be joined")
	}

	// Second dispatch of mp-bbb (still waiting) — must join the waiter.
	second, err := o.Dispatch(context.Background(), reqB)
	if err != nil {
		t.Fatalf("duplicate queued dispatch: %v", err)
	}
	if !second.Joined {
		t.Error("duplicate dispatch of a waiting bead must return Joined:true")
	}
	if second.Bead == nil {
		t.Fatal("joined result must carry the bead")
	}
	if second.Bead.ID != "mp-bbb" {
		t.Errorf("joined bead ID want mp-bbb got %q", second.Bead.ID)
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

// TestDispatch_OnComplete_PanicRecovered verifies that a panic inside the
// caller-supplied OnComplete callback is recovered on the watcher goroutine
// instead of crashing the whole process (OnComplete is an extension point;
// a bug in it must not take down every other in-flight run).
func TestDispatch_OnComplete_PanicRecovered(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	setupFakeClaude(t)

	transport := &fakeTransport{deadDead: true, deadCode: 0}
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))

	// OnComplete panics only for the first bead; the second dispatch's
	// completion must still be observed cleanly, proving the panic didn't
	// take down the watcher goroutine machinery for other runs.
	panicked := make(chan struct{})
	completedCleanly := make(chan struct{})
	o := orchestrator.New(orchestrator.Config{
		Adapters:     reg,
		Transport:    transport,
		RepoMap:      orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir: t.TempDir(),
		OnComplete: func(beadID string, exitCode int, success bool) {
			if beadID == "mp-panic" {
				close(panicked)
				panic("boom: simulated bug in OnComplete")
			}
			close(completedCleanly)
		},
	})

	_, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         "mp-panic",
		BeadTitle:      "Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	select {
	case <-panicked:
	case <-time.After(5 * time.Second):
		t.Fatal("OnComplete was not called within timeout")
	}

	// The orchestrator must still be usable: a second, unrelated dispatch's
	// OnComplete must still fire normally. If the panic were unrecovered, the
	// whole test binary would have already crashed before reaching this point.
	_, err = o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         "mp-afterpanic",
		BeadTitle:      "Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	})
	if err != nil {
		t.Fatalf("Dispatch after a recovered OnComplete panic should still succeed: %v", err)
	}

	select {
	case <-completedCleanly:
	case <-time.After(5 * time.Second):
		t.Fatal("second dispatch's OnComplete was not called within timeout")
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

// TestDispatch_ConcurrentDuplicate verifies FIX 2 (migrated for M4 US4): two
// goroutines dispatching the same bead → exactly one creates the run, the other
// joins it (Joined:true), and only one tmux session is spawned (no TOCTOU
// double-spawn). This replaces the former 409/ErrRunAlreadyActive assertion.
func TestDispatch_ConcurrentDuplicate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	setupFakeClaude(t)

	// Pane stays alive so the run remains active during the race.
	transport := &fakeTransport{deadDead: false, spawnDelay: 50 * time.Millisecond}
	t.Cleanup(func() { transport.forceDead.Store(true) })
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
	var successes, joiners atomic.Int32
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := o.Dispatch(context.Background(), req)
			if err != nil {
				t.Errorf("unexpected dispatch error: %v", err)
				return
			}
			if res.Joined {
				joiners.Add(1)
			} else {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()

	if s := successes.Load(); s != 1 {
		t.Errorf("want exactly 1 success, got %d", s)
	}
	if j := joiners.Load(); j != 1 {
		t.Errorf("want exactly 1 joiner, got %d", j)
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

// ── T009: DispatchResult — compile-only type definition check ─────────────────

// TestDispatchResult_ZeroValueSane verifies the orchestrator-package
// DispatchResult has sane zero values and the expected field set. It is a
// compile-time guard: a missing or renamed field causes a build failure.
func TestDispatchResult_ZeroValueSane(t *testing.T) {
	var dr orchestrator.DispatchResult

	if dr.Bead != nil {
		t.Errorf("DispatchResult.Bead zero value: want nil, got %v", dr.Bead)
	}
	if dr.Joined {
		t.Error("DispatchResult.Joined zero value: want false")
	}
	if dr.Queued {
		t.Error("DispatchResult.Queued zero value: want false")
	}

	// Exercise all fields to confirm they compile.
	_ = orchestrator.DispatchResult{Bead: nil, Joined: true, Queued: true}
}

// ── T051: Race — many concurrent identical dispatches yield exactly one run ──

// TestDispatch_RaceIdentical verifies that many concurrent dispatches for the
// same bead yield exactly one run (no leaked sessions or goroutines). All
// concurrent callers either succeed with Joined:true or are the sole winner
// that creates the run. (M4 US4 T051, -race clean)
func TestDispatch_RaceIdentical(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	o, transport := newOrchestratorForTest(t, repoPath)
	// Keep the watcher alive so the run stays active for the full test.
	t.Cleanup(func() { transport.forceDead.Store(true) })

	req := orchestrator.DispatchRequest{
		BeadID:         "mp-race",
		BeadTitle:      "Race test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	}

	const n = 20
	type result struct {
		res orchestrator.DispatchResult
		err error
	}
	results := make(chan result, n)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := o.Dispatch(context.Background(), req)
			results <- result{res, err}
		}()
	}
	wg.Wait()
	close(results)

	var winners, joiners, errs int
	for r := range results {
		if r.err != nil {
			errs++
			t.Errorf("concurrent dispatch error: %v", r.err)
			continue
		}
		if r.res.Joined {
			joiners++
		} else {
			winners++
		}
	}

	if errs > 0 {
		t.Fatalf("got %d errors from %d concurrent dispatches", errs, n)
	}
	if winners != 1 {
		t.Errorf("want exactly 1 winner, got %d (joiners=%d)", winners, joiners)
	}
	if joiners != n-1 {
		t.Errorf("want %d joiners, got %d", n-1, joiners)
	}
	// Exactly one run must be registered.
	if count := o.RunCount(); count != 1 {
		t.Errorf("RunCount want 1 got %d", count)
	}
}

// ── T052: Fresh dispatch after terminal state starts a new run ───────────────

// TestDispatch_FreshAfterTerminal verifies that after a run reaches a terminal
// state (StepDone/StepFailed and evicted), a fresh dispatch starts a new run
// rather than erroring or joining the old one. (M4 US4 T052)
func TestDispatch_FreshAfterTerminal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	setupFakeClaude(t)

	// Transport: pane is immediately dead (exit 0) so finishRun fires quickly.
	transport := &fakeTransport{deadDead: true, deadCode: 0}
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))

	done := make(chan struct{})
	o := orchestrator.New(orchestrator.Config{
		Adapters:     reg,
		Transport:    transport,
		RepoMap:      orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir: t.TempDir(),
		// Very short retention so the eviction fires promptly.
		RunRetention: 10 * time.Millisecond,
		OnComplete: func(beadID string, exitCode int, success bool) {
			// Signal only for the first completion (close is idempotent on nil channel).
			select {
			case <-done:
			default:
				close(done)
			}
		},
	})

	req := orchestrator.DispatchRequest{
		BeadID:         "mp-fresh",
		BeadTitle:      "Fresh test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	}

	// First dispatch — completes immediately.
	first, err := o.Dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	if first.Joined {
		t.Fatal("first dispatch must not be joined")
	}

	// Wait for completion + eviction.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("OnComplete not called")
	}

	// Wait for eviction.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if o.GetRun("mp-fresh") == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if o.GetRun("mp-fresh") != nil {
		t.Fatal("run should have been evicted before second dispatch")
	}

	// Second dispatch after eviction must start a NEW run (not join the old one).
	second, err := o.Dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("second dispatch after terminal: %v", err)
	}
	if second.Joined {
		t.Error("second dispatch after terminal must NOT be joined (new run)")
	}
	if second.Bead == nil {
		t.Fatal("second dispatch must return a bead")
	}
}

// ── T008: Run struct M4 extensions — compile-only skeleton ───────────────────

// TestRun_M4Fields_ZeroValueSane verifies that the M4 additions to Run have
// sane zero values: Chain nil (single-step default) and Quota.Known false (no
// usage data yet). This is a compile-time guard — if the fields are removed or
// renamed the test fails to build.
func TestRun_M4Fields_ZeroValueSane(t *testing.T) {
	var r orchestrator.Run

	// Chain nil means single-step M2 behaviour; never accidentally non-nil.
	if r.Chain != nil {
		t.Errorf("Run.Chain zero value: want nil, got %v", r.Chain)
	}

	// Quota.Known false means "no usage data" — the correct initial state before
	// US5 wires the on-disk reader.
	if r.Quota.Known {
		t.Error("Run.Quota.Known zero value: want false")
	}
}

// TestDispatch_ErrorPath_DrainsWaiters is the regression for tri-review-2 CRIT:
// when an admitted run's doLaunch fails while other dispatches are queued behind
// it (capacity 1), the Dispatch error path must launch the next FIFO waiter
// rather than stranding it. With every launch failing (spawnErr), the scheduler
// must fully drain — a stranded waiter would leave activeCount>0 / waiting!=0
// forever.
func TestDispatch_ErrorPath_DrainsWaiters(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell")
	}
	repoPath := initGitRepo(t)
	setupFakeClaude(t)
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))
	transport := &fakeTransport{spawnErr: errors.New("simulated spawn failure")}

	o := orchestrator.New(orchestrator.Config{
		Adapters:        reg,
		Transport:       transport,
		RepoMap:         orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir:    t.TempDir(),
		DefaultPermMode: core.PermAcceptEdits,
		MaxConcurrent:   1, // one active slot → the rest queue behind it
	})

	// Fire several dispatches concurrently so some queue while the admitted one
	// is in doLaunch (which fails at Spawn).
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = o.Dispatch(context.Background(), orchestrator.DispatchRequest{
				BeadID:         "mp-drain" + string(rune('a'+n)),
				Agent:          core.AgentClaude,
				Mode:           core.ModeAgent,
				PermissionMode: core.PermAcceptEdits,
			})
		}(i)
	}
	wg.Wait()

	// Poll until the scheduler fully drains (every launch failed, so nothing
	// should remain active or waiting). A stranded waiter never drains.
	deadline := time.Now().Add(3 * time.Second)
	for {
		snap := o.SchedulerSnapshot()
		if snap.ActiveCount == 0 && len(snap.Waiting) == 0 {
			return // drained — fix works
		}
		if time.Now().After(deadline) {
			t.Fatalf("scheduler did not drain: activeCount=%d waiting=%v (stranded waiter — tri-review-2 CRIT)", snap.ActiveCount, snap.Waiting)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
