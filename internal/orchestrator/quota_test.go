package orchestrator_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gitrgoliveira/muster/internal/adapter"
	"github.com/gitrgoliveira/muster/internal/adapter/claude"
	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/orchestrator"
	"github.com/gitrgoliveira/muster/internal/ws"
)

// ── T010: QuotaUsage — compile-only type definition check ────────────────────

// TestQuotaUsage_ZeroValueSane verifies that QuotaUsage has sane zero values.
// Known=false means "no usage data" — the correct initial state before US5
// wires the on-disk reader. All numeric fields are zero.
func TestQuotaUsage_ZeroValueSane(t *testing.T) {
	var q orchestrator.QuotaUsage

	if q.Known {
		t.Error("QuotaUsage.Known zero value: want false")
	}
	if q.InputTokens != 0 {
		t.Errorf("QuotaUsage.InputTokens zero value: want 0, got %d", q.InputTokens)
	}
	if q.OutputTokens != 0 {
		t.Errorf("QuotaUsage.OutputTokens zero value: want 0, got %d", q.OutputTokens)
	}
	if q.CostUSD != 0 {
		t.Errorf("QuotaUsage.CostUSD zero value: want 0, got %f", q.CostUSD)
	}

	// Exercise all fields to confirm they compile.
	_ = orchestrator.QuotaUsage{
		Known:        true,
		InputTokens:  1000,
		OutputTokens: 500,
		CostUSD:      0.05,
	}
}

// ── T060: on-disk quota reader tests ─────────────────────────────────────────
//
// These tests exercise ReadSessionQuota using FAKE on-disk session files written
// to a temp directory. They do NOT depend on a real claude installation.

// writeSessionJSONL writes a minimal claude JSONL session file to dir/<sessionID>.jsonl.
// Each entry in usages is one assistant turn's {input_tokens, output_tokens}.
func writeSessionJSONL(t *testing.T, dir string, sessionID string, usages []struct{ in, out int64 }) string {
	t.Helper()
	type usageShape struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
	}
	type msgShape struct {
		Usage usageShape `json:"usage"`
	}
	type entryShape struct {
		Type    string   `json:"type"`
		UUID    string   `json:"uuid"`
		Message msgShape `json:"message"`
	}

	path := filepath.Join(dir, sessionID+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("writeSessionJSONL: create %s: %v", path, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	// Add a non-assistant line first (noise — reader must ignore it).
	noise := map[string]string{"type": "user", "uuid": "u1"}
	if err := enc.Encode(noise); err != nil {
		t.Fatalf("writeSessionJSONL: encode noise: %v", err)
	}
	for i, u := range usages {
		entry := entryShape{
			Type:    "assistant",
			UUID:    "uuid" + string(rune('0'+i)),
			Message: msgShape{Usage: usageShape{InputTokens: u.in, OutputTokens: u.out}},
		}
		if err := enc.Encode(entry); err != nil {
			t.Fatalf("writeSessionJSONL: encode entry %d: %v", i, err)
		}
	}
	// Add a trailing non-assistant line (noise — reader must ignore it).
	trailer := map[string]string{"type": "last-prompt", "lastPrompt": "x"}
	if err := enc.Encode(trailer); err != nil {
		t.Fatalf("writeSessionJSONL: encode trailer: %v", err)
	}
	return path
}

// TestReadSessionQuota_ParsesFakeFile verifies that ReadSessionQuota returns
// Known:true with correct token totals when given a real JSONL file.
func TestReadSessionQuota_ParsesFakeFile(t *testing.T) {
	dir := t.TempDir()

	// Write a fake session JSONL with two assistant turns.
	writeSessionJSONL(t, dir, "session-abc", []struct{ in, out int64 }{
		{in: 1000, out: 200},
		{in: 500, out: 100},
	})

	got := orchestrator.ReadSessionQuota(dir)

	if !got.Known {
		t.Fatalf("ReadSessionQuota: want Known=true, got false")
	}
	if got.InputTokens != 1500 {
		t.Errorf("ReadSessionQuota: InputTokens want 1500 got %d", got.InputTokens)
	}
	if got.OutputTokens != 300 {
		t.Errorf("ReadSessionQuota: OutputTokens want 300 got %d", got.OutputTokens)
	}
	// CostUSD is 0 for interactive sessions (not in the JSONL).
	if got.CostUSD != 0.0 {
		t.Errorf("ReadSessionQuota: CostUSD want 0.0 got %f", got.CostUSD)
	}
}

// TestReadSessionQuota_MultipleSessions picks the most-recently-modified file.
func TestReadSessionQuota_MultipleSessions(t *testing.T) {
	dir := t.TempDir()

	// Write an older session with lower token counts.
	writeSessionJSONL(t, dir, "session-old", []struct{ in, out int64 }{
		{in: 10, out: 5},
	})

	// Small sleep so mtime differs on file systems with 1-second resolution.
	// On most modern macOS/Linux systems mtime has nanosecond precision, but
	// the test helper doesn't force an Chtimes so a tiny delay makes it robust.
	// We use a stat-based comparison anyway, so this is belt-and-suspenders.

	// Write a newer session with higher counts.
	writeSessionJSONL(t, dir, "session-new", []struct{ in, out int64 }{
		{in: 9000, out: 4000},
	})
	// Touch the new file so its mtime is guaranteed newer.
	touchLater(t, filepath.Join(dir, "session-new.jsonl"))

	got := orchestrator.ReadSessionQuota(dir)

	if !got.Known {
		t.Fatalf("ReadSessionQuota multi-session: want Known=true")
	}
	// Should pick the most-recently-modified file (session-new).
	if got.InputTokens != 9000 {
		t.Errorf("ReadSessionQuota multi-session: want InputTokens=9000 got %d", got.InputTokens)
	}
	if got.OutputTokens != 4000 {
		t.Errorf("ReadSessionQuota multi-session: want OutputTokens=4000 got %d", got.OutputTokens)
	}
}

// TestReadSessionQuota_MissingDir returns Known:false and does not error.
func TestReadSessionQuota_MissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "no-such-project-dir")

	got := orchestrator.ReadSessionQuota(dir)

	if got.Known {
		t.Errorf("ReadSessionQuota missing dir: want Known=false, got true")
	}
}

// TestReadSessionQuota_EmptyDir returns Known:false.
func TestReadSessionQuota_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	got := orchestrator.ReadSessionQuota(dir)

	if got.Known {
		t.Errorf("ReadSessionQuota empty dir: want Known=false, got true")
	}
}

// TestReadSessionQuota_GarbledFile returns Known:false.
func TestReadSessionQuota_GarbledFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "garbled.jsonl")
	if err := os.WriteFile(path, []byte("this is not valid JSON\n{\"broken\": true\n"), 0o600); err != nil {
		t.Fatalf("write garbled file: %v", err)
	}

	got := orchestrator.ReadSessionQuota(dir)

	// Even with garbled data: Known=false (no valid assistant turns → zero tokens → unknown).
	// The reader never returns an error — it degrades gracefully.
	if got.Known {
		t.Errorf("ReadSessionQuota garbled: want Known=false, got true")
	}
}

// TestReadSessionQuota_NoAssistantTurns returns Known:false when the JSONL has
// entries but none are assistant messages (e.g. an aborted run).
func TestReadSessionQuota_NoAssistantTurns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-assistant.jsonl")
	// Write only user/system lines — no assistant turns.
	if err := os.WriteFile(path, []byte(
		`{"type":"user","uuid":"u1"}`+"\n"+
			`{"type":"system","uuid":"s1"}`+"\n",
	), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got := orchestrator.ReadSessionQuota(dir)

	if got.Known {
		t.Errorf("ReadSessionQuota no-assistant: want Known=false, got true")
	}
}

// TestReadSessionQuotaForWorktree_EncodesPath verifies that
// ReadSessionQuotaForWorktree correctly derives the project dir from the
// worktree path. It uses a fake ~/.claude/projects structure in a temp home dir.
func TestReadSessionQuotaForWorktree_EncodesPath(t *testing.T) {
	// Create a fake home dir with the expected project path.
	fakeHome := t.TempDir()
	worktree := "/Users/alice/repos/myproj/.claude/worktrees/cool-newton-abc123"
	// Expected encoded dir: replace /. with --, then / with -
	encoded := "-Users-alice-repos-myproj--claude-worktrees-cool-newton-abc123"
	projectDir := filepath.Join(fakeHome, ".claude", "projects", encoded)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	writeSessionJSONL(t, projectDir, "session-xyz", []struct{ in, out int64 }{
		{in: 777, out: 333},
	})

	got := orchestrator.ReadSessionQuotaForWorktree(worktree, fakeHome)

	if !got.Known {
		t.Fatalf("ReadSessionQuotaForWorktree: want Known=true")
	}
	if got.InputTokens != 777 {
		t.Errorf("ReadSessionQuotaForWorktree: InputTokens want 777 got %d", got.InputTokens)
	}
	if got.OutputTokens != 333 {
		t.Errorf("ReadSessionQuotaForWorktree: OutputTokens want 333 got %d", got.OutputTokens)
	}
}

// touchLater sets the mtime of path to now+1s so it sorts after any file
// created just before in the same test.
func touchLater(t *testing.T, path string) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("touchLater stat: %v", err)
	}
	later := fi.ModTime().Add(2e9) // +2 seconds
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatalf("touchLater chtimes: %v", err)
	}
}

// TestReadSessionQuota_LineExceedsBuffer verifies that parseQuotaFromJSONL
// returns Known:false (not partial data) when the JSONL file contains a line
// longer than the 4 MiB scanner buffer. This guards against reporting
// incorrect totals from truncated reads (tri-review FIX 5).
func TestReadSessionQuota_LineExceedsBuffer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge-line.jsonl")

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Write a valid assistant turn first (so found=true before the oversize line).
	validLine := `{"type":"assistant","message":{"usage":{"input_tokens":100,"output_tokens":50}}}` + "\n"
	if _, err := f.WriteString(validLine); err != nil {
		t.Fatalf("write valid line: %v", err)
	}
	// Write a line larger than 4 MiB (4*1024*1024 + 1 bytes) to exceed the buffer.
	bigLine := `{"type":"assistant","x":"` + string(make([]byte, 4*1024*1024+1)) + `"}` + "\n"
	if _, err := f.WriteString(bigLine); err != nil {
		t.Fatalf("write big line: %v", err)
	}
	f.Close()

	got := orchestrator.ReadSessionQuota(dir)

	// Must degrade to Known:false — never report partial data.
	if got.Known {
		t.Errorf("ReadSessionQuota huge line: want Known=false (scan error), got Known=true (tokens=%d/%d)",
			got.InputTokens, got.OutputTokens)
	}
}

// ── T063: quota captured at run end, run.quota event emitted ─────────────────
//
// These tests verify that when a run finishes the orchestrator:
//   (a) reads the on-disk quota record (injected via a fake home dir),
//   (b) attaches it to Run.Quota, and
//   (c) emits a run.quota WS event with the correct fields.
//
// The run.quota event is emitted even when Known=false (best-effort advisory).

// TestT063_RunQuotaEvent_EmittedAfterRunEnd verifies that the orchestrator emits
// a run.quota WS event when a run completes, with Known=true and correct tokens
// when a fake session JSONL exists in the (injected) home dir.
func TestT063_RunQuotaEvent_EmittedAfterRunEnd(t *testing.T) {
	repoPath := initGitRepo(t)
	beadID := "mp-quota1"

	// Create a fake home dir with a session JSONL for the worktree.
	fakeHome := t.TempDir()
	// The worktree will be at <worktreesDir>/<beadID>. Because we don't know the
	// exact path until after Dispatch creates it, we set up the project dir
	// lazily by using the QuotaHomeDir hook. In this test we inject the fake
	// home dir and the reader uses it to find the project dir.
	// For the test to be deterministic, we pre-populate the project dir for the
	// expected worktree path pattern. Since the orchestrator creates the worktree
	// at <WorktreesDir>/<beadID>, we can predict the worktree path:
	worktreesDir := t.TempDir()

	var (
		events []ws.Frame
		mu     sync.Mutex
	)
	setupFakeClaude(t)
	transport := &fakeTransport{deadDead: true}
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))
	publish := func(f ws.Frame) {
		mu.Lock()
		events = append(events, f)
		mu.Unlock()
	}
	o := orchestrator.New(orchestrator.Config{
		Adapters:        reg,
		Transport:       transport,
		RepoMap:         orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir:    worktreesDir,
		DefaultPermMode: core.PermAcceptEdits,
		Publish:         publish,
		RunRetention:    200 * time.Millisecond,
		QuotaHomeDir:    fakeHome,
	})

	// Pre-populate the fake session JSONL under the expected project dir.
	// Worktree path = <worktreesDir>/<beadID>.
	worktreePath := filepath.Join(worktreesDir, beadID)
	// Encoded path: replace /. with --, then / with -.
	projectDir := filepath.Join(fakeHome, ".claude", "projects", encodeClaudeProjectDir(worktreePath))
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeSessionJSONL(t, projectDir, "sess-quota", []struct{ in, out int64 }{
		{in: 2000, out: 800},
	})

	// Dispatch and wait for run to complete.
	_, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         beadID,
		BeadTitle:      "Quota Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	// Wait for run.quota event.
	deadline := time.Now().Add(5 * time.Second)
	var quotaFrame *ws.Frame
	for time.Now().Before(deadline) {
		mu.Lock()
		for i := range events {
			if events[i].Type == ws.EventRunQuota && events[i].BeadID == beadID {
				cp := events[i]
				quotaFrame = &cp
				break
			}
		}
		mu.Unlock()
		if quotaFrame != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if quotaFrame == nil {
		t.Fatal("run.quota event not emitted after run completion")
	}
	if quotaFrame.Quota == nil {
		t.Fatal("run.quota event: Quota field is nil")
	}
	if !quotaFrame.Quota.Known {
		t.Errorf("run.quota event: want Known=true (session JSONL exists)")
	}
	if quotaFrame.Quota.InputTokens != 2000 {
		t.Errorf("run.quota event: InputTokens want 2000 got %d", quotaFrame.Quota.InputTokens)
	}
	if quotaFrame.Quota.OutputTokens != 800 {
		t.Errorf("run.quota event: OutputTokens want 800 got %d", quotaFrame.Quota.OutputTokens)
	}
}

// TestT063_RunQuotaEvent_EmittedKnownFalse verifies that run.quota is emitted
// even when no session JSONL exists (Known=false). The run still succeeds.
func TestT063_RunQuotaEvent_EmittedKnownFalse(t *testing.T) {
	repoPath := initGitRepo(t)
	beadID := "mp-quota2"
	fakeHome := t.TempDir() // empty: no session JSONL

	var (
		events []ws.Frame
		mu     sync.Mutex
	)
	setupFakeClaude(t)
	transport := &fakeTransport{deadDead: true}
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))
	publish := func(f ws.Frame) {
		mu.Lock()
		events = append(events, f)
		mu.Unlock()
	}
	o := orchestrator.New(orchestrator.Config{
		Adapters:        reg,
		Transport:       transport,
		RepoMap:         orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir:    t.TempDir(),
		DefaultPermMode: core.PermAcceptEdits,
		Publish:         publish,
		RunRetention:    200 * time.Millisecond,
		QuotaHomeDir:    fakeHome,
	})

	_, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         beadID,
		BeadTitle:      "Quota Test 2",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	// Wait for run.quota event.
	deadline := time.Now().Add(5 * time.Second)
	var quotaFrame *ws.Frame
	for time.Now().Before(deadline) {
		mu.Lock()
		for i := range events {
			if events[i].Type == ws.EventRunQuota && events[i].BeadID == beadID {
				cp := events[i]
				quotaFrame = &cp
				break
			}
		}
		mu.Unlock()
		if quotaFrame != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if quotaFrame == nil {
		t.Fatal("run.quota event not emitted even for unknown quota")
	}
	if quotaFrame.Quota == nil {
		t.Fatal("run.quota event: Quota field is nil")
	}
	if quotaFrame.Quota.Known {
		t.Errorf("run.quota event: want Known=false (no session JSONL)")
	}
}

// TestT063_RunSummaryIncludesQuota verifies that GetRun returns the quota
// attached to the run after run completion.
func TestT063_RunSummaryIncludesQuota(t *testing.T) {
	repoPath := initGitRepo(t)
	beadID := "mp-quota3"
	worktreesDir := t.TempDir()
	fakeHome := t.TempDir()

	// Pre-populate the session JSONL.
	worktreePath := filepath.Join(worktreesDir, beadID)
	projectDir := filepath.Join(fakeHome, ".claude", "projects", encodeClaudeProjectDir(worktreePath))
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeSessionJSONL(t, projectDir, "sess-summary", []struct{ in, out int64 }{
		{in: 3000, out: 1200},
	})

	setupFakeClaude(t)
	transport := &fakeTransport{deadDead: true}
	reg := adapter.NewRegistry()
	reg.Register(claude.New(claude.Options{}))
	o := orchestrator.New(orchestrator.Config{
		Adapters:        reg,
		Transport:       transport,
		RepoMap:         orchestrator.RepoMap{"mp": repoPath},
		WorktreesDir:    worktreesDir,
		DefaultPermMode: core.PermAcceptEdits,
		RunRetention:    500 * time.Millisecond,
		QuotaHomeDir:    fakeHome,
	})

	_, err := o.Dispatch(context.Background(), orchestrator.DispatchRequest{
		BeadID:         beadID,
		BeadTitle:      "Quota Summary Test",
		Agent:          core.AgentClaude,
		Mode:           core.ModeAgent,
		PermissionMode: core.PermAcceptEdits,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	// Wait for run to complete.
	deadline := time.Now().Add(5 * time.Second)
	var finalRun *orchestrator.Run
	for time.Now().Before(deadline) {
		r := o.GetRun(beadID)
		if r != nil && r.State != core.StepActive {
			finalRun = r
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if finalRun == nil {
		t.Fatal("run did not complete")
	}
	if !finalRun.Quota.Known {
		t.Errorf("run.Quota.Known want true, got false")
	}
	if finalRun.Quota.InputTokens != 3000 {
		t.Errorf("run.Quota.InputTokens want 3000 got %d", finalRun.Quota.InputTokens)
	}
	if finalRun.Quota.OutputTokens != 1200 {
		t.Errorf("run.Quota.OutputTokens want 1200 got %d", finalRun.Quota.OutputTokens)
	}
}

// encodeClaudeProjectDir encodes a cwd into the Claude project directory name.
// This mirrors the logic in quota.go's claudeSessionDir.
func encodeClaudeProjectDir(cwd string) string {
	result := ""
	for i := 0; i < len(cwd); i++ {
		if cwd[i] == '/' {
			if i+1 < len(cwd) && cwd[i+1] == '.' {
				result += "--"
				i++ // skip the dot too
			} else {
				result += "-"
			}
		} else {
			result += string(cwd[i])
		}
	}
	return result
}
