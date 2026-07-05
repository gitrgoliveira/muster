package orchestrator

import (
	"errors"
	"sync"

	"github.com/gitrgoliveira/muster/internal/core"
)

// ErrInvalidCapacity is returned when capacity is set to a non-positive value.
var ErrInvalidCapacity = errors.New("capacity must be > 0")

// SchedulerSnapshot is a point-in-time view of the scheduler state.
type SchedulerSnapshot struct {
	Capacity    int
	ActiveCount int
	Waiting     []string // bead IDs in FIFO order
}

// scheduler is a capacity-gated FIFO queue embedded in the Orchestrator.
// Most methods mutate shared state and MUST be called with the orchestrator's
// mu held (write lock) — see each method's own doc comment for its exact
// contract. Two deliberate exceptions: snapshot() only reads and accepts a read
// OR write lock, and setCapacity() acquires the write lock internally (so it
// MUST be called WITHOUT the lock already held).
type scheduler struct {
	capacity int
	active   map[string]struct{} // bead IDs currently running
	waiting  []*Run              // FIFO queue of StepPending runs
}

// newScheduler constructs a scheduler with the given capacity.
// capacity must be > 0; the caller is responsible for validation.
func newScheduler(capacity int) *scheduler {
	return &scheduler{
		capacity: capacity,
		active:   make(map[string]struct{}),
	}
}

// admitOrEnqueue admits the run (returns queued=false) when a slot is free, or
// appends it to the FIFO queue (queued=true) and returns its 0-based position.
// Must be called with the orchestrator's write lock held.
func (s *scheduler) admitOrEnqueue(run *Run) (queued bool, pos int) {
	if len(s.active) < s.capacity {
		s.active[run.BeadID] = struct{}{}
		return false, 0
	}
	pos = len(s.waiting) // 0-based index this run will occupy after append
	s.waiting = append(s.waiting, run)
	return true, pos
}

// evictAndPopWaiter is the launch-failure cleanup shared by Dispatch's deferred
// error path and launchAdmittedRun's error path (tri-review 4 DRY). Under the
// lock it removes `run` from the registry (only if it is still the current entry
// for its bead — a later dispatch may have replaced it), pops the next FIFO
// waiter via onRunEnd, and flips that waiter to StepActive. Returns the waiter to
// launch (nil if the queue was empty), which the caller MUST `go
// launchAdmittedRun` outside the lock — otherwise the waiter's capacity slot leaks.
func (o *Orchestrator) evictAndPopWaiter(run *Run) *Run {
	o.mu.Lock()
	defer o.mu.Unlock()
	cur, ok := o.runs[run.BeadID]
	if !ok || cur != run {
		return nil
	}
	delete(o.runs, run.BeadID)
	next := o.sched.onRunEnd(run.BeadID)
	if next != nil {
		next.State = core.StepActive
		// launching sentinel: blocks Advance/LoopBack until doLaunch arms
		// run.cancel, closing the nil-cancel window (tri-review 6).
		next.pendingAdvance = true
	}
	return next
}

// onRunEnd removes the finished bead from the active set and pops the next
// waiter (if any). Returns the next run to launch (nil if queue is empty).
// Must be called with the orchestrator's write lock held.
func (s *scheduler) onRunEnd(beadID string) *Run {
	delete(s.active, beadID)
	if len(s.waiting) == 0 {
		return nil
	}
	next := s.waiting[0]
	s.waiting = s.waiting[1:]
	s.active[next.BeadID] = struct{}{}
	return next
}

// recoverActive adds a beadID directly to the active set, bypassing the
// capacity check. This is used only by RecoverSessions at startup: recovered
// runs may transiently exceed capacity (the limit drains naturally as they
// finish). Must be called with the orchestrator's write lock held.
func (s *scheduler) recoverActive(beadID string) {
	s.active[beadID] = struct{}{}
}

// SetCapacity changes the scheduler's capacity. n must be > 0. If n is larger
// than the current capacity, the method admits as many waiters as possible FIFO
// (up to the newly freed slots). If n is smaller, the capacity is lowered
// (drain semantics: running agents are never killed; new slots fill once runs
// finish naturally). Must NOT be called with the orchestrator's lock held;
// acquires the lock internally so it can return the list of newly-admitted runs
// to launch outside the lock.
//
// Returns the list of runs newly admitted (to be launched by the caller outside
// the lock) and any error.
func (s *scheduler) setCapacity(mu *sync.RWMutex, n int) ([]*Run, error) {
	if n <= 0 {
		return nil, ErrInvalidCapacity
	}
	mu.Lock()
	defer mu.Unlock()
	s.capacity = n
	// Admit waiters up to the new capacity. Flip State to StepActive under the
	// lock (before launching outside it), mirroring the onRunEnd/Dispatch admit
	// paths — otherwise an admitted-but-not-yet-launched run reads as StepPending
	// and a concurrent idempotent Dispatch would wrongly report it Queued
	// (Copilot #361).
	var admitted []*Run
	for len(s.waiting) > 0 && len(s.active) < s.capacity {
		next := s.waiting[0]
		s.waiting = s.waiting[1:]
		s.active[next.BeadID] = struct{}{}
		next.State = core.StepActive
		// launching sentinel: blocks Advance/LoopBack until doLaunch arms
		// run.cancel, closing the nil-cancel window (tri-review 6).
		next.pendingAdvance = true
		admitted = append(admitted, next)
	}
	return admitted, nil
}

// snapshot returns a point-in-time view of the scheduler state.
// Must be called with the orchestrator's lock held (read or write).
func (s *scheduler) snapshot() SchedulerSnapshot {
	waiting := make([]string, len(s.waiting))
	for i, r := range s.waiting {
		waiting[i] = r.BeadID
	}
	return SchedulerSnapshot{
		Capacity:    s.capacity,
		ActiveCount: len(s.active),
		Waiting:     waiting,
	}
}
