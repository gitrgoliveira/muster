package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	if err != nil {
		fmt.Fprintf(os.Stderr, "watcher: fsnotify unavailable: %v; falling back to polling\n", err)
		return w.runPolling(ctx)
	}
	// Watch the parent directory rather than the file itself. bd rewrites
	// issues.jsonl via temp-file + rename, which detaches an inode-level watch;
	// watching the directory and filtering by filename keeps the fast fsnotify
	// path working across atomic replacements.
	dir := filepath.Dir(w.path)
	if addErr := fsw.Add(dir); addErr != nil {
		fmt.Fprintf(os.Stderr, "watcher: cannot watch %s: %v; falling back to polling\n", dir, addErr)
		_ = fsw.Close()
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
				fmt.Fprintf(os.Stderr, "watcher: fsnotify channel closed; falling back to polling\n")
				_ = fsw.Close()
				return w.runPolling(ctx)
			}
			// The directory watch reports events for every entry; only react
			// to ones affecting our target file.
			if filepath.Clean(ev.Name) != w.path {
				continue
			}
			if ev.Has(fsnotify.Write) || ev.Has(fsnotify.Create) || ev.Has(fsnotify.Rename) || ev.Has(fsnotify.Remove) {
				source = "fsnotify"
				if debounceTimer != nil {
					if !debounceTimer.Stop() {
						select {
						case <-debounceTimer.C:
						default:
						}
					}
				}
				debounceTimer = time.NewTimer(w.debounce)
				timerCh = debounceTimer.C
			}

		case werr := <-fsw.Errors:
			if werr != nil {
				fmt.Fprintf(os.Stderr, "watcher: fsnotify error: %v\n", werr)
			}

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

// reloader is implemented by backends (e.g. JSONL) that cache file contents and
// can be told to re-read unconditionally.
type reloader interface {
	Reload(ctx context.Context) error
}

// emitDiff lists current issues, diffs against snapshot, and emits an event if anything changed.
func (w *Watcher) emitDiff(ctx context.Context, source string) {
	// On a definitive fsnotify signal, force a reload so a same-size rewrite at
	// the same mtime is not masked by the backend's staleness cache. The poll
	// path keeps the cheap mtime/size gate.
	if source != "poll" {
		if r, ok := w.backend.(reloader); ok {
			if err := r.Reload(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "watcher: %s reload failed: %v\n", source, err)
			}
		}
	}

	issues, err := w.backend.List(ctx, Filter{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "watcher: %s diff failed: %v\n", source, err)
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

	ev := WatcherEvent{
		Source:     source,
		ChangedIDs: changed,
		CreatedIDs: created,
		DeletedIDs: deleted,
		At:         time.Now(),
	}
	select {
	case w.out <- ev:
		// Only advance the snapshot once the event is delivered. If the send
		// dropped (default branch), the old snapshot is retained so these same
		// changes are re-detected and re-emitted on the next diff instead of
		// being lost permanently.
		w.snapshot = current
	default:
		fmt.Fprintf(os.Stderr, "watcher: event channel full, retrying diff next tick (%d changed, %d created, %d deleted)\n",
			len(changed), len(created), len(deleted))
	}
}
