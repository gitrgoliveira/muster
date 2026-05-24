// Package jsonl implements a read-only store.Backend that reads from issues.jsonl.
package jsonl

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gitrgoliveira/muster/internal/store"
)

const (
	maxFileSize = 64 << 20 // 64 MB
	maxLineSize = 4 << 20  // 4 MB
)

// Backend reads issues from a JSONL file.
type Backend struct {
	path  string
	mu    sync.RWMutex
	cache []store.Issue
	mtime time.Time
}

// NewJSONL constructs a Backend for the given beads directory.
// Returns an error if issues.jsonl does not exist or exceeds 64 MB.
func NewJSONL(beadsDir string) (*Backend, error) {
	path := filepath.Join(beadsDir, "issues.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("issues.jsonl: %w", err)
	}
	if info.Size() > maxFileSize {
		return nil, fmt.Errorf("issues.jsonl exceeds 64 MB limit (%d bytes)", info.Size())
	}
	b := &Backend{path: path}
	if err := b.reload(); err != nil {
		return nil, err
	}
	return b, nil
}

// List returns issues matching the filter.
func (b *Backend) List(_ context.Context, f store.Filter) ([]store.Issue, error) {
	if err := b.refreshIfStale(); err != nil {
		return nil, err
	}
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]store.Issue, 0)
	for _, iss := range b.cache {
		if !matchesFilter(iss, f) {
			continue
		}
		if f.TruncateDesc > 0 && len(iss.Description) > f.TruncateDesc {
			iss.Description = iss.Description[:f.TruncateDesc]
		}
		result = append(result, iss)
		if f.Limit > 0 && len(result) >= f.Limit {
			break
		}
	}
	return result, nil
}

// Get returns the issue with the given ID, or store.ErrNotFound.
func (b *Backend) Get(_ context.Context, id string) (*store.Issue, error) {
	if id == "" {
		return nil, store.ErrNotFound
	}
	if err := b.refreshIfStale(); err != nil {
		return nil, err
	}
	b.mu.RLock()
	defer b.mu.RUnlock()

	for i := range b.cache {
		if b.cache[i].ID == id {
			cp := b.cache[i]
			return &cp, nil
		}
	}
	return nil, store.ErrNotFound
}

// Ping checks that the file is still readable.
func (b *Backend) Ping(_ context.Context) error {
	_, err := os.Stat(b.path)
	return err
}

// Close is a no-op for the JSONL backend.
func (b *Backend) Close() error { return nil }

// Refresh re-reads the file if the mtime has changed.
func (b *Backend) refreshIfStale() error {
	info, err := os.Stat(b.path)
	if err != nil {
		return fmt.Errorf("stat issues.jsonl: %w", err)
	}
	b.mu.RLock()
	stale := info.ModTime().After(b.mtime)
	b.mu.RUnlock()
	if !stale {
		return nil
	}
	return b.reload()
}

// reload parses the JSONL file and updates the cache.
func (b *Backend) reload() error {
	var issues []store.Issue
	var lastErr error

	// Retry up to 3 times in case of a partial write during atomic rename.
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(100 * time.Millisecond)
		}
		issues, lastErr = b.parse()
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		return lastErr
	}

	info, err := os.Stat(b.path)
	if err != nil {
		return fmt.Errorf("stat issues.jsonl: %w", err)
	}

	b.mu.Lock()
	b.cache = issues
	b.mtime = info.ModTime()
	b.mu.Unlock()
	return nil
}

// parse reads and parses the JSONL file.
func (b *Backend) parse() ([]store.Issue, error) {
	f, err := os.Open(b.path)
	if err != nil {
		return nil, fmt.Errorf("open issues.jsonl: %w", err)
	}
	defer f.Close()

	var issues []store.Issue
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64<<10), maxLineSize)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var iss store.Issue
		if err := json.Unmarshal([]byte(line), &iss); err != nil {
			// Skip unparseable lines (e.g., partial writes).
			continue
		}
		issues = append(issues, iss)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan issues.jsonl: %w", err)
	}
	return issues, nil
}

func matchesFilter(iss store.Issue, f store.Filter) bool {
	if len(f.Status) > 0 {
		found := false
		for _, s := range f.Status {
			if strings.EqualFold(iss.Status, s) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(f.IDs) > 0 {
		found := false
		for _, id := range f.IDs {
			if iss.ID == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
