package store

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	watcherDebounce = 500 * time.Millisecond
	watcherPoll     = 5 * time.Second
)

// WatcherEvent describes changes detected in issues.jsonl.
type WatcherEvent struct {
	Source     string    // "fsnotify" or "poll"
	ChangedIDs []string  // IDs whose content changed
	CreatedIDs []string  // IDs not present before
	DeletedIDs []string  // IDs no longer present
	At         time.Time // when the diff was computed
}

// Watcher watches issues.jsonl for changes and emits WatcherEvents.
type Watcher struct {
	backend  Backend
	path     string
	out      chan<- WatcherEvent
	debounce time.Duration
	poll     time.Duration

	// snapshot maps ID → Issue for the last known state.
	snapshot map[string]Issue
}

// NewWatcher creates a Watcher. out must be buffered to avoid dropping events.
func NewWatcher(backend Backend, path string, out chan<- WatcherEvent) *Watcher {
	return &Watcher{
		backend:  backend,
		path:     path,
		out:      out,
		debounce: watcherDebounce,
		poll:     watcherPoll,
	}
}

// Run starts the watcher and blocks until ctx is done.
// It populates the initial snapshot synchronously before starting fsnotify,
// so the first real file change triggers a diff (not a flood of create events).
func (w *Watcher) Run(ctx context.Context) error {
	// Populate initial snapshot before watching.
	if err := w.refreshSnapshot(ctx); err != nil {
		return fmt.Errorf("initial snapshot: %w", err)
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil || fsw.Add(w.path) != nil {
		// Fall back to pure polling if fsnotify is unavailable.
		if fsw != nil {
			_ = fsw.Close()
		}
		return w.runPolling(ctx)
	}
	defer fsw.Close() //nolint:errcheck

	var debounceTimer *time.Timer
	var timerCh <-chan time.Time
	source := "fsnotify"

	pollTick := time.NewTicker(w.poll)
	defer pollTick.Stop()

	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return ctx.Err()

		case ev, ok := <-fsw.Events:
			if !ok {
				return nil
			}
			if ev.Has(fsnotify.Write) || ev.Has(fsnotify.Create) || ev.Has(fsnotify.Rename) {
				// Re-add after rename (atomic write).
				if ev.Has(fsnotify.Rename) {
					fsw.Add(w.path) //nolint:errcheck
				}
				source = "fsnotify"
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.NewTimer(w.debounce)
				timerCh = debounceTimer.C
			}

		case <-fsw.Errors:
			// Ignore watch errors; polling fallback covers us.

		case <-timerCh:
			timerCh = nil
			w.emitDiff(ctx, source)

		case <-pollTick.C:
			w.emitDiff(ctx, "poll")
		}
	}
}

// runPolling is the fallback loop used when fsnotify is unavailable.
func (w *Watcher) runPolling(ctx context.Context) error {
	tick := time.NewTicker(w.poll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			w.emitDiff(ctx, "poll")
		}
	}
}

// refreshSnapshot loads the current issues into the snapshot without emitting events.
func (w *Watcher) refreshSnapshot(ctx context.Context) error {
	issues, err := w.backend.List(ctx, Filter{})
	if err != nil {
		return err
	}
	m := make(map[string]Issue, len(issues))
	for _, iss := range issues {
		m[iss.ID] = iss
	}
	w.snapshot = m
	return nil
}

// emitDiff lists current issues, diffs against snapshot, and emits an event if anything changed.
func (w *Watcher) emitDiff(ctx context.Context, source string) {
	issues, err := w.backend.List(ctx, Filter{})
	if err != nil {
		return
	}

	current := make(map[string]Issue, len(issues))
	for _, iss := range issues {
		current[iss.ID] = iss
	}

	var changed, created, deleted []string

	for id, cur := range current {
		prev, existed := w.snapshot[id]
		if !existed {
			created = append(created, id)
		} else if !reflect.DeepEqual(prev, cur) {
			changed = append(changed, id)
		}
	}
	for id := range w.snapshot {
		if _, still := current[id]; !still {
			deleted = append(deleted, id)
		}
	}

	if len(changed) == 0 && len(created) == 0 && len(deleted) == 0 {
		return
	}

	w.snapshot = current
	select {
	case w.out <- WatcherEvent{
		Source:     source,
		ChangedIDs: changed,
		CreatedIDs: created,
		DeletedIDs: deleted,
		At:         time.Now(),
	}:
	default:
		fmt.Fprintf(os.Stderr, "watcher: event channel full, dropping diff (%d changed, %d created, %d deleted)\n",
			len(changed), len(created), len(deleted))
	}
}
