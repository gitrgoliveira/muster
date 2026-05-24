package jsonl_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gitrgoliveira/muster/internal/store"
	"github.com/gitrgoliveira/muster/internal/store/jsonl"
)

// writeJSONL writes issues to a JSONL file.
func writeJSONL(t *testing.T, path string, issues []store.Issue) {
	t.Helper()
	var sb strings.Builder
	for _, iss := range issues {
		b, err := json.Marshal(iss)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

// atomicWrite writes content via a temp file + rename (simulates bd atomic write).
func atomicWrite(t *testing.T, path string, content string) {
	t.Helper()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
}

var fixtureIssues = []store.Issue{
	{
		ID: "mp-aaa", Title: "First issue", Description: "Short description",
		Status: "open", Priority: 1, IssueType: "task",
		Assignee: "alice", Owner: "team-a",
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	},
	{
		ID: "mp-bbb", Title: "Second issue", Description: "Multi\nline\ndescription",
		Status: "in_progress", Priority: 2, IssueType: "bug",
	},
	{
		ID: "mp-ccc", Title: "Closed issue", Description: strings.Repeat("x", 200),
		Status: "closed", IssueType: "feature",
	},
	{
		ID: "mp-ddd", Title: "Unicode: こんにちは", Description: "日本語テスト",
		Status: "open", IssueType: "task",
	},
	{
		ID: "mp-eee", Title: "No started_at or closed_at", Description: "nil times",
		Status: "in_review", IssueType: "task",
	},
}

func newBackend(t *testing.T) (store.Backend, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "issues.jsonl")
	writeJSONL(t, path, fixtureIssues)
	b, err := jsonl.NewJSONL(dir)
	if err != nil {
		t.Fatalf("NewJSONL: %v", err)
	}
	t.Cleanup(func() { b.Close() })
	return b, path
}

func TestJSONL_ListAll(t *testing.T) {
	b, _ := newBackend(t)
	issues, err := b.List(context.Background(), store.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != len(fixtureIssues) {
		t.Errorf("want %d issues, got %d", len(fixtureIssues), len(issues))
	}
}

func TestJSONL_ListByStatus(t *testing.T) {
	b, _ := newBackend(t)
	issues, err := b.List(context.Background(), store.Filter{Status: []string{"open"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, iss := range issues {
		if !strings.EqualFold(iss.Status, "open") {
			t.Errorf("unexpected status %q in open filter results", iss.Status)
		}
	}
	if len(issues) != 2 {
		t.Errorf("want 2 open issues, got %d", len(issues))
	}
}

func TestJSONL_ListByIDs(t *testing.T) {
	b, _ := newBackend(t)
	issues, err := b.List(context.Background(), store.Filter{IDs: []string{"mp-aaa", "mp-ccc"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 {
		t.Errorf("want 2, got %d", len(issues))
	}
}

func TestJSONL_ListWithLimit(t *testing.T) {
	b, _ := newBackend(t)
	issues, err := b.List(context.Background(), store.Filter{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 {
		t.Errorf("want 2, got %d", len(issues))
	}
}

func TestJSONL_ListTruncateDesc(t *testing.T) {
	b, _ := newBackend(t)
	issues, err := b.List(context.Background(), store.Filter{IDs: []string{"mp-ccc"}, TruncateDesc: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
	if len(issues[0].Description) > 100 {
		t.Errorf("description not truncated: len=%d", len(issues[0].Description))
	}
}

func TestJSONL_GetExisting(t *testing.T) {
	b, _ := newBackend(t)
	iss, err := b.Get(context.Background(), "mp-bbb")
	if err != nil {
		t.Fatal(err)
	}
	if iss.ID != "mp-bbb" {
		t.Errorf("want mp-bbb got %q", iss.ID)
	}
	if iss.Status != "in_progress" {
		t.Errorf("want in_progress got %q", iss.Status)
	}
}

func TestJSONL_GetMissing(t *testing.T) {
	b, _ := newBackend(t)
	_, err := b.Get(context.Background(), "mp-zzz")
	if err != store.ErrNotFound {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestJSONL_GetEmptyID(t *testing.T) {
	b, _ := newBackend(t)
	_, err := b.Get(context.Background(), "")
	if err != store.ErrNotFound {
		t.Errorf("want ErrNotFound for empty id, got %v", err)
	}
}

func TestJSONL_CloseNoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issues.jsonl")
	writeJSONL(t, path, fixtureIssues[:1])
	b, err := jsonl.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestJSONL_FileAbsentError(t *testing.T) {
	dir := t.TempDir()
	_, err := jsonl.NewJSONL(dir)
	if err == nil {
		t.Fatal("want error for missing issues.jsonl")
	}
}

func TestJSONL_UnparseableLineSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issues.jsonl")
	b, _ := json.Marshal(fixtureIssues[0])
	content := string(b) + "\n{not valid json}\n" + func() string {
		b2, _ := json.Marshal(fixtureIssues[1])
		return string(b2) + "\n"
	}()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	backend, err := jsonl.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	issues, err := backend.List(context.Background(), store.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 {
		t.Errorf("want 2 parseable issues, got %d", len(issues))
	}
}

func TestJSONL_OversizeLineSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issues.jsonl")
	// Write one normal issue, then one with a >4MB description (will exceed scanner buffer)
	normal, _ := json.Marshal(fixtureIssues[0])
	big := store.Issue{ID: "mp-big", Title: "big", Description: strings.Repeat("X", 4<<20+1), Status: "open"}
	bigJSON, _ := json.Marshal(big)
	content := string(normal) + "\n" + string(bigJSON) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	backend, err := jsonl.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	issues, err := backend.List(context.Background(), store.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	// The oversized line is skipped; only the normal one survives.
	if len(issues) != 1 {
		t.Errorf("want 1 issue (oversized skipped), got %d", len(issues))
	}
}

func TestJSONL_OversizeFileError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issues.jsonl")
	// Create a file that reports as >64MB via a sparse file trick.
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	// Seek past 64MB and write one byte to set file size.
	if _, err := f.Seek(64<<20+1, 0); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0}); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	_, err = jsonl.NewJSONL(dir)
	if err == nil {
		t.Fatal("want error for oversized file")
	}
	if !strings.Contains(err.Error(), "64 MB") {
		t.Errorf("error %q should mention 64 MB", err.Error())
	}
}

func TestJSONL_AtomicRenameRefresh(t *testing.T) {
	b, path := newBackend(t)

	// Initial read.
	issues, err := b.List(context.Background(), store.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	origCount := len(issues)

	// Sleep briefly to ensure mtime difference is detectable.
	time.Sleep(10 * time.Millisecond)

	// Add an extra issue via atomic rename.
	extra := append(fixtureIssues, store.Issue{ID: "mp-new", Title: "New", Status: "open"})
	atomicWrite(t, path, func() string {
		var sb strings.Builder
		for _, iss := range extra {
			raw, _ := json.Marshal(iss)
			sb.Write(raw)
			sb.WriteByte('\n')
		}
		return sb.String()
	}())

	// Force mtime to differ by touching timestamp.
	now := time.Now().Add(time.Second)
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatal(err)
	}

	issues2, err := b.List(context.Background(), store.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues2) != origCount+1 {
		t.Errorf("after atomic rename: want %d issues, got %d", origCount+1, len(issues2))
	}
}

func TestJSONL_Ping(t *testing.T) {
	b, _ := newBackend(t)
	if err := b.Ping(context.Background()); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

func TestJSONL_Unicode(t *testing.T) {
	b, _ := newBackend(t)
	iss, err := b.Get(context.Background(), "mp-ddd")
	if err != nil {
		t.Fatal(err)
	}
	if iss.Title != "Unicode: こんにちは" {
		t.Errorf("title mismatch: %q", iss.Title)
	}
}

func TestJSONL_MultipleStatuses(t *testing.T) {
	b, _ := newBackend(t)
	issues, err := b.List(context.Background(), store.Filter{Status: []string{"open", "closed"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 3 {
		t.Errorf("want 3 (2 open + 1 closed), got %d", len(issues))
	}
}

func TestJSONL_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issues.jsonl")
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := jsonl.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	issues, err := b.List(context.Background(), store.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Errorf("want 0, got %d", len(issues))
	}
}

func TestJSONL_ListEmpty(t *testing.T) {
	b, _ := newBackend(t)
	issues, err := b.List(context.Background(), store.Filter{Status: []string{"nonexistent_status"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Errorf("want 0, got %d", len(issues))
	}
}

// Ensure the result slice is never nil (not just empty).
func TestJSONL_ListReturnsNonNilSlice(t *testing.T) {
	b, _ := newBackend(t)
	issues, err := b.List(context.Background(), store.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if issues == nil {
		t.Error("want non-nil slice, got nil")
	}
}

// Verify the cache does not share memory with callers.
func TestJSONL_GetReturnsCopy(t *testing.T) {
	b, _ := newBackend(t)
	iss, err := b.Get(context.Background(), "mp-aaa")
	if err != nil {
		t.Fatal(err)
	}
	iss.Title = "mutated"
	iss2, err := b.Get(context.Background(), "mp-aaa")
	if err != nil {
		t.Fatal(err)
	}
	if iss2.Title == "mutated" {
		t.Error("Get should return a copy, not a shared pointer")
	}
}

func TestJSONL_ConcurrentReads(t *testing.T) {
	// Race detector canary — concurrent List calls must not race.
	b, _ := newBackend(t)

	done := make(chan struct{}, 10)
	for i := 0; i < 10; i++ {
		go func() {
			b.List(context.Background(), store.Filter{}) //nolint:errcheck
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
