// Package jsonl implements a read-only store.Backend that reads from issues.jsonl.
package jsonl

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	size  int64
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
func (b *Backend) List(ctx context.Context, f store.Filter) ([]store.Issue, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := b.refreshIfStale(); err != nil {
		return nil, err
	}
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]store.Issue, 0)
	for _, iss := range b.cache {
		if !store.MatchesFilter(iss, f) {
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
func (b *Backend) Get(ctx context.Context, id string) (*store.Issue, error) {
	if id == "" {
		return nil, store.ErrNotFound
	}
	if err := ctx.Err(); err != nil {
		return nil, err
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

// Refresh re-reads the file if mtime OR size has changed.
// Size is an additional signal for filesystems with coarse mtime resolution
// where two writes within the same timestamp quantum would otherwise be missed.
func (b *Backend) refreshIfStale() error {
	info, err := os.Stat(b.path)
	if err != nil {
		return fmt.Errorf("stat issues.jsonl: %w", err)
	}
	b.mu.RLock()
	stale := !info.ModTime().Equal(b.mtime) || info.Size() != b.size
	b.mu.RUnlock()
	if !stale {
		return nil
	}
	return b.reload()
}

// reload parses the JSONL file and updates the cache.
// Stat is captured before parse to record the mtime/size for staleness checks.
// Atomic rename between stat and parse is acceptable: at worst we'll record an
// older mtime+size for newer content, triggering one extra reload next call.
func (b *Backend) reload() error {
	info, err := os.Stat(b.path)
	if err != nil {
		return fmt.Errorf("stat issues.jsonl: %w", err)
	}
	if info.Size() > maxFileSize {
		return fmt.Errorf("issues.jsonl exceeds 64 MB limit (%d bytes)", info.Size())
	}
	mtime := info.ModTime()
	size := info.Size()

	issues := make([]store.Issue, 0)
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

	b.mu.Lock()
	b.cache = issues
	b.mtime = mtime
	b.size = size
	b.mu.Unlock()
	return nil
}

// parse reads and parses the JSONL file.
// Lines longer than maxLineSize are silently skipped; unparseable lines are skipped too.
func (b *Backend) parse() ([]store.Issue, error) {
	f, err := os.Open(b.path)
	if err != nil {
		return nil, fmt.Errorf("open issues.jsonl: %w", err)
	}
	defer f.Close()

	reader := bufio.NewReaderSize(f, maxLineSize)
	issues := make([]store.Issue, 0)

	for {
		// ReadLine returns (line, isPrefix, err).
		// isPrefix==true means the internal buffer was full before a newline —
		// the line is longer than maxLineSize; we drain and skip it.
		chunk, isPrefix, rerr := reader.ReadLine()
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			return nil, fmt.Errorf("scan issues.jsonl: %w", rerr)
		}

		if isPrefix {
			// Drain the rest of the oversized line.
			for isPrefix && rerr == nil {
				_, isPrefix, rerr = reader.ReadLine()
			}
			if rerr != nil && !errors.Is(rerr, io.EOF) {
				return nil, fmt.Errorf("scan issues.jsonl: %w", rerr)
			}
			continue
		}

		line := strings.TrimSpace(string(chunk))
		if line == "" {
			continue
		}
		var iss store.Issue
		if err := json.Unmarshal([]byte(line), &iss); err != nil {
			continue
		}
		issues = append(issues, iss)
	}
	return issues, nil
}
