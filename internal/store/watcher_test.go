package store_test

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

// writeJSONLFile writes issues to path as JSONL.
func writeJSONLFile(t *testing.T, path string, issues []store.Issue) {
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

func atomicWriteIssues(t *testing.T, path string, issues []store.Issue) {
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
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(sb.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
}

var seed = []store.Issue{
	{ID: "mp-001", Title: "First", Status: "open"},
	{ID: "mp-002", Title: "Second", Status: "in_progress"},
}

func TestWatcher_InitialSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issues.jsonl")
	writeJSONLFile(t, path, seed)

	backend, err := jsonl.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close() //nolint:errcheck

	out := make(chan store.WatcherEvent, 10)
	w := store.NewWatcher(backend, path, out)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	// Give the watcher a moment to initialise.
	time.Sleep(100 * time.Millisecond)

	// No events should have been emitted for the initial snapshot.
	select {
	case ev := <-out:
		t.Errorf("unexpected event on startup: %+v", ev)
	default:
	}

	cancel()
	<-done
}

func TestWatcher_DetectsChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issues.jsonl")
	writeJSONLFile(t, path, seed)

	backend, err := jsonl.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close() //nolint:errcheck

	out := make(chan store.WatcherEvent, 10)
	w := store.NewWatcher(backend, path, out)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go w.Run(ctx) //nolint:errcheck

	// Allow initial snapshot.
	time.Sleep(100 * time.Millisecond)

	// Write updated content — change mp-001 title.
	updated := []store.Issue{
		{ID: "mp-001", Title: "First UPDATED", Status: "open"},
		{ID: "mp-002", Title: "Second", Status: "in_progress"},
	}
	atomicWriteIssues(t, path, updated)
	// Bump mtime so the backend detects the change.
	now := time.Now().Add(time.Second)
	os.Chtimes(path, now, now) //nolint:errcheck

	select {
	case ev := <-out:
		if !containsID(ev.ChangedIDs, "mp-001") && !containsID(ev.CreatedIDs, "mp-001") {
			t.Errorf("expected mp-001 in changed/created IDs; event: %+v", ev)
		}
	case <-time.After(8 * time.Second):
		t.Error("timeout waiting for watcher event")
	}
}

func TestWatcher_IdenticalContentNoEvent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issues.jsonl")
	writeJSONLFile(t, path, seed)

	backend, err := jsonl.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close() //nolint:errcheck

	out := make(chan store.WatcherEvent, 10)
	w := store.NewWatcher(backend, path, out)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go w.Run(ctx) //nolint:errcheck
	time.Sleep(100 * time.Millisecond)

	// Write identical content.
	atomicWriteIssues(t, path, seed)
	now := time.Now().Add(time.Second)
	os.Chtimes(path, now, now) //nolint:errcheck

	// Allow debounce to fire.
	time.Sleep(800 * time.Millisecond)

	select {
	case ev := <-out:
		t.Errorf("expected no event for identical content, got %+v", ev)
	default:
	}
}

func TestWatcher_DetectsNewIssue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issues.jsonl")
	writeJSONLFile(t, path, seed)

	backend, err := jsonl.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close() //nolint:errcheck

	out := make(chan store.WatcherEvent, 10)
	w := store.NewWatcher(backend, path, out)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go w.Run(ctx) //nolint:errcheck
	time.Sleep(100 * time.Millisecond)

	// Add a new issue.
	newIssues := append(seed, store.Issue{ID: "mp-003", Title: "New", Status: "open"})
	atomicWriteIssues(t, path, newIssues)
	now := time.Now().Add(time.Second)
	os.Chtimes(path, now, now) //nolint:errcheck

	select {
	case ev := <-out:
		if !containsID(ev.CreatedIDs, "mp-003") {
			t.Errorf("expected mp-003 in CreatedIDs; event: %+v", ev)
		}
	case <-time.After(8 * time.Second):
		t.Error("timeout waiting for created event")
	}
}

func TestWatcher_DetectsDeletedIssue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issues.jsonl")
	writeJSONLFile(t, path, seed)

	backend, err := jsonl.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close() //nolint:errcheck

	out := make(chan store.WatcherEvent, 10)
	w := store.NewWatcher(backend, path, out)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go w.Run(ctx) //nolint:errcheck
	time.Sleep(100 * time.Millisecond)

	// Remove mp-002.
	atomicWriteIssues(t, path, seed[:1])
	now := time.Now().Add(time.Second)
	os.Chtimes(path, now, now) //nolint:errcheck

	select {
	case ev := <-out:
		if !containsID(ev.DeletedIDs, "mp-002") {
			t.Errorf("expected mp-002 in DeletedIDs; event: %+v", ev)
		}
	case <-time.After(8 * time.Second):
		t.Error("timeout waiting for deleted event")
	}
}

func TestWatcher_WatcherEventFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issues.jsonl")
	writeJSONLFile(t, path, seed)

	backend, err := jsonl.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close() //nolint:errcheck

	out := make(chan store.WatcherEvent, 10)
	w := store.NewWatcher(backend, path, out)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go w.Run(ctx) //nolint:errcheck
	time.Sleep(100 * time.Millisecond)

	updated := []store.Issue{{ID: "mp-001", Title: "Updated", Status: "open"}, seed[1]}
	atomicWriteIssues(t, path, updated)
	now := time.Now().Add(time.Second)
	os.Chtimes(path, now, now) //nolint:errcheck

	select {
	case ev := <-out:
		if ev.At.IsZero() {
			t.Error("WatcherEvent.At should be set")
		}
		if ev.Source == "" {
			t.Error("WatcherEvent.Source should be set")
		}
	case <-time.After(8 * time.Second):
		t.Error("timeout")
	}
}

func containsID(ids []string, id string) bool {
	for _, s := range ids {
		if s == id {
			return true
		}
	}
	return false
}
