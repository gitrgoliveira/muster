package store

import "context"

// Backend is the read-only persistence interface for issues.
// All implementations must be safe for concurrent use by multiple goroutines.
type Backend interface {
	// List returns issues matching the filter. Filter.Status nil/empty = all.
	List(ctx context.Context, f Filter) ([]Issue, error)

	// Get returns the issue with the given ID, or ErrNotFound.
	Get(ctx context.Context, id string) (*Issue, error)

	// Ping reports whether the backend is reachable.
	Ping(ctx context.Context) error

	// Close releases any held resources.
	Close() error
}
