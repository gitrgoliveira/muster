package store

import (
	"context"
	"strings"
	"sync"
)

// MemoryBackend is the in-memory implementation of Backend, backed by []Issue.
// Safe for concurrent use by multiple goroutines.
type MemoryBackend struct {
	mu     sync.RWMutex
	issues []Issue
}

// NewMemoryBackend creates a MemoryBackend seeded with the given issues.
func NewMemoryBackend(seeds []Issue) *MemoryBackend {
	cp := make([]Issue, len(seeds))
	copy(cp, seeds)
	return &MemoryBackend{issues: cp}
}

// List returns issues matching the filter.
func (m *MemoryBackend) List(_ context.Context, f Filter) ([]Issue, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Issue, 0, len(m.issues))
	for _, iss := range m.issues {
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

// Get returns the issue with the given ID, or ErrNotFound.
func (m *MemoryBackend) Get(_ context.Context, id string) (*Issue, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for i := range m.issues {
		if m.issues[i].ID == id {
			cp := m.issues[i]
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

// Ping always succeeds for the in-memory backend.
func (m *MemoryBackend) Ping(_ context.Context) error { return nil }

// Close is a no-op for the in-memory backend.
func (m *MemoryBackend) Close() error { return nil }

func matchesFilter(iss Issue, f Filter) bool {
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
