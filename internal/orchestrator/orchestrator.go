package orchestrator

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/gitrgoliveira/muster/internal/adapter"
	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/tmux"
)

// ErrRunAlreadyActive is returned by Dispatch when a run is already active for
// the given bead (409 Conflict in the HTTP layer).
var ErrRunAlreadyActive = errors.New("run already active for bead")

// ErrUnmappedPrefix is returned when the bead's ID prefix has no repo mapping.
var ErrUnmappedPrefix = errors.New("bead prefix has no repo mapping")

// ErrNoPermissionMode is returned when neither the request nor the default
// provides a permission mode (FR-021: muster never silently defaults autonomy).
var ErrNoPermissionMode = errors.New("permissionMode is required (no default configured)")

// ErrAdapterNotFound is returned when the requested adapter is not registered.
var ErrAdapterNotFound = errors.New("adapter not registered")

// ErrAdapterNotInstalled is returned when the adapter binary is not installed.
var ErrAdapterNotInstalled = errors.New("adapter not installed")

// ErrAdapterNotLoggedIn is returned when the adapter is not logged in.
var ErrAdapterNotLoggedIn = errors.New("adapter not logged in; run: claude auth login")

// Run holds the in-memory state of an active (or recently completed) agent run.
// The registry is rebuilt on restart from tmux.List().
type Run struct {
	BeadID         string
	StepIdx        int             // always 0 in M2
	Loop           int             // always 0 in M2
	Agent          core.AgentID
	Mode           core.Mode
	PermissionMode core.PermissionMode
	Worktree       string          // absolute path to the worktree
	Session        string          // tmux session name (empty in fallback)
	State          core.StepStatus // running | done | failed | cancelled
	ExitCode       int
	StartedAt      time.Time
	EndedAt        time.Time

	// cancel cancels the context for this run's watcher goroutine.
	cancel context.CancelFunc
}

// DispatchRequest carries the inputs for Orchestrator.Dispatch.
type DispatchRequest struct {
	BeadID         string
	BeadTitle      string
	BeadDesc       string
	Agent          core.AgentID
	Mode           core.Mode
	PermissionMode core.PermissionMode // empty = use DefaultPermissionMode
}

// Publisher is a function that broadcasts a WS frame to connected clients.
type Publisher func(frame interface{})

// Orchestrator manages agent run lifecycle.
type Orchestrator struct {
	mu              sync.RWMutex
	runs            map[string]*Run // keyed by beadID

	adapters        *adapter.Registry
	transport       tmux.Manager    // may be a fallback transport
	repoMap         RepoMap
	worktreesDir    string
	defaultPermMode core.PermissionMode
	publish         Publisher
}

// RepoMap maps bead-ID prefixes to absolute repo paths.
type RepoMap map[string]string

// Resolve returns the repo path for a given beadID by extracting the prefix
// (everything before the first '-').
func (m RepoMap) Resolve(beadID string) (string, error) {
	prefix := prefixOf(beadID)
	path, ok := m[prefix]
	if !ok {
		return "", ErrUnmappedPrefix
	}
	return path, nil
}

// Config holds Orchestrator constructor options.
type Config struct {
	Adapters        *adapter.Registry
	Transport       tmux.Manager
	RepoMap         RepoMap
	WorktreesDir    string
	DefaultPermMode core.PermissionMode
	Publish         Publisher
}

// New creates a new Orchestrator.
func New(cfg Config) *Orchestrator {
	return &Orchestrator{
		runs:            make(map[string]*Run),
		adapters:        cfg.Adapters,
		transport:       cfg.Transport,
		repoMap:         cfg.RepoMap,
		worktreesDir:    cfg.WorktreesDir,
		defaultPermMode: cfg.DefaultPermMode,
		publish:         cfg.Publish,
	}
}

// RunCount returns the number of currently active runs.
func (o *Orchestrator) RunCount() int {
	o.mu.RLock()
	defer o.mu.RUnlock()
	count := 0
	for _, r := range o.runs {
		if r.State == core.StepActive {
			count++
		}
	}
	return count
}

// GetRun returns the Run for a beadID, or nil if not found.
func (o *Orchestrator) GetRun(beadID string) *Run {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.runs[beadID]
}

// registerRun adds a run to the registry. Must be called with write lock held.
func (o *Orchestrator) registerRun(r *Run) {
	o.runs[r.BeadID] = r
}

// removeRun removes a run from the registry.
func (o *Orchestrator) removeRun(beadID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.runs, beadID)
}

// resolvePermMode returns the effective permission mode, applying the default
// if the request omits it, or returning ErrNoPermissionMode if neither is set.
func (o *Orchestrator) resolvePermMode(requested core.PermissionMode) (core.PermissionMode, error) {
	if requested != "" {
		if !requested.Valid() {
			return "", &PermModeError{Mode: requested}
		}
		return requested, nil
	}
	if o.defaultPermMode != "" {
		return o.defaultPermMode, nil
	}
	return "", ErrNoPermissionMode
}

// PermModeError is returned when an invalid permission mode is supplied.
type PermModeError struct {
	Mode core.PermissionMode
}

func (e *PermModeError) Error() string {
	return "invalid permissionMode: " + string(e.Mode)
}
